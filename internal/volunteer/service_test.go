package volunteer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"hearable/backend/internal/ai"
	"hearable/backend/package/audio"
	apperrors "hearable/backend/package/errors"
)

type fakeRepo struct {
	listening []listeningQuestionRecord
	sentences []sentenceRecord
	module    moduleRecord
	err       error
}

func (f *fakeRepo) CreateListeningModule(_ context.Context, m moduleRecord, qs []listeningQuestionRecord) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.module, f.listening = m, qs
	return "module-1", nil
}

func (f *fakeRepo) CreateSpeakingModule(_ context.Context, m moduleRecord, ss []sentenceRecord) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.module, f.sentences = m, ss
	return "module-2", nil
}

type fakeAI struct {
	transcript    string
	transcribeErr error
	promptErr     error
	difficulty    int
	assessErr     error
	assessed      ai.DifficultyInput
}

func (f *fakeAI) Transcribe(context.Context, []byte, string) (string, error) {
	return f.transcript, f.transcribeErr
}

func (f *fakeAI) SuggestListeningPrompt(context.Context, string) (string, error) {
	return "Dengarkan audio berikut.", f.promptErr
}

func (f *fakeAI) AssessDifficulty(_ context.Context, in ai.DifficultyInput) (ai.DifficultyAssessment, error) {
	f.assessed = in
	return ai.DifficultyAssessment{Difficulty: f.difficulty, Explanation: "alasan"}, f.assessErr
}

type fakeStore struct {
	uploaded  []string
	deleted   []string
	failAfter int // fail the Nth upload (1-based); 0 never fails
}

func (f *fakeStore) Upload(_ context.Context, path string, _ []byte, _ string) (string, error) {
	if f.failAfter > 0 && len(f.uploaded)+1 == f.failAfter {
		return "", errors.New("boom")
	}
	f.uploaded = append(f.uploaded, path)
	return "https://cdn.test/" + path, nil
}

func (f *fakeStore) Delete(_ context.Context, paths []string) error {
	f.deleted = append(f.deleted, paths...)
	return nil
}

func newTestService() (*Service, *fakeRepo, *fakeAI, *fakeStore) {
	repo, aiFake, store := &fakeRepo{}, &fakeAI{transcript: "Selamat pagi", difficulty: 2}, &fakeStore{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(repo, aiFake, store, logger), repo, aiFake, store
}

func wantStatus(t *testing.T, err error, status int) {
	t.Helper()
	if appErr, ok := apperrors.As(err); !ok || appErr.Status != status {
		t.Fatalf("want %d, got %v", status, err)
	}
}

var m4a = AudioFile{Data: []byte("x"), Format: audio.Format{Ext: "m4a", ContentType: "audio/mp4"}}

func validListening() CreateListeningRequest {
	return CreateListeningRequest{
		Title: " Sapaan Pagi ", Label: "Sapaan",
		Questions: []ListeningQuestionInput{
			{
				Prompt: "Pilih sapaan yang kamu dengar.", Transcription: "Selamat pagi", Type: QuestionTypeChoice,
				Options: []OptionInput{{Text: "Selamat pagi", IsCorrect: true}, {Text: "Selamat malam"}},
			},
			{Prompt: "Tulis yang kamu dengar.", Transcription: "Apa kabar", Type: QuestionTypeInput, ExpectedAnswer: "Apa kabar"},
		},
	}
}

func TestTranscribe(t *testing.T) {
	svc, _, aiFake, _ := newTestService()
	got, err := svc.Transcribe(t.Context(), m4a)
	if err != nil || got.Transcription != "Selamat pagi" || got.Prompt != "Dengarkan audio berikut." {
		t.Fatalf("got %+v %v", got, err)
	}

	aiFake.promptErr = ai.ErrBadResponse
	got, err = svc.Transcribe(t.Context(), m4a)
	if err != nil || got.Prompt != defaultPrompt {
		t.Fatalf("prompt failure should fall back to default, got %+v %v", got, err)
	}

	aiFake.transcript = ""
	_, err = svc.Transcribe(t.Context(), m4a)
	wantStatus(t, err, http.StatusUnprocessableEntity)

	aiFake.transcribeErr = ai.ErrRateLimited
	_, err = svc.Transcribe(t.Context(), m4a)
	wantStatus(t, err, http.StatusServiceUnavailable)
}

func TestCreateListeningModule(t *testing.T) {
	svc, repo, aiFake, store := newTestService()
	got, err := svc.CreateListeningModule(t.Context(), "user-1", validListening(), []AudioFile{m4a, m4a})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "module-1" || got.Difficulty != 2 || got.DifficultyLabel != "Dasar" || got.EstimatedMinutes != 3 || got.ItemCount != 2 || got.Title != "Sapaan Pagi" {
		t.Fatalf("unexpected module %+v", got)
	}
	if repo.module.CreatedBy != "user-1" || len(repo.listening) != 2 || len(store.uploaded) != 2 {
		t.Fatalf("unexpected persistence %+v %d uploads", repo.module, len(store.uploaded))
	}
	if !strings.HasPrefix(repo.listening[0].AudioURL, "https://cdn.test/volunteer/") || !strings.HasSuffix(repo.listening[0].AudioURL, ".m4a") {
		t.Fatalf("unexpected audio url %s", repo.listening[0].AudioURL)
	}
	if aiFake.assessed.ModuleType != ModuleTypeListening || aiFake.assessed.Items[0].Answer != "Selamat pagi" {
		t.Fatalf("unexpected assessment input %+v", aiFake.assessed)
	}
}

func TestCreateListeningModuleValidation(t *testing.T) {
	mutations := map[string]func(*CreateListeningRequest){
		"no title":      func(r *CreateListeningRequest) { r.Title = " " },
		"no label":      func(r *CreateListeningRequest) { r.Label = "" },
		"no questions":  func(r *CreateListeningRequest) { r.Questions = nil },
		"no transcript": func(r *CreateListeningRequest) { r.Questions[0].Transcription = "" },
		"no prompt":     func(r *CreateListeningRequest) { r.Questions[0].Prompt = "" },
		"bad type":      func(r *CreateListeningRequest) { r.Questions[0].Type = "essay" },
		"one option":    func(r *CreateListeningRequest) { r.Questions[0].Options = r.Questions[0].Options[:1] },
		"two correct":   func(r *CreateListeningRequest) { r.Questions[0].Options[1].IsCorrect = true },
		"no correct":    func(r *CreateListeningRequest) { r.Questions[0].Options[0].IsCorrect = false },
		"blank option":  func(r *CreateListeningRequest) { r.Questions[0].Options[1].Text = " " },
		"no expected":   func(r *CreateListeningRequest) { r.Questions[1].ExpectedAnswer = "" },
		"too many":      func(r *CreateListeningRequest) { r.Questions = make([]ListeningQuestionInput, MaxItems+1) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			svc, _, _, store := newTestService()
			req := validListening()
			mutate(&req)
			files := make([]AudioFile, len(req.Questions))
			for i := range files {
				files[i] = m4a
			}
			_, err := svc.CreateListeningModule(t.Context(), "u", req, files)
			wantStatus(t, err, http.StatusUnprocessableEntity)
			if len(store.uploaded) != 0 {
				t.Fatal("nothing should be uploaded on validation failure")
			}
		})
	}
}

