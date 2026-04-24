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

func TestProductivityIsStaleTask(t *testing.T) {
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	todayStart := time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC)
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	nextWeekStart := todayStart.AddDate(0, 0, 8)
	staleBefore := todayStart.AddDate(0, 0, -14)

	tests := []struct {
		name string
		task ProductivityTask
		want bool
	}{
		{
			name: "old task without due date is stale",
			task: ProductivityTask{
				AddedAt: timePtr(time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)),
			},
			want: true,
		},
		{
			name: "old task with future due date is not stale",
			task: ProductivityTask{
				AddedAt: timePtr(time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)),
				DueDate: timePtr(time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC)),
			},
			want: false,
		},
		{
			name: "old task with future due timestamp is not stale",
			task: ProductivityTask{
				AddedAt: timePtr(time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)),
				DueAt:   timePtr(time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)),
			},
			want: false,
		},
		{
			name: "old overdue task is stale",
			task: ProductivityTask{
				AddedAt: timePtr(time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)),
				DueDate: timePtr(time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)),
			},
			want: true,
		},
		{
			name: "recent task is not stale",
			task: ProductivityTask{
				AddedAt: timePtr(time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := productivityIsStaleTask(tt.task, now, todayStart, tomorrowStart, nextWeekStart, staleBefore)
			if got != tt.want {
				t.Fatalf("expected stale=%v, got %v", tt.want, got)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
