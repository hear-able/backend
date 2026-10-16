package progress

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"hearable/backend/package/difficulty"
	apperrors "hearable/backend/package/errors"
	"hearable/backend/package/request"
)

type repository interface {
	CompletionsBetween(ctx context.Context, userID string, from, to time.Time) ([]completion, error)
	ActiveDates(ctx context.Context, userID, timezone string) ([]string, error)
	History(ctx context.Context, userID string, status *string, after *cursor, limit int) ([]HistoryItem, error)
}

type Service struct {
	repo repository
	loc  *time.Location
	now  func() time.Time
}

func NewService(repo repository, loc *time.Location) *Service {
	return &Service{repo: repo, loc: loc, now: time.Now}
}

var weekdayLabels = [7]string{"Sen", "Sel", "Rab", "Kam", "Jum", "Sab", "Min"}

// Activity returns the Monday–Sunday week containing weekOf (default: today)
// in the app timezone. Only completed sessions count as activity.
func (s *Service) Activity(ctx context.Context, userID string, weekOf string) (Activity, error) {
	today := s.now().In(s.loc)
	ref := today
	if weekOf != "" {
		d, err := time.ParseInLocation(time.DateOnly, weekOf, s.loc)
		if err != nil {
			return Activity{}, apperrors.BadRequest("week_of harus berformat YYYY-MM-DD.")
		}
		ref = d
	}
	start := weekStart(ref, s.loc)
	end := start.AddDate(0, 0, 7)

	completions, err := s.repo.CompletionsBetween(ctx, userID, start, end)
	if err != nil {
		return Activity{}, err
	}
	dates, err := s.repo.ActiveDates(ctx, userID, s.loc.String())
	if err != nil {
		return Activity{}, err
	}

	act := Activity{
		Timezone:   s.loc.String(),
		WeekStart:  start.Format(time.DateOnly),
		WeekEnd:    end.AddDate(0, 0, -1).Format(time.DateOnly),
		Days:       make([]Day, 7),
		TargetDays: WeeklyTargetDays,
		StreakDays: streak(dates, today),
	}
	dayIndex := make(map[string]int, 7)
	for i := range act.Days {
		date := start.AddDate(0, 0, i).Format(time.DateOnly)
		act.Days[i] = Day{Date: date, Weekday: weekdayLabels[i]}
		dayIndex[date] = i
	}
	for _, c := range completions {
		idx, ok := dayIndex[c.CompletedAt.In(s.loc).Format(time.DateOnly)]
		if !ok {
			continue
		}
		act.Days[idx].Sessions++
		act.CompletedSessions++
		switch c.Type {
		case TypeListening:
			act.ByType.Listening++
		case TypeSpeaking:
			act.ByType.Speaking++
		}
	}
	for i := range act.Days {
		if act.Days[i].Sessions > 0 {
			act.Days[i].Active = true
			act.ActiveDays++
		}
	}
	return act, nil
}

func (s *Service) History(ctx context.Context, userID string, q HistoryQuery) (History, error) {
	limit := q.Limit
	if limit == 0 {
		limit = defaultHistoryLimit
	}
	if limit < 1 || limit > maxHistoryLimit {
		return History{}, apperrors.BadRequest("limit harus antara 1 dan 50.")
	}

	var status *string
	switch q.Status {
	case "":
	case "in_progress", "completed":
		status = &q.Status
	default:
		return History{}, apperrors.BadRequest("status harus in_progress atau completed.")
	}

	var after *cursor
	if q.Cursor != "" {
		c, err := decodeCursor(q.Cursor)
		if err != nil {
			return History{}, apperrors.BadRequest("cursor tidak valid.")
		}
		after = &c
	}

	items, err := s.repo.History(ctx, userID, status, after, limit+1)
	if err != nil {
		return History{}, err
	}

	var next *string
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		c := encodeCursor(cursor{StartedAt: last.StartedAt, ID: last.SessionID})
		next = &c
	}
	for i := range items {
		items[i].DiffLabel = difficulty.Label(items[i].Difficulty)
	}
	return History{Items: items, NextCursor: next}, nil
}

func dayStart(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func weekStart(t time.Time, loc *time.Location) time.Time {
	d := dayStart(t, loc)
	offset := (int(d.Weekday()) + 6) % 7 // Monday = 0
	return d.AddDate(0, 0, -offset)
}

// streak counts consecutive active days ending today, or ending yesterday
// if the user hasn't practiced yet today (the streak is still alive).
// dates must be distinct YYYY-MM-DD strings, newest first.
func streak(dates []string, today time.Time) int {
	if len(dates) == 0 {
		return 0
	}
	expected := today
	if dates[0] != today.Format(time.DateOnly) {
		expected = today.AddDate(0, 0, -1)
	}
	n := 0
	for _, d := range dates {
		if d != expected.Format(time.DateOnly) {
			break
		}
		n++
		expected = expected.AddDate(0, 0, -1)
	}
	return n
}

func encodeCursor(c cursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.StartedAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID))
}

func decodeCursor(s string) (cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return cursor{}, err
	}
	ts, id, ok := strings.Cut(string(raw), "|")
	if !ok || !request.IsUUID(id) {
		return cursor{}, apperrors.BadRequest("cursor tidak valid.")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return cursor{}, err
	}
	return cursor{StartedAt: t, ID: id}, nil
}
