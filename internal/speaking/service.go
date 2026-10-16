package speaking

import (
	"context"
	"errors"
	"net/http"
	"time"

	"hearable/backend/internal/ai"
	"hearable/backend/package/difficulty"
	apperrors "hearable/backend/package/errors"
	"hearable/backend/package/request"
)

type repository interface {
	ListModules(ctx context.Context, difficulty *int) ([]Module, error)
	GetModule(ctx context.Context, id string) (Module, error)
	ListSentences(ctx context.Context, moduleID string) ([]Sentence, error)
	GetSentenceKey(ctx context.Context, moduleID, sentenceID string) (sentenceKey, error)
	CreateSession(ctx context.Context, userID, moduleID string) (string, error)
	GetSession(ctx context.Context, id string) (Session, error)
	ListAttempts(ctx context.Context, sessionID string) ([]Attempt, error)
	ClaimAttempt(ctx context.Context, sessionID, sentenceID string, position int, status string) error
	FinalizeAttempt(ctx context.Context, rec attemptRecord) (time.Time, error)
	ReleaseAttempt(ctx context.Context, sessionID, sentenceID string) error
	CompleteSession(ctx context.Context, id string) error
	Summary(ctx context.Context, sessionID, moduleID string) (Summary, error)
}

type speechEvaluator interface {
	Transcribe(ctx context.Context, audio []byte, filename string) (string, error)
	EvaluateSpeaking(ctx context.Context, in ai.SpeakingInput) (ai.SpeakingEvaluation, error)
}

type Service struct {
	repo repository
	ai   speechEvaluator
}

func NewService(repo repository, evaluator speechEvaluator) *Service {
	return &Service{repo: repo, ai: evaluator}
}

func (s *Service) ListModules(ctx context.Context, level *int) ([]Module, error) {
	if level != nil && !difficulty.Valid(*level) {
		return nil, apperrors.Unprocessable("difficulty harus bernilai 1 sampai 4.")
	}
	modules, err := s.repo.ListModules(ctx, level)
	if err != nil {
		return nil, err
	}
	for i := range modules {
		modules[i].DifficultyLabel = difficulty.Label(modules[i].Difficulty)
	}
	return modules, nil
}

func (s *Service) StartSession(ctx context.Context, userID, moduleID string) (SessionDetail, error) {
	if !request.IsUUID(moduleID) {
		return SessionDetail{}, apperrors.Unprocessable("module_id tidak valid.")
	}
	module, err := s.repo.GetModule(ctx, moduleID)
	if errors.Is(err, ErrNotFound) {
		return SessionDetail{}, apperrors.NotFound("Modul tidak ditemukan.")
	}
	if err != nil {
		return SessionDetail{}, err
	}
	if module.SentenceCount == 0 {
		return SessionDetail{}, apperrors.Unprocessable("Modul ini belum memiliki kalimat latihan.")
	}
	sessionID, err := s.repo.CreateSession(ctx, userID, moduleID)
	if err != nil {
		return SessionDetail{}, err
	}
	return s.GetSession(ctx, userID, sessionID)
}

func (s *Service) GetSession(ctx context.Context, userID, sessionID string) (SessionDetail, error) {
	session, err := s.ownedSession(ctx, userID, sessionID)
	if err != nil {
		return SessionDetail{}, err
	}
	sentences, err := s.repo.ListSentences(ctx, session.ModuleID)
	if err != nil {
		return SessionDetail{}, err
	}
	attempts, err := s.repo.ListAttempts(ctx, sessionID)
	if err != nil {
		return SessionDetail{}, err
	}
	return SessionDetail{
		Session:   session,
		Progress:  Progress{Done: len(attempts), Total: len(sentences)},
		Sentences: sentences,
		Attempts:  attempts,
	}, nil
}

// SubmitAttempt evaluates one recording (or records a skip). The audio is
// only held in memory for the duration of the request.
func (s *Service) SubmitAttempt(ctx context.Context, userID, sessionID string, in AttemptInput) (Attempt, error) {
	session, err := s.ownedSession(ctx, userID, sessionID)
	if err != nil {
		return Attempt{}, err
	}
	if session.Status != SessionInProgress {
		return Attempt{}, apperrors.Conflict("Sesi sudah selesai.")
	}
	if !request.IsUUID(in.SentenceID) {
		return Attempt{}, apperrors.Unprocessable("sentence_id tidak valid.")
	}
	key, err := s.repo.GetSentenceKey(ctx, session.ModuleID, in.SentenceID)
	if errors.Is(err, ErrNotFound) {
		return Attempt{}, apperrors.Unprocessable("Kalimat tidak termasuk dalam sesi ini.")
	}
	if err != nil {
		return Attempt{}, err
	}
	if !in.Skipped && len(in.Audio) == 0 {
		return Attempt{}, apperrors.Unprocessable("Rekaman suara wajib diunggah.")
	}

	status := AttemptEvaluating
	if in.Skipped {
		status = AttemptSkipped
	}
	err = s.repo.ClaimAttempt(ctx, sessionID, key.ID, key.Position, status)
	if errors.Is(err, ErrAttemptExists) {
		return Attempt{}, apperrors.Conflict("Kalimat ini sudah dikerjakan atau sedang dinilai.")
	}
	if err != nil {
		return Attempt{}, err
	}
	if in.Skipped {
		return Attempt{SentenceID: key.ID, Status: AttemptSkipped, CreatedAt: time.Now()}, nil
	}

	attempt, err := s.evaluate(ctx, sessionID, key, in)
	if err != nil {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if relErr := s.repo.ReleaseAttempt(releaseCtx, sessionID, key.ID); relErr != nil {
			return Attempt{}, errors.Join(err, relErr)
		}
		return Attempt{}, err
	}
	return attempt, nil
}

