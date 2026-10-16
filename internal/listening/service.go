package listening

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"hearable/backend/internal/ai"
	"hearable/backend/package/difficulty"
	apperrors "hearable/backend/package/errors"
	"hearable/backend/package/request"
)

type repository interface {
	ListModules(ctx context.Context, difficulty *int) ([]Module, error)
	GetModule(ctx context.Context, id string) (Module, error)
	ListQuestions(ctx context.Context, moduleID string) ([]Question, error)
	GetQuestionKey(ctx context.Context, moduleID, questionID string) (questionKey, error)
	CreateSession(ctx context.Context, userID, moduleID string) (string, error)
	GetSession(ctx context.Context, id string) (Session, error)
	ListAnswers(ctx context.Context, sessionID string) ([]Answer, error)
	ClaimAnswer(ctx context.Context, rec answerRecord) (time.Time, error)
	FinalizeAnswer(ctx context.Context, rec answerRecord) (time.Time, error)
	CompleteSession(ctx context.Context, id string) error
	Summary(ctx context.Context, sessionID, moduleID string) (Summary, error)
}

type evaluator interface {
	EvaluateListening(ctx context.Context, in ai.ListeningInput) (ai.ListeningEvaluation, error)
}

type Service struct {
	repo      repository
	evaluator evaluator
}

