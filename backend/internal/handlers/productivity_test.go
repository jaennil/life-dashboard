package handlers

import (
	"testing"
	"time"
)

func TestProductivityDueState(t *testing.T) {
	now := time.Date(2026, 4, 17, 14, 30, 0, 0, time.UTC)
	todayStart := time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	nextWeekStart := todayStart.AddDate(0, 0, 8)

	tests := []struct {
		name       string
		dueAt      *time.Time
		dueDate    *time.Time
		overdue    bool
		wantBucket string
	}{
		{
			name:       "timestamp overdue",
			dueAt:      timePtr(time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)),
			overdue:    true,
			wantBucket: "overdue",
		},
		{
			name:       "date today",
			dueDate:    timePtr(time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)),
			overdue:    false,
			wantBucket: "today",
		},
		{
			name:       "timestamp upcoming",
			dueAt:      timePtr(time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)),
			overdue:    false,
			wantBucket: "upcoming",
		},
		{
			name:       "date later",
			dueDate:    timePtr(time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)),
			overdue:    false,
			wantBucket: "later",
		},
		{
			name:       "no due",
			overdue:    false,
			wantBucket: "no_due",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOverdue, gotBucket := productivityDueState(tt.dueAt, tt.dueDate, now, todayStart, tomorrowStart, nextWeekStart)
			if gotOverdue != tt.overdue {
				t.Fatalf("expected overdue=%v, got %v", tt.overdue, gotOverdue)
			}
			if gotBucket != tt.wantBucket {
				t.Fatalf("expected bucket=%q, got %q", tt.wantBucket, gotBucket)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
