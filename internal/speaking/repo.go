package speaking

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
		return nil, fmt.Errorf("list speaking modules: %w", err)
	}
	defer rows.Close()

	modules := []Module{}
	for rows.Next() {
		var m Module
		if err := rows.Scan(&m.ID, &m.Label, &m.Title, &m.Description, &m.Difficulty, &m.EstimatedMinutes, &m.SentenceCount); err != nil {
			return nil, fmt.Errorf("scan speaking module: %w", err)
		}
		modules = append(modules, m)
	}
	return modules, rows.Err()
}

func (r *Repository) GetModule(ctx context.Context, id string) (Module, error) {
	var m Module
	err := r.db.QueryRowContext(ctx, queryGetModule, id).
		Scan(&m.ID, &m.Label, &m.Title, &m.Description, &m.Difficulty, &m.EstimatedMinutes, &m.SentenceCount)
	if errors.Is(err, sql.ErrNoRows) {
		return Module{}, ErrNotFound
	}
	if err != nil {
		return Module{}, fmt.Errorf("get speaking module: %w", err)
	}
	return m, nil
}

func (r *Repository) ListSentences(ctx context.Context, moduleID string) ([]Sentence, error) {
	rows, err := r.db.QueryContext(ctx, queryListSentences, moduleID)
	if err != nil {
		return nil, fmt.Errorf("list sentences: %w", err)
	}
	defer rows.Close()

	sentences := []Sentence{}
	for rows.Next() {
		var (
			s         Sentence
			syllables []byte
		)
		if err := rows.Scan(&s.ID, &s.Text, &syllables, &s.Position); err != nil {
			return nil, fmt.Errorf("scan sentence: %w", err)
		}
		s.Syllables = json.RawMessage(syllables)
		sentences = append(sentences, s)
	}
	return sentences, rows.Err()
}

func (r *Repository) GetSentenceKey(ctx context.Context, moduleID, sentenceID string) (sentenceKey, error) {
	var k sentenceKey
	err := r.db.QueryRowContext(ctx, queryGetSentenceKey, moduleID, sentenceID).Scan(&k.ID, &k.Text, &k.Position)
	if errors.Is(err, sql.ErrNoRows) {
		return sentenceKey{}, ErrNotFound
	}
	if err != nil {
		return sentenceKey{}, fmt.Errorf("get sentence: %w", err)
	}
	return k, nil
}

func (r *Repository) CreateSession(ctx context.Context, userID, moduleID string) (string, error) {
	var id string
	if err := r.db.QueryRowContext(ctx, queryInsertSession, userID, moduleID).Scan(&id); err != nil {
		return "", fmt.Errorf("insert speaking session: %w", err)
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
		return Session{}, fmt.Errorf("get speaking session: %w", err)
	}
	if completedAt.Valid {
		s.CompletedAt = &completedAt.Time
	}
	return s, nil
}

func (r *Repository) ListAttempts(ctx context.Context, sessionID string) ([]Attempt, error) {
	rows, err := r.db.QueryContext(ctx, queryListAttempts, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list attempts: %w", err)
	}
	defer rows.Close()

	attempts := []Attempt{}
	for rows.Next() {
		var (
			a           Attempt
			transcript  sql.NullString
			score       sql.NullInt16
			explanation sql.NullString
			feedback    []byte
		)
		if err := rows.Scan(&a.SentenceID, &a.Status, &transcript, &score, &explanation, &feedback, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan attempt: %w", err)
		}
		if transcript.Valid {
			a.Transcript = &transcript.String
		}
		if explanation.Valid {
			a.Explanation = &explanation.String
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
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}

func (r *Repository) ClaimAttempt(ctx context.Context, sessionID, sentenceID string, position int, status string) error {
	var createdAt time.Time
	err := r.db.QueryRowContext(ctx, queryClaimAttempt, sessionID, sentenceID, position, status).Scan(&createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAttemptExists
	}
	if err != nil {
		return fmt.Errorf("claim attempt: %w", err)
	}
	return nil
}

func (r *Repository) FinalizeAttempt(ctx context.Context, rec attemptRecord) (time.Time, error) {
	var feedback *string
	if rec.Feedback != nil {
		b, err := json.Marshal(rec.Feedback)
		if err != nil {
			return time.Time{}, fmt.Errorf("encode feedback: %w", err)
		}
		s := string(b)
		feedback = &s
	}
	var createdAt time.Time
	err := r.db.QueryRowContext(ctx, queryFinalizeAttempt,
		rec.SessionID, rec.SentenceID, rec.Status, rec.Transcript, rec.Score, rec.Explanation, feedback, rec.AIModel,
	).Scan(&createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, ErrAttemptExists
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("finalize attempt: %w", err)
	}
	return createdAt, nil
}

// ReleaseAttempt removes an in-flight claim after a failed evaluation so the
// user can record again.
func (r *Repository) ReleaseAttempt(ctx context.Context, sessionID, sentenceID string) error {
	if _, err := r.db.ExecContext(ctx, queryReleaseAttempt, sessionID, sentenceID); err != nil {
		return fmt.Errorf("release attempt: %w", err)
	}
	return nil
}

func (r *Repository) CompleteSession(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, queryCompleteSession, id)
	if err != nil {
		return fmt.Errorf("complete speaking session: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotInProgress
	}
	return nil
}

func (r *Repository) Summary(ctx context.Context, sessionID, moduleID string) (Summary, error) {
	var s Summary
	var avg sql.NullInt32
	err := r.db.QueryRowContext(ctx, querySummary, sessionID, moduleID).
		Scan(&s.Total, &s.Evaluated, &s.Skipped, &avg)
	if err != nil {
		return Summary{}, fmt.Errorf("speaking summary: %w", err)
	}
	if avg.Valid {
		v := int(avg.Int32)
		s.AverageScore = &v
	}
	return s, nil
}
