package listening

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"hearable/backend/internal/ai"
	apperrors "hearable/backend/package/errors"
)

const (
	userID    = "aaaaaaaa-0000-0000-0000-000000000001"
	otherUser = "aaaaaaaa-0000-0000-0000-000000000002"
	sessionID = "bbbbbbbb-0000-0000-0000-000000000001"
	moduleID  = "cccccccc-0000-0000-0000-000000000001"
	choiceQ   = "dddddddd-0000-0000-0000-000000000001"
	inputQ    = "dddddddd-0000-0000-0000-000000000002"
	optRight  = "eeeeeeee-0000-0000-0000-000000000001"
	optWrong  = "eeeeeeee-0000-0000-0000-000000000002"
)

type fakeRepo struct {
	session   Session
	keys      map[string]questionKey
	claimErr  error
	claimed   []answerRecord
	finalized []answerRecord
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		session: Session{ID: sessionID, UserID: userID, ModuleID: moduleID, Status: SessionInProgress, Difficulty: 1},
		keys: map[string]questionKey{
			choiceQ: {ID: choiceQ, Type: QuestionTypeChoice, CorrectOptionID: optRight, Explanation: "Bel pintu",
				OptionIDs: map[string]bool{optRight: true, optWrong: true}},
			inputQ: {ID: inputQ, Type: QuestionTypeInput, ExpectedAnswer: "Selamat pagi", OptionIDs: map[string]bool{}},
		},
	}
}

func (f *fakeRepo) ListModules(context.Context, *int) ([]Module, error) { return nil, nil }
func (f *fakeRepo) GetModule(context.Context, string) (Module, error)   { return Module{}, nil }
func (f *fakeRepo) ListQuestions(context.Context, string) ([]Question, error) {
	return nil, nil
}
func (f *fakeRepo) GetQuestionKey(_ context.Context, _ string, qid string) (questionKey, error) {
	k, ok := f.keys[qid]
	if !ok {
		return questionKey{}, ErrNotFound
	}
	return k, nil
}
func (f *fakeRepo) CreateSession(context.Context, string, string) (string, error) { return "", nil }
func (f *fakeRepo) GetSession(_ context.Context, id string) (Session, error) {
	if id != f.session.ID {
		return Session{}, ErrNotFound
	}
	return f.session, nil
}
func (f *fakeRepo) ListAnswers(context.Context, string) ([]Answer, error) { return nil, nil }
func (f *fakeRepo) ClaimAnswer(_ context.Context, rec answerRecord) (time.Time, error) {
	if f.claimErr != nil {
		return time.Time{}, f.claimErr
	}
	f.claimed = append(f.claimed, rec)
	return time.Now(), nil
}
func (f *fakeRepo) FinalizeAnswer(_ context.Context, rec answerRecord) (time.Time, error) {
	f.finalized = append(f.finalized, rec)
	return time.Now(), nil
}
func (f *fakeRepo) CompleteSession(context.Context, string) error { return nil }
func (f *fakeRepo) Summary(context.Context, string, string) (Summary, error) {
	return Summary{Total: 4, Answered: 2, Skipped: 1, Correct: 1}, nil
}

type fakeEvaluator struct {
	result ai.ListeningEvaluation
	err    error
	calls  int
	input  ai.ListeningInput
}

func (f *fakeEvaluator) EvaluateListening(_ context.Context, in ai.ListeningInput) (ai.ListeningEvaluation, error) {
	f.calls++
	f.input = in
	return f.result, f.err
}

func strp(s string) *string { return &s }

func wantStatus(t *testing.T, err error, status int) *apperrors.AppError {
	t.Helper()
	appErr, ok := apperrors.As(err)
	if !ok || appErr.Status != status {
		t.Fatalf("want status %d, got %v", status, err)
	}
	return appErr
}

func TestSubmitChoiceAnswer(t *testing.T) {
	repo, eval := newFakeRepo(), &fakeEvaluator{}
	svc := NewService(repo, eval)

	ans, err := svc.SubmitAnswer(context.Background(), userID, sessionID, SubmitAnswerRequest{QuestionID: choiceQ, SelectedOptionID: strp(optWrong)})
	if err != nil {
		t.Fatal(err)
	}
	if *ans.IsCorrect || *ans.Score != 0 || *ans.CorrectOptionID != optRight || ans.Feedback.Explanation != "Bel pintu" {
		t.Fatalf("unexpected answer %+v", ans)
	}
	if eval.calls != 0 {
		t.Fatal("choice questions must not call the AI")
	}
	if len(repo.claimed) != 1 || repo.claimed[0].Status != AnswerAnswered {
		t.Fatalf("claimed %+v", repo.claimed)
	}
}

