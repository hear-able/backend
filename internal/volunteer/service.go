package volunteer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"hearable/backend/internal/ai"
	"hearable/backend/package/difficulty"
	apperrors "hearable/backend/package/errors"
	"hearable/backend/package/syllable"
)

// defaultPrompt is used when the AI cannot draft an instruction; the
// volunteer edits it before submitting anyway.
const defaultPrompt = "Dengarkan audio berikut, lalu jawab pertanyaannya."

type repository interface {
	CreateListeningModule(ctx context.Context, m moduleRecord, questions []listeningQuestionRecord) (string, error)
	CreateSpeakingModule(ctx context.Context, m moduleRecord, sentences []sentenceRecord) (string, error)
}

type contentAI interface {
	Transcribe(ctx context.Context, audio []byte, filename string) (string, error)
	SuggestListeningPrompt(ctx context.Context, transcription string) (string, error)
	AssessDifficulty(ctx context.Context, in ai.DifficultyInput) (ai.DifficultyAssessment, error)
}

type objectStore interface {
	Upload(ctx context.Context, path string, data []byte, contentType string) (string, error)
	Delete(ctx context.Context, paths []string) error
}

type Service struct {
	repo    repository
	ai      contentAI
	storage objectStore
	logger  *slog.Logger
}

func NewService(repo repository, aiClient contentAI, storage objectStore, logger *slog.Logger) *Service {
	return &Service{repo: repo, ai: aiClient, storage: storage, logger: logger}
}

// Transcribe turns a question's audio into text and drafts an instruction.
// Nothing is stored: the volunteer reviews both before submitting.
func (s *Service) Transcribe(ctx context.Context, file AudioFile) (TranscriptionResponse, error) {
	text, err := s.ai.Transcribe(ctx, file.Data, "audio."+file.Format.Ext)
	if err != nil {
		return TranscriptionResponse{}, providerError(err)
	}
	if text == "" {
		return TranscriptionResponse{}, apperrors.Unprocessable("Tidak ada ucapan yang terdeteksi pada audio. Coba unggah audio lain.")
	}

	prompt, err := s.ai.SuggestListeningPrompt(ctx, text)
	if err != nil {
		s.logger.WarnContext(ctx, "suggest listening prompt failed, using default", "error", err)
		prompt = defaultPrompt
	}
	return TranscriptionResponse{Transcription: text, Prompt: prompt}, nil
}

func (s *Service) CreateListeningModule(ctx context.Context, userID string, req CreateListeningRequest, files []AudioFile) (CreatedModule, error) {
	m, err := validateModule(req.Title, req.Label, req.Description)
	if err != nil {
		return CreatedModule{}, err
	}
	if err := validateCount(len(req.Questions), "soal"); err != nil {
		return CreatedModule{}, err
	}
	if len(files) != len(req.Questions) {
		return CreatedModule{}, apperrors.Unprocessable("Setiap soal wajib memiliki satu berkas audio.")
	}

	questions := make([]listeningQuestionRecord, len(req.Questions))
	items := make([]ai.DifficultyItem, len(req.Questions))
	for i, q := range req.Questions {
		rec, answer, err := validateQuestion(i+1, q)
		if err != nil {
			return CreatedModule{}, err
		}
		questions[i] = rec
		items[i] = ai.DifficultyItem{Transcription: rec.Transcription, Instruction: rec.Prompt, AnswerType: rec.Type, Answer: answer}
	}

	assessment, err := s.assess(ctx, ModuleTypeListening, m, items)
	if err != nil {
		return CreatedModule{}, err
	}
	m.Difficulty = assessment.Difficulty
	m.EstimatedMinutes = max(1, int(math.Ceil(float64(len(questions))*1.5)))
	m.CreatedBy = userID

	// Upload only after validation and assessment pass, so rejected submits
	// leave no files behind.
	uploaded := make([]string, 0, len(files))
	for i, f := range files {
		path := fmt.Sprintf("volunteer/%s.%s", randomID(), f.Format.Ext)
		url, err := s.storage.Upload(ctx, path, f.Data, f.Format.ContentType)
		if err != nil {
			s.cleanup(ctx, uploaded)
			return CreatedModule{}, apperrors.Upstream(http.StatusBadGateway, "Audio gagal diunggah. Silakan kirim ulang.", err)
		}
		uploaded = append(uploaded, path)
		questions[i].AudioURL = url
	}

	id, err := s.repo.CreateListeningModule(ctx, m, questions)
	if err != nil {
		s.cleanup(ctx, uploaded)
		return CreatedModule{}, err
	}
	return created(id, ModuleTypeListening, m, assessment, len(questions)), nil
}

