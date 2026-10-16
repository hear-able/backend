package progress

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CompletionsBetween(ctx context.Context, userID string, from, to time.Time) ([]completion, error) {
	rows, err := r.db.QueryContext(ctx, queryCompletionsBetween, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("list completions: %w", err)
	}
	defer rows.Close()

	var out []completion
	for rows.Next() {
		var c completion
		if err := rows.Scan(&c.Type, &c.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan completion: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ActiveDates returns local dates (YYYY-MM-DD) with at least one completed
// session, newest first.
func (r *Repository) ActiveDates(ctx context.Context, userID, timezone string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, queryActiveDates, userID, timezone)
	if err != nil {
		return nil, fmt.Errorf("list active dates: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("scan active date: %w", err)
		}
		out = append(out, d.Format(time.DateOnly))
	}
	return out, rows.Err()
}

func (r *Repository) History(ctx context.Context, userID string, status *string, after *cursor, limit int) ([]HistoryItem, error) {
	var afterTime *time.Time
	var afterID *string
	if after != nil {
		afterTime, afterID = &after.StartedAt, &after.ID
	}
	rows, err := r.db.QueryContext(ctx, queryHistory, userID, status, afterTime, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	defer rows.Close()

	items := []HistoryItem{}
	for rows.Next() {
		var (
			it          HistoryItem
			completedAt sql.NullTime
			correct     sql.NullInt32
			avg         sql.NullInt32
		)
		if err := rows.Scan(&it.Type, &it.SessionID, &it.ModuleID, &it.ModuleTitle, &it.Difficulty, &it.Status,
			&it.StartedAt, &completedAt, &it.Progress.Total, &it.Progress.Done, &correct, &avg); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		if completedAt.Valid {
			it.CompletedAt = &completedAt.Time
		}
		if correct.Valid {
			v := int(correct.Int32)
			it.Correct = &v
		}
		if avg.Valid {
			v := int(avg.Int32)
			it.AverageScore = &v
		}
		items = append(items, it)
	}
	return items, rows.Err()
}