func TestSubmitValidation(t *testing.T) {
	cases := map[string]SubmitAnswerRequest{
		"unknown option":       {QuestionID: choiceQ, SelectedOptionID: strp("eeeeeeee-0000-0000-0000-000000000099")},
		"text for choice":      {QuestionID: choiceQ, AnswerText: strp("x")},
		"option for input":     {QuestionID: inputQ, SelectedOptionID: strp(optRight)},
		"blank text":           {QuestionID: inputQ, AnswerText: strp("   ")},
		"skip with answer":     {QuestionID: inputQ, AnswerText: strp("x"), Skipped: true},
		"question not in mod":  {QuestionID: "dddddddd-0000-0000-0000-000000000099", Skipped: true},
		"malformed questionID": {QuestionID: "nope", Skipped: true},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			svc := NewService(newFakeRepo(), &fakeEvaluator{})
			_, err := svc.SubmitAnswer(context.Background(), userID, sessionID, req)
			wantStatus(t, err, http.StatusUnprocessableEntity)
		})
	}
}

func TestSubmitInputAnswerUsesTrustedReference(t *testing.T) {
	repo := newFakeRepo()
	eval := &fakeEvaluator{result: ai.ListeningEvaluation{Score: 85, Explanation: "Tepat", Model: "m"}}
	svc := NewService(repo, eval)

	ans, err := svc.SubmitAnswer(context.Background(), userID, sessionID, SubmitAnswerRequest{QuestionID: inputQ, AnswerText: strp("  selamat pagi ")})
	if err != nil {
		t.Fatal(err)
	}
	if eval.input.ReferenceAnswer != "Selamat pagi" || eval.input.UserAnswer != "selamat pagi" {
		t.Fatalf("evaluator input %+v", eval.input)
	}
	if !*ans.IsCorrect || *ans.Score != 85 || ans.Status != AnswerAnswered || ans.Feedback.Explanation != "Tepat" {
		t.Fatalf("answer %+v", ans)
	}
	if repo.claimed[0].Status != AnswerEvaluating || repo.finalized[0].Status != AnswerAnswered {
		t.Fatalf("claim/finalize statuses %s/%s", repo.claimed[0].Status, repo.finalized[0].Status)
	}
}

func TestSubmitInputBelowPassingScoreIsIncorrect(t *testing.T) {
	eval := &fakeEvaluator{result: ai.ListeningEvaluation{Score: PassingScore - 1, Explanation: "x"}}
	ans, err := NewService(newFakeRepo(), eval).SubmitAnswer(context.Background(), userID, sessionID, SubmitAnswerRequest{QuestionID: inputQ, AnswerText: strp("x")})
	if err != nil || *ans.IsCorrect {
		t.Fatalf("got %+v, %v", ans, err)
	}
}

func TestSubmitInputAIFailureIsRetryable(t *testing.T) {
	repo := newFakeRepo()
	eval := &fakeEvaluator{err: ai.ErrRateLimited}
	_, err := NewService(repo, eval).SubmitAnswer(context.Background(), userID, sessionID, SubmitAnswerRequest{QuestionID: inputQ, AnswerText: strp("x")})
	appErr := wantStatus(t, err, http.StatusServiceUnavailable)
	if !appErr.Retryable {
		t.Fatal("expected retryable error")
	}
	if len(repo.finalized) != 1 || repo.finalized[0].Status != AnswerEvaluationFailed || repo.finalized[0].Score != nil {
		t.Fatalf("failed answer must be stored without a score: %+v", repo.finalized)
	}
}

func TestSubmitConflictsAndOwnership(t *testing.T) {
	repo := newFakeRepo()
	repo.claimErr = ErrAnswerExists
	eval := &fakeEvaluator{}
	_, err := NewService(repo, eval).SubmitAnswer(context.Background(), userID, sessionID, SubmitAnswerRequest{QuestionID: inputQ, AnswerText: strp("x")})
	wantStatus(t, err, http.StatusConflict)
	if eval.calls != 0 {
		t.Fatal("AI must not be called when the answer already exists")
	}

	repo = newFakeRepo()
	repo.session.Status = SessionCompleted
	_, err = NewService(repo, eval).SubmitAnswer(context.Background(), userID, sessionID, SubmitAnswerRequest{QuestionID: choiceQ, Skipped: true})
	wantStatus(t, err, http.StatusConflict)

	_, err = NewService(newFakeRepo(), eval).SubmitAnswer(context.Background(), otherUser, sessionID, SubmitAnswerRequest{QuestionID: choiceQ, Skipped: true})
	wantStatus(t, err, http.StatusNotFound)

	_, err = NewService(newFakeRepo(), eval).GetSession(context.Background(), userID, "not-a-uuid")
	wantStatus(t, err, http.StatusNotFound)
}

func TestCompleteSessionCountsUnansweredAsSkipped(t *testing.T) {
	resp, err := NewService(newFakeRepo(), &fakeEvaluator{}).CompleteSession(context.Background(), userID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Summary.Skipped != 2 || resp.Summary.Correct != 1 || resp.Session.DifficultyLabel != "Pemula" {
		t.Fatalf("summary %+v", resp)
	}
}

func TestListModulesRejectsBadDifficulty(t *testing.T) {
	bad := 5
	_, err := NewService(newFakeRepo(), &fakeEvaluator{}).ListModules(context.Background(), &bad)
	if !errors.As(err, new(*apperrors.AppError)) {
		t.Fatalf("got %v", err)
	}
}