func (s *Service) CreateSpeakingModule(ctx context.Context, userID string, req CreateSpeakingRequest) (CreatedModule, error) {
	m, err := validateModule(req.Title, req.Label, req.Description)
	if err != nil {
		return CreatedModule{}, err
	}
	if err := validateCount(len(req.Sentences), "kalimat"); err != nil {
		return CreatedModule{}, err
	}

	sentences := make([]sentenceRecord, len(req.Sentences))
	items := make([]ai.DifficultyItem, len(req.Sentences))
	for i, in := range req.Sentences {
		text := strings.TrimSpace(in.Text)
		switch {
		case text == "":
			return CreatedModule{}, apperrors.Unprocessable(fmt.Sprintf("Kalimat %d wajib diisi.", i+1))
		case utf8.RuneCountInString(text) > maxSentenceRunes:
			return CreatedModule{}, apperrors.Unprocessable(fmt.Sprintf("Kalimat %d maksimal %d karakter.", i+1, maxSentenceRunes))
		case utf8.RuneCountInString(in.Description) > maxDescriptionRunes:
			return CreatedModule{}, apperrors.Unprocessable(fmt.Sprintf("Keterangan kalimat %d maksimal %d karakter.", i+1, maxDescriptionRunes))
		}
		syllables, err := json.Marshal(syllable.Split(text))
		if err != nil {
			return CreatedModule{}, fmt.Errorf("marshal syllables: %w", err)
		}
		sentences[i] = sentenceRecord{Text: text, Description: optional(in.Description), Syllables: syllables}
		items[i] = ai.DifficultyItem{Text: text}
	}

	assessment, err := s.assess(ctx, ModuleTypeSpeaking, m, items)
	if err != nil {
		return CreatedModule{}, err
	}
	m.Difficulty = assessment.Difficulty
	m.EstimatedMinutes = max(1, len(sentences))
	m.CreatedBy = userID

	id, err := s.repo.CreateSpeakingModule(ctx, m, sentences)
	if err != nil {
		return CreatedModule{}, err
	}
	return created(id, ModuleTypeSpeaking, m, assessment, len(sentences)), nil
}

func (s *Service) assess(ctx context.Context, moduleType string, m moduleRecord, items []ai.DifficultyItem) (ai.DifficultyAssessment, error) {
	assessment, err := s.ai.AssessDifficulty(ctx, ai.DifficultyInput{
		ModuleType: moduleType, Title: m.Title, Description: m.Description, Items: items,
	})
	if err != nil {
		return ai.DifficultyAssessment{}, providerError(err)
	}
	if !difficulty.Valid(assessment.Difficulty) {
		return ai.DifficultyAssessment{}, providerError(ai.ErrBadResponse)
	}
	return assessment, nil
}

// cleanup removes uploads after a failed submit. It must not be cancelled
// with the request, otherwise a client disconnect would leak files.
func (s *Service) cleanup(ctx context.Context, paths []string) {
	if len(paths) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if err := s.storage.Delete(ctx, paths); err != nil {
		s.logger.ErrorContext(ctx, "cleanup volunteer uploads failed", "error", err, "count", len(paths))
	}
}

func validateModule(title, label, description string) (moduleRecord, error) {
	title, label, description = strings.TrimSpace(title), strings.TrimSpace(label), strings.TrimSpace(description)
	switch {
	case title == "":
		return moduleRecord{}, apperrors.Unprocessable("Judul modul wajib diisi.")
	case utf8.RuneCountInString(title) > maxTitleRunes:
		return moduleRecord{}, apperrors.Unprocessable(fmt.Sprintf("Judul modul maksimal %d karakter.", maxTitleRunes))
	case label == "":
		return moduleRecord{}, apperrors.Unprocessable("Label modul wajib diisi.")
	case utf8.RuneCountInString(label) > maxLabelRunes:
		return moduleRecord{}, apperrors.Unprocessable(fmt.Sprintf("Label modul maksimal %d karakter.", maxLabelRunes))
	case utf8.RuneCountInString(description) > maxDescriptionRunes:
		return moduleRecord{}, apperrors.Unprocessable(fmt.Sprintf("Deskripsi modul maksimal %d karakter.", maxDescriptionRunes))
	}
	return moduleRecord{Title: title, Label: label, Description: description}, nil
}

func validateCount(n int, noun string) error {
	if n < 1 {
		return apperrors.Unprocessable(fmt.Sprintf("Modul wajib memiliki minimal 1 %s.", noun))
	}
	if n > MaxItems {
		return apperrors.Unprocessable(fmt.Sprintf("Modul maksimal berisi %d %s.", MaxItems, noun))
	}
	return nil
}

