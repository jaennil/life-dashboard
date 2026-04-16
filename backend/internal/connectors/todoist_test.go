package connectors

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestTodoistIsRecurringHabit(t *testing.T) {
	item := todoistItem{ID: "1", Content: "Brush teeth", Due: &todoistDue{Recurring: true}}
	if !todoistIsRecurringHabit(item) {
		t.Fatalf("expected recurring task to be treated as habit")
	}

	item = todoistItem{ID: "2", Content: "Wash face", Due: &todoistDue{IsRecurring: true}}
	if !todoistIsRecurringHabit(item) {
		t.Fatalf("expected api v1 is_recurring task to be treated as habit")
	}

	item.Checked = true
	if todoistIsRecurringHabit(item) {
		t.Fatalf("expected completed task to be excluded from active habits")
	}
}

func TestTodoistTimeOfDay(t *testing.T) {
	result := todoistTimeOfDay(&todoistDue{DateTime: "2026-04-16T06:30:00Z"})
	if len(result) != 1 || result[0] != "06:30" {
		t.Fatalf("unexpected time_of_day %#v", result)
	}

	result = todoistTimeOfDay(&todoistDue{Date: "2026-04-16T06:30:00.000000"})
	if len(result) != 1 || result[0] != "06:30" {
		t.Fatalf("unexpected floating time_of_day %#v", result)
	}
}

func TestTodoistSyncStart(t *testing.T) {
	connector := NewTodoist(nil, zerolog.Nop())
	now := time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC)

	start := connector.syncStart(time.Time{}, now)
	if start.Format("2006-01-02") != "2026-02-16" {
		t.Fatalf("unexpected initial lookback start %s", start.Format("2006-01-02"))
	}

	lastSync := time.Date(2026, 4, 14, 18, 0, 0, 0, time.UTC)
	start = connector.syncStart(lastSync, now)
	if start.Format("2006-01-02") != "2026-03-31" {
		t.Fatalf("unexpected incremental lookback start %s", start.Format("2006-01-02"))
	}
}
