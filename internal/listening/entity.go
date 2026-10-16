package listening

import (
	"errors"
	"time"
)

const (
	QuestionTypeChoice = "choice"
	QuestionTypeInput  = "input"

	SessionInProgress = "in_progress"
	SessionCompleted  = "completed"

	AnswerEvaluating       = "evaluating"
	AnswerAnswered         = "answered"
	AnswerSkipped          = "skipped"
	AnswerEvaluationFailed = "evaluation_failed"

	// PassingScore is the minimum AI score for an input answer to count as
	// correct in the "X dari Y benar" summary.
	PassingScore = 70

	maxAnswerRunes = 2000
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAnswerExists  = errors.New("answer already exists")
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
	QuestionCount    int    `json:"question_count"`
}

type Audio struct {
	URL        string  `json:"url"`
	DurationMs *int    `json:"duration_ms"`
	HintText   *string `json:"hint_text"`
}

type Option struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Question is the client-facing view: it never includes the correct option
// or the expected answer.
type Question struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Prompt   string   `json:"prompt"`
	Position int      `json:"position"`
	Audio    *Audio   `json:"audio"`
	Options  []Option `json:"options,omitempty"`
}

// questionKey is the trusted grading data loaded from the database.
type questionKey struct {
	ID               string
	Type             string
	Prompt           string
	AudioDescription string
	ExpectedAnswer   string
	Explanation      string
	CorrectOptionID  string
	OptionIDs        map[string]bool
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
	MainIdea  int `json:"main_idea"`
	Details   int `json:"details"`
	Relevance int `json:"relevance"`
}

type Feedback struct {
	Criteria     *Criteria `json:"criteria,omitempty"`
	Explanation  string    `json:"explanation"`
	Strengths    []string  `json:"strengths"`
	MissedPoints []string  `json:"missed_points"`
	Suggestions  []string  `json:"suggestions"`
}

type Answer struct {
	QuestionID       string    `json:"question_id"`
	Type             string    `json:"type"`
	Status           string    `json:"status"`
	SelectedOptionID *string   `json:"selected_option_id"`
	AnswerText       *string   `json:"answer_text"`
	IsCorrect        *bool     `json:"is_correct"`
	Score            *int      `json:"score"`
	CorrectOptionID  *string   `json:"correct_option_id,omitempty"`
	Feedback         *Feedback `json:"feedback"`
	AnsweredAt       time.Time `json:"answered_at"`
}

type Progress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

type SessionDetail struct {
	Session   Session    `json:"session"`
	Progress  Progress   `json:"progress"`
	Questions []Question `json:"questions"`
	Answers   []Answer   `json:"answers"`
}

type Summary struct {
	Total    int `json:"total"`
	Answered int `json:"answered"`
	Skipped  int `json:"skipped"`
	Correct  int `json:"correct"`
}

type CompleteResponse struct {
	Session Session `json:"session"`
	Summary Summary `json:"summary"`
}

type StartSessionRequest struct {
	ModuleID string `json:"module_id"`
}

type SubmitAnswerRequest struct {
	QuestionID       string  `json:"question_id"`
	SelectedOptionID *string `json:"selected_option_id"`
	AnswerText       *string `json:"answer_text"`
	Skipped          bool    `json:"skipped"`
}

// answerRecord is what gets written to listening_session_answers.
type answerRecord struct {
	SessionID        string
	QuestionID       string
	Status           string
	SelectedOptionID *string
	AnswerText       *string
	IsCorrect        *bool
	Score            *int
	Feedback         *Feedback
	AIModel          *string
}