func TestCreateListeningModuleFileCountMismatch(t *testing.T) {
	svc, _, _, _ := newTestService()
	_, err := svc.CreateListeningModule(t.Context(), "u", validListening(), []AudioFile{m4a})
	wantStatus(t, err, http.StatusUnprocessableEntity)
}

func TestCreateListeningModuleAIFailureStoresNothing(t *testing.T) {
	svc, repo, aiFake, store := newTestService()
	aiFake.assessErr = ai.ErrUnavailable
	_, err := svc.CreateListeningModule(t.Context(), "u", validListening(), []AudioFile{m4a, m4a})
	wantStatus(t, err, http.StatusServiceUnavailable)
	if len(store.uploaded) != 0 || repo.listening != nil {
		t.Fatal("nothing should be stored when assessment fails")
	}
}

func TestCreateListeningModuleCleansUpUploads(t *testing.T) {
	t.Run("upload failure", func(t *testing.T) {
		svc, _, _, store := newTestService()
		store.failAfter = 2
		_, err := svc.CreateListeningModule(t.Context(), "u", validListening(), []AudioFile{m4a, m4a})
		wantStatus(t, err, http.StatusBadGateway)
		if len(store.deleted) != 1 || store.deleted[0] != store.uploaded[0] {
			t.Fatalf("first upload should be deleted, got %v", store.deleted)
		}
	})
	t.Run("db failure", func(t *testing.T) {
		svc, repo, _, store := newTestService()
		repo.err = errors.New("db down")
		_, err := svc.CreateListeningModule(t.Context(), "u", validListening(), []AudioFile{m4a, m4a})
		if err == nil || len(store.deleted) != 2 {
			t.Fatalf("both uploads should be deleted, err=%v deleted=%v", err, store.deleted)
		}
	})
}

func TestCreateSpeakingModule(t *testing.T) {
	svc, repo, aiFake, _ := newTestService()
	aiFake.difficulty = 1
	got, err := svc.CreateSpeakingModule(t.Context(), "user-1", CreateSpeakingRequest{
		Title: "Sapaan", Label: "Salam", Sentences: []SentenceInput{{Text: " Selamat Pagi "}, {Text: "Terima Kasih", Description: "ucapan"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "module-2" || got.Type != ModuleTypeSpeaking || got.DifficultyLabel != "Pemula" || got.EstimatedMinutes != 2 || got.ItemCount != 2 {
		t.Fatalf("unexpected module %+v", got)
	}
	s := repo.sentences[0]
	if s.Text != "Selamat Pagi" || s.Description != nil || !strings.Contains(string(s.Syllables), `{"text":"mat","stress":true}`) {
		t.Fatalf("unexpected sentence %+v %s", s, s.Syllables)
	}
	if *repo.sentences[1].Description != "ucapan" || aiFake.assessed.Items[1].Text != "Terima Kasih" {
		t.Fatalf("unexpected second sentence / assessment input")
	}
}

func TestCreateSpeakingModuleValidation(t *testing.T) {
	cases := map[string]CreateSpeakingRequest{
		"no sentences":   {Title: "a", Label: "b"},
		"blank sentence": {Title: "a", Label: "b", Sentences: []SentenceInput{{Text: " "}}},
		"long sentence":  {Title: "a", Label: "b", Sentences: []SentenceInput{{Text: strings.Repeat("a", maxSentenceRunes+1)}}},
		"no title":       {Label: "b", Sentences: []SentenceInput{{Text: "a"}}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			svc, _, _, _ := newTestService()
			_, err := svc.CreateSpeakingModule(t.Context(), "u", req)
			wantStatus(t, err, http.StatusUnprocessableEntity)
		})
	}
}