func NewService(repo repository, evaluator evaluator) *Service {
	return &Service{repo: repo, evaluator: evaluator}
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
	if module.QuestionCount == 0 {
		return SessionDetail{}, apperrors.Unprocessable("Modul ini belum memiliki soal.")
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
	questions, err := s.repo.ListQuestions(ctx, session.ModuleID)
	if err != nil {
		return SessionDetail{}, err
	}
	answers, err := s.repo.ListAnswers(ctx, sessionID)
	if err != nil {
		return SessionDetail{}, err
	}

	done := 0
	for _, a := range answers {
		if a.Status == AnswerAnswered || a.Status == AnswerSkipped {
			done++
		}
	}
	return SessionDetail{
		Session:   session,
		Progress:  Progress{Done: done, Total: len(questions)},
		Questions: questions,
		Answers:   answers,
	}, nil
}

func (s *Service) SubmitAnswer(ctx context.Context, userID, sessionID string, req SubmitAnswerRequest) (Answer, error) {
	session, err := s.ownedSession(ctx, userID, sessionID)
	if err != nil {
		return Answer{}, err
	}
	if session.Status != SessionInProgress {
		return Answer{}, apperrors.Conflict("Sesi sudah selesai.")
	}
	if !request.IsUUID(req.QuestionID) {
		return Answer{}, apperrors.Unprocessable("question_id tidak valid.")
	}
	key, err := s.repo.GetQuestionKey(ctx, session.ModuleID, req.QuestionID)
	if errors.Is(err, ErrNotFound) {
		return Answer{}, apperrors.Unprocessable("Soal tidak termasuk dalam sesi ini.")
	}
	if err != nil {
		return Answer{}, err
	}

	rec, err := buildRecord(sessionID, key, req)
	if err != nil {
		return Answer{}, err
	}

	answeredAt, err := s.repo.ClaimAnswer(ctx, rec)
	if errors.Is(err, ErrAnswerExists) {
		return Answer{}, apperrors.Conflict("Soal ini sudah dijawab atau sedang dinilai.")
	}
	if err != nil {
		return Answer{}, err
	}

	if rec.Status == AnswerEvaluating {
		rec, answeredAt, err = s.evaluateInput(ctx, key, rec)
		if err != nil {
			return Answer{}, err
		}
	}

	answer := Answer{
		QuestionID:       rec.QuestionID,
		Type:             key.Type,
		Status:           rec.Status,
		SelectedOptionID: rec.SelectedOptionID,
		AnswerText:       rec.AnswerText,
		IsCorrect:        rec.IsCorrect,
		Score:            rec.Score,
		Feedback:         rec.Feedback,
		AnsweredAt:       answeredAt,
	}
	if key.Type == QuestionTypeChoice && rec.Status == AnswerAnswered && key.CorrectOptionID != "" {
		correct := key.CorrectOptionID
		answer.CorrectOptionID = &correct
	}
	return answer, nil
}

// evaluateInput grades a claimed input answer with the AI. On provider
// failure the answer is kept as evaluation_failed so the client can resend
// the same request.
func (s *Service) evaluateInput(ctx context.Context, key questionKey, rec answerRecord) (answerRecord, time.Time, error) {
	eval, evalErr := s.evaluator.EvaluateListening(ctx, ai.ListeningInput{
		Prompt:           key.Prompt,
		AudioDescription: key.AudioDescription,
		ReferenceAnswer:  key.ExpectedAnswer,
		UserAnswer:       *rec.AnswerText,
	})
	if evalErr != nil {
		rec.Status = AnswerEvaluationFailed
		// Use a fresh context: the request context may be what timed out.
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := s.repo.FinalizeAnswer(saveCtx, rec); err != nil {
			return rec, time.Time{}, errors.Join(evalErr, err)
		}
		return rec, time.Time{}, providerError(evalErr)
	}

	score := eval.Score
	correct := score >= PassingScore
	model := eval.Model
	rec.Status = AnswerAnswered
	rec.Score = &score
	rec.IsCorrect = &correct
	rec.AIModel = &model
	rec.Feedback = &Feedback{
		Criteria: &Criteria{
			MainIdea:  eval.Criteria.MainIdea,
			Details:   eval.Criteria.Details,
			Relevance: eval.Criteria.Relevance,
		},
		Explanation:  eval.Explanation,
		Strengths:    eval.Strengths,
		MissedPoints: eval.MissedPoints,
		Suggestions:  eval.Suggestions,
	}
	answeredAt, err := s.repo.FinalizeAnswer(ctx, rec)
	if err != nil {
		return rec, time.Time{}, err
	}
	return rec, answeredAt, nil
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
	// Unanswered and failed questions count as skipped in the result.
	summary.Skipped = summary.Total - summary.Answered
	session.DifficultyLabel = difficulty.Label(session.Difficulty)
	return CompleteResponse{Session: session, Summary: summary}, nil
}

// ownedSession loads a session and hides other users' sessions behind 404.
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

func buildRecord(sessionID string, key questionKey, req SubmitAnswerRequest) (answerRecord, error) {
	rec := answerRecord{SessionID: sessionID, QuestionID: key.ID}
	hasOption := req.SelectedOptionID != nil
	hasText := req.AnswerText != nil

	if req.Skipped {
		if hasOption || hasText {
			return rec, apperrors.Unprocessable("Jawaban yang dilewati tidak boleh berisi jawaban.")
		}
		f := false
		rec.Status = AnswerSkipped
		rec.IsCorrect = &f
		return rec, nil
	}

	switch key.Type {
	case QuestionTypeChoice:
		if !hasOption || hasText {
			return rec, apperrors.Unprocessable("Soal pilihan ganda membutuhkan selected_option_id.")
		}
		if !key.OptionIDs[*req.SelectedOptionID] {
			return rec, apperrors.Unprocessable("Pilihan jawaban tidak valid untuk soal ini.")
		}
		correct := *req.SelectedOptionID == key.CorrectOptionID
		score := 0
		if correct {
			score = 100
		}
		rec.Status = AnswerAnswered
		rec.SelectedOptionID = req.SelectedOptionID
		rec.IsCorrect = &correct
		rec.Score = &score
		rec.Feedback = &Feedback{Explanation: key.Explanation, Strengths: []string{}, MissedPoints: []string{}, Suggestions: []string{}}
		return rec, nil

	case QuestionTypeInput:
		if !hasText || hasOption {
			return rec, apperrors.Unprocessable("Soal isian membutuhkan answer_text.")
		}
		text := strings.TrimSpace(*req.AnswerText)
		if text == "" {
			return rec, apperrors.Unprocessable("Jawaban tidak boleh kosong.")
		}
		if utf8.RuneCountInString(text) > maxAnswerRunes {
			return rec, apperrors.Unprocessable("Jawaban maksimal 2000 karakter.")
		}
		rec.Status = AnswerEvaluating
		rec.AnswerText = &text
		return rec, nil
	}
	return rec, errors.New("unknown question type " + key.Type)
}

func providerError(err error) error {
	if errors.Is(err, ai.ErrRateLimited) {
		return apperrors.Upstream(http.StatusServiceUnavailable, "Layanan penilaian sedang sibuk. Jawabanmu tersimpan, silakan kirim ulang sebentar lagi.", err)
	}
	if errors.Is(err, ai.ErrUnavailable) {
		return apperrors.Upstream(http.StatusServiceUnavailable, "Layanan penilaian sedang tidak tersedia. Jawabanmu tersimpan, silakan kirim ulang.", err)
	}
	return apperrors.Upstream(http.StatusBadGateway, "Penilaian gagal diproses. Jawabanmu tersimpan, silakan kirim ulang.", err)
}