func (s *Service) evaluate(ctx context.Context, sessionID string, key sentenceKey, in AttemptInput) (Attempt, error) {
	transcript, err := s.ai.Transcribe(ctx, in.Audio, in.Filename)
	if err != nil {
		return Attempt{}, providerError(err)
	}
	if transcript == "" {
		return Attempt{}, apperrors.Unprocessable("Suara belum terdengar jelas dalam rekaman. Coba rekam lagi di tempat yang lebih tenang dan dekatkan mikrofon.")
	}

	eval, err := s.ai.EvaluateSpeaking(ctx, ai.SpeakingInput{TargetText: key.Text, Transcript: transcript})
	if err != nil {
		return Attempt{}, providerError(err)
	}

	score := eval.Score
	model := eval.Model
	explanation := eval.Explanation
	rec := attemptRecord{
		SessionID:   sessionID,
		SentenceID:  key.ID,
		Position:    key.Position,
		Status:      AttemptEvaluated,
		Transcript:  &transcript,
		Score:       &score,
		Explanation: &explanation,
		AIModel:     &model,
		Feedback: &Feedback{
			Criteria:     Criteria{Kejelasan: eval.Criteria.Kejelasan, Kelengkapan: eval.Criteria.Kelengkapan},
			Strengths:    eval.Strengths,
			Improvements: eval.Improvements,
			Suggestions:  eval.Suggestions,
		},
	}
	createdAt, err := s.repo.FinalizeAttempt(ctx, rec)
	if err != nil {
		return Attempt{}, err
	}
	return Attempt{
		SentenceID:  rec.SentenceID,
		Status:      rec.Status,
		Transcript:  rec.Transcript,
		Score:       rec.Score,
		Explanation: rec.Explanation,
		Feedback:    rec.Feedback,
		CreatedAt:   createdAt,
	}, nil
}

func (s *Service) CompleteSession(ctx context.Context, userID, sessionID string) (CompleteResponse, error) {
	session, err := s.ownedSession(ctx, userID, sessionID)
	if err != nil {
		return CompleteResponse{}, err
	}
	if session.Status == SessionInProgress {
		err := s.repo.CompleteSession(ctx, sessionID)
		if err != nil && !errors.Is(err, ErrNotInProgress) {
			return CompleteResponse{}, err
		}
		if session, err = s.repo.GetSession(ctx, sessionID); err != nil {
			return CompleteResponse{}, err
		}
	}
	summary, err := s.repo.Summary(ctx, sessionID, session.ModuleID)
	if err != nil {
		return CompleteResponse{}, err
	}
	summary.Skipped = summary.Total - summary.Evaluated
	session.DifficultyLabel = difficulty.Label(session.Difficulty)
	return CompleteResponse{Session: session, Summary: summary}, nil
}

func (s *Service) ownedSession(ctx context.Context, userID, sessionID string) (Session, error) {
	notFound := apperrors.NotFound("Sesi tidak ditemukan.")
	if !request.IsUUID(sessionID) {
		return Session{}, notFound
	}
	session, err := s.repo.GetSession(ctx, sessionID)
	if errors.Is(err, ErrNotFound) {
		return Session{}, notFound
	}
	if err != nil {
		return Session{}, err
	}
	if session.UserID != userID {
		return Session{}, notFound
	}
	session.DifficultyLabel = difficulty.Label(session.Difficulty)
	return session, nil
}

// providerError maps AI failures to retryable HTTP errors. Recordings are
// not stored, so the client must upload again.
func providerError(err error) error {
	if errors.Is(err, ai.ErrRateLimited) || errors.Is(err, ai.ErrUnavailable) {
		return apperrors.Upstream(http.StatusServiceUnavailable, "Layanan penilaian sedang sibuk. Silakan rekam dan kirim ulang sebentar lagi.", err)
	}
	return apperrors.Upstream(http.StatusBadGateway, "Penilaian gagal diproses. Silakan rekam dan kirim ulang.", err)
}
