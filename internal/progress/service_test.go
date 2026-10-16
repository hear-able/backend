package progress

import (
	"context"
	"testing"
	"time"
)

type fakeRepo struct {
	completions []completion
	dates       []string
	gotFrom     time.Time
	gotTo       time.Time
	history     []HistoryItem
	gotLimit    int
}

func (f *fakeRepo) CompletionsBetween(_ context.Context, _ string, from, to time.Time) ([]completion, error) {
	f.gotFrom, f.gotTo = from, to
	return f.completions, nil
}
func (f *fakeRepo) ActiveDates(context.Context, string, string) ([]string, error) {
	return f.dates, nil
}
func (f *fakeRepo) History(_ context.Context, _ string, _ *string, _ *cursor, limit int) ([]HistoryItem, error) {
	f.gotLimit = limit
	return f.history, nil
}

func jakarta(t *testing.T) *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestActivityBucketsByLocalDay(t *testing.T) {
	loc := jakarta(t)
	// Wednesday 2026-10-14 10:00 WIB.
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, loc)
	repo := &fakeRepo{
		completions: []completion{
			// Monday 23:30 WIB = Monday 16:30 UTC.
			{Type: TypeListening, CompletedAt: time.Date(2026, 10, 12, 16, 30, 0, 0, time.UTC)},
			// Tuesday 00:30 WIB = Monday 17:30 UTC: must land on Tuesday.
			{Type: TypeSpeaking, CompletedAt: time.Date(2026, 10, 12, 17, 30, 0, 0, time.UTC)},
			{Type: TypeSpeaking, CompletedAt: time.Date(2026, 10, 14, 2, 0, 0, 0, time.UTC)},
		},
		dates: []string{"2026-10-14", "2026-10-13", "2026-10-12", "2026-10-10"},
	}
	svc := NewService(repo, loc)
	svc.now = func() time.Time { return now }

	act, err := svc.Activity(context.Background(), "u", "")
	if err != nil {
		t.Fatal(err)
	}
	if act.WeekStart != "2026-10-12" || act.WeekEnd != "2026-10-18" {
		t.Fatalf("week = %s..%s", act.WeekStart, act.WeekEnd)
	}
	if !repo.gotFrom.Equal(time.Date(2026, 10, 12, 0, 0, 0, 0, loc)) || !repo.gotTo.Equal(time.Date(2026, 10, 19, 0, 0, 0, 0, loc)) {
		t.Fatalf("query range %v - %v", repo.gotFrom, repo.gotTo)
	}
	wantSessions := []int{1, 1, 1, 0, 0, 0, 0}
	for i, d := range act.Days {
		if d.Sessions != wantSessions[i] {
			t.Errorf("day %d (%s) sessions=%d want %d", i, d.Date, d.Sessions, wantSessions[i])
		}
	}
	if act.Days[0].Weekday != "Sen" || act.Days[6].Weekday != "Min" {
		t.Errorf("weekday labels %s..%s", act.Days[0].Weekday, act.Days[6].Weekday)
	}
	if act.ActiveDays != 3 || act.TargetDays != 5 || act.CompletedSessions != 3 {
		t.Errorf("active=%d target=%d completed=%d", act.ActiveDays, act.TargetDays, act.CompletedSessions)
	}
	if act.ByType.Listening != 1 || act.ByType.Speaking != 2 {
		t.Errorf("by type %+v", act.ByType)
	}
	if act.StreakDays != 3 {
		t.Errorf("streak=%d want 3", act.StreakDays)
	}
}

func TestActivityRejectsBadWeekOf(t *testing.T) {
	svc := NewService(&fakeRepo{}, time.UTC)
	if _, err := svc.Activity(context.Background(), "u", "14-10-2026"); err == nil {
		t.Fatal("expected error")
	}
}

func TestStreak(t *testing.T) {
	today := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		dates []string
		want  int
	}{
		{"none", nil, 0},
		{"today only", []string{"2026-10-14"}, 1},
		{"alive from yesterday", []string{"2026-10-13", "2026-10-12"}, 2},
		{"broken", []string{"2026-10-12", "2026-10-11"}, 0},
		{"gap", []string{"2026-10-14", "2026-10-13", "2026-10-11"}, 2},
	}
	for _, c := range cases {
		if got := streak(c.dates, today); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

func TestHistoryPagination(t *testing.T) {
	t0 := time.Date(2026, 10, 14, 1, 2, 3, 456789000, time.UTC)
	repo := &fakeRepo{history: []HistoryItem{
		{SessionID: "11111111-1111-1111-1111-111111111111", StartedAt: t0, Difficulty: 1},
		{SessionID: "22222222-2222-2222-2222-222222222222", StartedAt: t0.Add(-time.Hour), Difficulty: 2},
		{SessionID: "33333333-3333-3333-3333-333333333333", StartedAt: t0.Add(-2 * time.Hour), Difficulty: 3},
	}}
	svc := NewService(repo, time.UTC)
	h, err := svc.History(context.Background(), "u", HistoryQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if repo.gotLimit != 3 || len(h.Items) != 2 || h.NextCursor == nil {
		t.Fatalf("limit=%d items=%d next=%v", repo.gotLimit, len(h.Items), h.NextCursor)
	}
	if h.Items[1].DiffLabel != "Dasar" {
		t.Errorf("label %q", h.Items[1].DiffLabel)
	}
	c, err := decodeCursor(*h.NextCursor)
	if err != nil || !c.StartedAt.Equal(t0.Add(-time.Hour)) || c.ID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("cursor %+v %v", c, err)
	}

	for _, q := range []HistoryQuery{{Limit: 51}, {Status: "done"}, {Cursor: "!!"}} {
		if _, err := svc.History(context.Background(), "u", q); err == nil {
			t.Errorf("expected error for %+v", q)
		}
	}
}
