package speaking

import (
	"context"
	"net/http"
	"testing"
	"time"

	"hearable/backend/internal/ai"
	apperrors "hearable/backend/package/errors"
)

const (
	userID     = "aaaaaaaa-0000-0000-0000-000000000001"
	sessionID  = "bbbbbbbb-0000-0000-0000-000000000001"
	moduleID   = "cccccccc-0000-0000-0000-000000000001"
	sentenceID = "dddddddd-0000-0000-0000-000000000001"
)

type fakeRepo struct {
	session   Session
	claimErr  error
	claims    []string
	finalized []attemptRecord
	released  int
}

func (f *fakeRepo) ListModules(context.Context, *int) ([]Module, error)       { return nil, nil }
func (f *fakeRepo) GetModule(context.Context, string) (Module, error)         { return Module{}, nil }
func (f *fakeRepo) ListSentences(context.Context, string) ([]Sentence, error) { return nil, nil }
func (f *fakeRepo) GetSentenceKey(_ context.Context, _ string, id string) (sentenceKey, error) {
	if id != sentenceID {
		return sentenceKey{}, ErrNotFound
	}
	return sentenceKey{ID: sentenceID, Text: "Selamat Pagi", Position: 0}, nil
}
func (f *fakeRepo) CreateSession(context.Context, string, string) (string, error) { return "", nil }
func (f *fakeRepo) GetSession(context.Context, string) (Session, error)           { return f.session, nil }
func (f *fakeRepo) ListAttempts(context.Context, string) ([]Attempt, error)       { return nil, nil }
func (f *fakeRepo) ClaimAttempt(_ context.Context, _, _ string, _ int, status string) error {
	if f.claimErr != nil {
		return f.claimErr
	}
	f.claims = append(f.claims, status)
	return nil
}
func (f *fakeRepo) FinalizeAttempt(_ context.Context, rec attemptRecord) (time.Time, error) {
	f.finalized = append(f.finalized, rec)
	return time.Now(), nil
}
func (f *fakeRepo) ReleaseAttempt(context.Context, string, string) error { f.released++; return nil }
func (f *fakeRepo) CompleteSession(context.Context, string) error        { return nil }
func (f *fakeRepo) Summary(context.Context, string, string) (Summary, error) {
	return Summary{}, nil
}

type fakeAI struct {
	transcript    string
	transcribeErr error
	eval          ai.SpeakingEvaluation
	evalCalls     int
}

func (f *fakeAI) Transcribe(context.Context, []byte, string) (string, error) {
	return f.transcript, f.transcribeErr
}
func (f *fakeAI) EvaluateSpeaking(context.Context, ai.SpeakingInput) (ai.SpeakingEvaluation, error) {
	f.evalCalls++
	return f.eval, nil
}

func newRepo() *fakeRepo {
	return &fakeRepo{session: Session{ID: sessionID, UserID: userID, ModuleID: moduleID, Status: SessionInProgress}}
}

func audioInput() AttemptInput {
	return AttemptInput{SentenceID: sentenceID, Audio: []byte("data"), Filename: "recording.webm"}
}

func TestSubmitAttemptSuccess(t *testing.T) {
	repo := newRepo()
	fake := &fakeAI{transcript: "selamat pagi", eval: ai.SpeakingEvaluation{Score: 92, Explanation: "Jelas", Model: "m"}}
	got, err := NewService(repo, fake).SubmitAttempt(context.Background(), userID, sessionID, audioInput())
	if err != nil {
		t.Fatal(err)
	}
	if *got.Score != 92 || *got.Transcript != "selamat pagi" || got.Status != AttemptEvaluated {
		t.Fatalf("attempt %+v", got)
	}
	if repo.claims[0] != AttemptEvaluating || len(repo.finalized) != 1 {
		t.Fatalf("claims=%v finalized=%d", repo.claims, len(repo.finalized))
	}
}

func TestSubmitAttemptEmptyTranscriptReleasesClaim(t *testing.T) {
	repo := newRepo()
	fake := &fakeAI{transcript: ""}
	_, err := NewService(repo, fake).SubmitAttempt(context.Background(), userID, sessionID, audioInput())
	if appErr, ok := apperrors.As(err); !ok || appErr.Status != http.StatusUnprocessableEntity {
		t.Fatalf("got %v", err)
	}
	if repo.released != 1 || len(repo.finalized) != 0 || fake.evalCalls != 0 {
		t.Fatalf("released=%d finalized=%d evalCalls=%d", repo.released, len(repo.finalized), fake.evalCalls)
	}
}

func TestSubmitAttemptProviderFailure(t *testing.T) {
	repo := newRepo()
	_, err := NewService(repo, &fakeAI{transcribeErr: ai.ErrUnavailable}).SubmitAttempt(context.Background(), userID, sessionID, audioInput())
	appErr, ok := apperrors.As(err)
	if !ok || appErr.Status != http.StatusServiceUnavailable || !appErr.Retryable {
		t.Fatalf("got %v", err)
	}
	if repo.released != 1 {
		t.Fatal("claim must be released so the user can record again")
	}
}

func TestSubmitAttemptConflictSkipsAI(t *testing.T) {
	repo := newRepo()
	repo.claimErr = ErrAttemptExists
	fake := &fakeAI{transcript: "x"}
	_, err := NewService(repo, fake).SubmitAttempt(context.Background(), userID, sessionID, audioInput())
	if appErr, ok := apperrors.As(err); !ok || appErr.Status != http.StatusConflict {
		t.Fatalf("got %v", err)
	}
	if fake.evalCalls != 0 {
		t.Fatal("AI must not be called on conflict")
	}
}

func TestSubmitAttemptSkip(t *testing.T) {
	repo := newRepo()
	got, err := NewService(repo, &fakeAI{}).SubmitAttempt(context.Background(), userID, sessionID, AttemptInput{SentenceID: sentenceID, Skipped: true})
	if err != nil || got.Status != AttemptSkipped || repo.claims[0] != AttemptSkipped {
		t.Fatalf("got %+v %v claims=%v", got, err, repo.claims)
	}
}