// validateQuestion normalises one question and returns the correct answer
// text, which is shown to the difficulty assessor.
func validateQuestion(n int, q ListeningQuestionInput) (listeningQuestionRecord, string, error) {
	fail := func(msg string) (listeningQuestionRecord, string, error) {
		return listeningQuestionRecord{}, "", apperrors.Unprocessable(fmt.Sprintf("Soal %d: %s", n, msg))
	}

	rec := listeningQuestionRecord{
		Type:          q.Type,
		Prompt:        strings.TrimSpace(q.Prompt),
		Transcription: strings.TrimSpace(q.Transcription),
	}
	switch {
	case rec.Transcription == "":
		return fail("transkripsi wajib diisi.")
	case utf8.RuneCountInString(rec.Transcription) > maxTranscriptionRunes:
		return fail(fmt.Sprintf("transkripsi maksimal %d karakter.", maxTranscriptionRunes))
	case rec.Prompt == "":
		return fail("instruksi soal wajib diisi.")
	case utf8.RuneCountInString(rec.Prompt) > maxPromptRunes:
		return fail(fmt.Sprintf("instruksi soal maksimal %d karakter.", maxPromptRunes))
	}

	switch q.Type {
	case QuestionTypeChoice:
		if len(q.Options) < minOptions || len(q.Options) > maxOptions {
			return fail(fmt.Sprintf("pilihan jawaban harus berjumlah %d sampai %d.", minOptions, maxOptions))
		}
		var answer string
		correct := 0
		for i, o := range q.Options {
			text := strings.TrimSpace(o.Text)
			if text == "" {
				return fail(fmt.Sprintf("pilihan %d wajib diisi.", i+1))
			}
			if utf8.RuneCountInString(text) > maxOptionRunes {
				return fail(fmt.Sprintf("pilihan %d maksimal %d karakter.", i+1, maxOptionRunes))
			}
			if o.IsCorrect {
				correct++
				answer = text
			}
			rec.Options = append(rec.Options, optionRecord{Text: text, IsCorrect: o.IsCorrect})
		}
		if correct != 1 {
			return fail("tandai tepat satu pilihan yang benar.")
		}
		if utf8.RuneCountInString(q.AnswerExplanation) > maxAnswerRunes {
			return fail(fmt.Sprintf("penjelasan jawaban maksimal %d karakter.", maxAnswerRunes))
		}
		rec.AnswerExplanation = optional(q.AnswerExplanation)
		return rec, answer, nil
	case QuestionTypeInput:
		rec.ExpectedAnswer = strings.TrimSpace(q.ExpectedAnswer)
		if rec.ExpectedAnswer == "" {
			return fail("jawaban yang diharapkan wajib diisi.")
		}
		if utf8.RuneCountInString(rec.ExpectedAnswer) > maxAnswerRunes {
			return fail(fmt.Sprintf("jawaban yang diharapkan maksimal %d karakter.", maxAnswerRunes))
		}
		return rec, rec.ExpectedAnswer, nil
	}
	return fail("tipe soal harus choice atau input.")
}

func created(id, moduleType string, m moduleRecord, a ai.DifficultyAssessment, count int) CreatedModule {
	return CreatedModule{
		ID:                    id,
		Type:                  moduleType,
		Label:                 m.Label,
		Title:                 m.Title,
		Description:           m.Description,
		Difficulty:            m.Difficulty,
		DifficultyLabel:       difficulty.Label(m.Difficulty),
		DifficultyExplanation: a.Explanation,
		EstimatedMinutes:      m.EstimatedMinutes,
		ItemCount:             count,
	}
}

func optional(s string) *string {
	if s = strings.TrimSpace(s); s == "" {
		return nil
	}
	return &s
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// providerError maps AI failures to retryable HTTP errors. Nothing has been
// stored at this point, so the volunteer can simply resubmit.
func providerError(err error) error {
	if errors.Is(err, ai.ErrRateLimited) {
		return apperrors.Upstream(http.StatusServiceUnavailable, "Layanan AI sedang sibuk. Silakan coba lagi sebentar lagi.", err)
	}
	if errors.Is(err, ai.ErrUnavailable) {
		return apperrors.Upstream(http.StatusServiceUnavailable, "Layanan AI sedang tidak tersedia. Silakan coba lagi.", err)
	}
	return apperrors.Upstream(http.StatusBadGateway, "AI gagal memproses permintaan. Silakan coba lagi.", err)
}
