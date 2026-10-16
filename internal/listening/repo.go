package listening

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListModules(ctx context.Context, difficulty *int) ([]Module, error) {
	rows, err := r.db.QueryContext(ctx, queryListModules, difficulty)
	if err != nil {
		return nil, fmt.Errorf("list listening modules: %w", err)
	}
	defer rows.Close()

	modules := []Module{}
	for rows.Next() {
		var m Module
		if err := rows.Scan(&m.ID, &m.Label, &m.Title, &m.Description, &m.Difficulty, &m.EstimatedMinutes, &m.QuestionCount); err != nil {
			return nil, fmt.Errorf("scan listening module: %w", err)
		}
		modules = append(modules, m)
	}
	return modules, rows.Err()
}

func (r *Repository) GetModule(ctx context.Context, id string) (Module, error) {
	var m Module
	err := r.db.QueryRowContext(ctx, queryGetModule, id).
		Scan(&m.ID, &m.Label, &m.Title, &m.Description, &m.Difficulty, &m.EstimatedMinutes, &m.QuestionCount)
	if errors.Is(err, sql.ErrNoRows) {
		return Module{}, ErrNotFound
	}
	if err != nil {
		return Module{}, fmt.Errorf("get listening module: %w", err)
	}
	return m, nil
}

func (r *Repository) ListQuestions(ctx context.Context, moduleID string) ([]Question, error) {
	rows, err := r.db.QueryContext(ctx, queryListQuestions, moduleID)
	if err != nil {
		return nil, fmt.Errorf("list questions: %w", err)
	}
	defer rows.Close()

	questions := []Question{}
	index := map[string]int{}
	for rows.Next() {
		var (
			q        Question
			url      sql.NullString
			duration sql.NullInt32
			hint     sql.NullString
		)
		if err := rows.Scan(&q.ID, &q.Type, &q.Prompt, &q.Position, &url, &duration, &hint); err != nil {
			return nil, fmt.Errorf("scan question: %w", err)
		}
		if url.Valid {
			q.Audio = &Audio{URL: url.String}
			if duration.Valid {
				d := int(duration.Int32)
				q.Audio.DurationMs = &d
			}
			if hint.Valid {
				q.Audio.HintText = &hint.String
			}
		}
		index[q.ID] = len(questions)
		questions = append(questions, q)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	optRows, err := r.db.QueryContext(ctx, queryListModuleOptions, moduleID)
	if err != nil {
		return nil, fmt.Errorf("list options: %w", err)
	}
	defer optRows.Close()
	for optRows.Next() {
		var o Option
		var questionID string
		if err := optRows.Scan(&o.ID, &questionID, &o.Text); err != nil {
			return nil, fmt.Errorf("scan option: %w", err)
		}
		if i, ok := index[questionID]; ok {
			questions[i].Options = append(questions[i].Options, o)
		}
	}
	return questions, optRows.Err()
}

func (r *Repository) GetQuestionKey(ctx context.Context, moduleID, questionID string) (questionKey, error) {
	var k questionKey
	err := r.db.QueryRowContext(ctx, queryGetQuestionKey, moduleID, questionID).
		Scan(&k.ID, &k.Type, &k.Prompt, &k.AudioDescription, &k.ExpectedAnswer, &k.Explanation)
	if errors.Is(err, sql.ErrNoRows) {
		return questionKey{}, ErrNotFound
	}
	if err != nil {
		return questionKey{}, fmt.Errorf("get question key: %w", err)
	}

	k.OptionIDs = map[string]bool{}
	if k.Type != QuestionTypeChoice {
		return k, nil
	}
	rows, err := r.db.QueryContext(ctx, queryGetQuestionOptions, questionID)
	if err != nil {
		return questionKey{}, fmt.Errorf("get question options: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var correct bool
		if err := rows.Scan(&id, &correct); err != nil {
			return questionKey{}, fmt.Errorf("scan question option: %w", err)
		}
		k.OptionIDs[id] = true
		if correct {
			k.CorrectOptionID = id
		}
	}
	return k, rows.Err()
}

func (r *Repository) CreateSession(ctx context.Context, userID, moduleID string) (string, error) {
	var id string
	if err := r.db.QueryRowContext(ctx, queryInsertSession, userID, moduleID).Scan(&id); err != nil {
		return "", fmt.Errorf("insert listening session: %w", err)
	}
	return id, nil
}

func (r *Repository) GetSession(ctx context.Context, id string) (Session, error) {
	var s Session
	var completedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, queryGetSession, id).
		Scan(&s.ID, &s.UserID, &s.ModuleID, &s.ModuleTitle, &s.Difficulty, &s.Status, &s.StartedAt, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("get listening session: %w", err)
	}
	if completedAt.Valid {
		s.CompletedAt = &completedAt.Time
	}
	return s, nil
}

func (r *Repository) ListAnswers(ctx context.Context, sessionID string) ([]Answer, error) {
	rows, err := r.db.QueryContext(ctx, queryListAnswers, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list answers: %w", err)
	}
	defer rows.Close()

	answers := []Answer{}
	for rows.Next() {
		var (
			a         Answer
			selected  sql.NullString
			text      sql.NullString
			isCorrect sql.NullBool
			score     sql.NullInt16
			feedback  []byte
			correctID sql.NullString
		)
		if err := rows.Scan(&a.QuestionID, &a.Status, &selected, &text, &isCorrect, &score, &feedback, &a.AnsweredAt, &correctID); err != nil {
			return nil, fmt.Errorf("scan answer: %w", err)
		}
		a.SelectedOptionID = nullString(selected)
		a.AnswerText = nullString(text)
		a.CorrectOptionID = nullString(correctID)
		if isCorrect.Valid {
			a.IsCorrect = &isCorrect.Bool
		}
		if score.Valid {
			s := int(score.Int16)
			a.Score = &s
		}
		if len(feedback) > 0 {
			var fb Feedback
			if err := json.Unmarshal(feedback, &fb); err != nil {
				return nil, fmt.Errorf("decode feedback: %w", err)
			}
			a.Feedback = &fb
		}
		answers = append(answers, a)
	}
	return answers, rows.Err()
}

// ClaimAnswer writes rec unless a final answer already exists for the
// question (ErrAnswerExists).
func (r *Repository) ClaimAnswer(ctx context.Context, rec answerRecord) (time.Time, error) {
	feedback, err := encodeFeedback(rec.Feedback)
	if err != nil {
		return time.Time{}, err
	}
	var answeredAt time.Time
	err = r.db.QueryRowContext(ctx, queryClaimAnswer,
		rec.SessionID, rec.QuestionID, rec.Status, rec.SelectedOptionID, rec.AnswerText,
		rec.IsCorrect, rec.Score, feedback, rec.AIModel,
	).Scan(&answeredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, ErrAnswerExists
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("claim answer: %w", err)
	}
	return answeredAt, nil
}

// FinalizeAnswer stores the evaluation result for a claimed answer.
func (r *Repository) FinalizeAnswer(ctx context.Context, rec answerRecord) (time.Time, error) {
	feedback, err := encodeFeedback(rec.Feedback)
	if err != nil {
		return time.Time{}, err
	}
	var answeredAt time.Time
	err = r.db.QueryRowContext(ctx, queryFinalizeAnswer,
		rec.SessionID, rec.QuestionID, rec.Status, rec.IsCorrect, rec.Score, feedback, rec.AIModel,
	).Scan(&answeredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, ErrAnswerExists
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("finalize answer: %w", err)
	}
	return answeredAt, nil
}

func (r *Repository) CompleteSession(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, queryCompleteSession, id)
	if err != nil {
		return fmt.Errorf("complete listening session: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotInProgress
	}
	return nil
}

func (r *Repository) Summary(ctx context.Context, sessionID, moduleID string) (Summary, error) {
	var s Summary
	err := r.db.QueryRowContext(ctx, querySummary, sessionID, moduleID).
		Scan(&s.Total, &s.Answered, &s.Skipped, &s.Correct)
	if err != nil {
		return Summary{}, fmt.Errorf("listening summary: %w", err)
	}
	return s, nil
}

func encodeFeedback(fb *Feedback) (*string, error) {
	if fb == nil {
		return nil, nil
	}
	b, err := json.Marshal(fb)
	if err != nil {
		return nil, fmt.Errorf("encode feedback: %w", err)
	}
	s := string(b)
	return &s, nil
}

func nullString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}
