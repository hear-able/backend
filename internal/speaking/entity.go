package speaking

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	SessionInProgress = "in_progress"
	SessionCompleted  = "completed"

	AttemptEvaluating = "evaluating"
	AttemptEvaluated  = "evaluated"
	AttemptSkipped    = "skipped"

	// MaxAudioBytes bounds uploaded recordings.
	MaxAudioBytes = 10 << 20
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAttemptExists = errors.New("attempt already exists")
	ErrNotInProgress = errors.New("session not in progress")
)

type Module struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Difficulty       int    `json:"difficulty"`
	DifficultyLabel  string `json:"difficulty_label"`
	EstimatedMinutes int    `json:"estimated_minutes"`
	SentenceCount    int    `json:"sentence_count"`
}

type Sentence struct {
	ID        string          `json:"id"`
	Text      string          `json:"text"`
	Syllables json.RawMessage `json:"syllables"`
	Position  int             `json:"position"`
}

type Session struct {
	ID              string     `json:"id"`
	ModuleID        string     `json:"module_id"`
	ModuleTitle     string     `json:"module_title"`
	Difficulty      int        `json:"difficulty"`
	DifficultyLabel string     `json:"difficulty_label"`
	Status          string     `json:"status"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`

	UserID string `json:"-"`
}

type Criteria struct {
	Kejelasan   int `json:"kejelasan"`
	Kelengkapan int `json:"kelengkapan"`
}

type Feedback struct {
	Criteria     Criteria `json:"criteria"`
	Strengths    []string `json:"strengths"`
	Improvements []string `json:"improvements"`
	Suggestions  []string `json:"suggestions"`
}

type Attempt struct {
	SentenceID  string    `json:"sentence_id"`
	Status      string    `json:"status"`
	Transcript  *string   `json:"transcript"`
	Score       *int      `json:"score"`
	Explanation *string   `json:"explanation"`
	Feedback    *Feedback `json:"feedback"`
	CreatedAt   time.Time `json:"created_at"`
}

type Progress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

type SessionDetail struct {
	Session   Session    `json:"session"`
	Progress  Progress   `json:"progress"`
	Sentences []Sentence `json:"sentences"`
	Attempts  []Attempt  `json:"attempts"`
}

type Summary struct {
	Total        int  `json:"total"`
	Evaluated    int  `json:"evaluated"`
	Skipped      int  `json:"skipped"`
	AverageScore *int `json:"average_score"`
}

type CompleteResponse struct {
	Session Session `json:"session"`
	Summary Summary `json:"summary"`
}

type StartSessionRequest struct {
	ModuleID string `json:"module_id"`
}

// AttemptInput is the parsed multipart submission.
type AttemptInput struct {
	SentenceID string
	Skipped    bool
	Audio      []byte
	Filename   string
}

type sentenceKey struct {
	ID       string
	Text     string
	Position int
}

type attemptRecord struct {
	SessionID   string
	SentenceID  string
	Position    int
	Status      string
	Transcript  *string
	Score       *int
	Explanation *string
	Feedback    *Feedback
	AIModel     *string
}
