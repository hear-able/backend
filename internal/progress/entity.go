package progress

import "time"

const (
	// WeeklyTargetDays is the "Target 5 Hari" goal shown on the dashboard.
	WeeklyTargetDays = 5

	TypeListening = "listening"
	TypeSpeaking  = "speaking"

	defaultHistoryLimit = 20
	maxHistoryLimit     = 50
)

type Day struct {
	Date     string `json:"date"` // YYYY-MM-DD in the app timezone
	Weekday  string `json:"weekday"`
	Sessions int    `json:"sessions"`
	Active   bool   `json:"active"`
}

type ByType struct {
	Listening int `json:"listening"`
	Speaking  int `json:"speaking"`
}

type Activity struct {
	Timezone          string `json:"timezone"`
	WeekStart         string `json:"week_start"`
	WeekEnd           string `json:"week_end"`
	Days              []Day  `json:"days"`
	ActiveDays        int    `json:"active_days"`
	TargetDays        int    `json:"target_days"`
	CompletedSessions int    `json:"completed_sessions"`
	ByType            ByType `json:"by_type"`
	StreakDays        int    `json:"streak_days"`
}

type completion struct {
	Type        string
	CompletedAt time.Time
}

type HistoryProgress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

type HistoryItem struct {
	Type         string          `json:"type"`
	SessionID    string          `json:"session_id"`
	ModuleID     string          `json:"module_id"`
	ModuleTitle  string          `json:"module_title"`
	Difficulty   int             `json:"difficulty"`
	DiffLabel    string          `json:"difficulty_label"`
	Status       string          `json:"status"`
	StartedAt    time.Time       `json:"started_at"`
	CompletedAt  *time.Time      `json:"completed_at"`
	Progress     HistoryProgress `json:"progress"`
	Correct      *int            `json:"correct"`
	AverageScore *int            `json:"average_score"`
}

type History struct {
	Items      []HistoryItem `json:"items"`
	NextCursor *string       `json:"next_cursor"`
}

type HistoryQuery struct {
	Limit  int
	Cursor string
	Status string
}

type cursor struct {
	StartedAt time.Time
	ID        string
}
