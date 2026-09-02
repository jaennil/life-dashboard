package connectors

import (
	"strings"
	"testing"
	"time"
)

func TestFatSecretRecentSyncError(t *testing.T) {
	t.Run("success when recent days refreshed", func(t *testing.T) {
		if err := fatSecretRecentSyncError(nil, 2); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("fails when recent days failed", func(t *testing.T) {
		err := fatSecretRecentSyncError([]string{"2026-04-22", "2026-04-21"}, 1)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "2026-04-22") {
			t.Fatalf("expected failed date in error, got %v", err)
		}
	})

	t.Run("fails when nothing recent refreshed", func(t *testing.T) {
		err := fatSecretRecentSyncError(nil, 0)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestIsFatSecretRateLimitError(t *testing.T) {
	if !isFatSecretRateLimitError(assertErr("api error: User is performing too many actions: please try again later")) {
		t.Fatal("expected rate limit error to be detected")
	}
	if isFatSecretRateLimitError(assertErr("api status 500")) {
		t.Fatal("did not expect generic status error to be treated as rate limit")
	}
}

func TestFatSecretSyncDays(t *testing.T) {
	if got := fatSecretSyncDays(SyncTriggerScheduled); got != fatSecretScheduledSyncDays+1 {
		t.Fatalf("expected scheduled sync days %d, got %d", fatSecretScheduledSyncDays+1, got)
	}
	if got := fatSecretSyncDays(SyncTriggerManual); got != nutritionSyncDays {
		t.Fatalf("expected manual sync days %d, got %d", nutritionSyncDays, got)
	}
	if got := fatSecretSyncDays(SyncTriggerInitial); got != nutritionSyncDays {
		t.Fatalf("expected initial sync days %d, got %d", nutritionSyncDays, got)
	}
	if got := fatSecretSyncDays(SyncTriggerUnknown); got != nutritionSyncDays {
		t.Fatalf("expected unknown sync days %d, got %d", nutritionSyncDays, got)
	}
}

func TestFatSecretScheduledSyncDates(t *testing.T) {
	now := time.Date(2026, time.August, 4, 0, 10, 0, 0, time.FixedZone("MSK", 3*60*60))
	dates := fatSecretSyncDates(SyncTriggerScheduled, now, false)

	if len(dates) != fatSecretScheduledSyncDays+1 {
		t.Fatalf("expected %d dates, got %d", fatSecretScheduledSyncDays+1, len(dates))
	}
	if got := dates[0].Format("2006-01-02"); got != "2026-08-04" {
		t.Fatalf("expected current calendar date, got %s", got)
	}
	if got := dates[1].Format("2006-01-02"); got != "2026-08-03" {
		t.Fatalf("expected previous calendar date, got %s", got)
	}

	historyAge := int(dates[0].Sub(dates[len(dates)-1]).Hours() / 24)
	if historyAge < fatSecretScheduledSyncDays || historyAge >= nutritionSyncDays {
		t.Fatalf("expected historical date age in [%d, %d), got %d", fatSecretScheduledSyncDays, nutritionSyncDays, historyAge)
	}
}

func TestFatSecretScheduledHistoryRotates(t *testing.T) {
	now := time.Date(2026, time.August, 4, 0, 10, 0, 0, time.UTC)
	first := fatSecretSyncDates(SyncTriggerScheduled, now, false)
	second := fatSecretSyncDates(SyncTriggerScheduled, now.Add(fatSecretHistorySlot), false)

	if first[len(first)-1].Equal(second[len(second)-1]) {
		t.Fatal("expected historical date to rotate between sync slots")
	}
}

func TestFatSecretManualSyncDates(t *testing.T) {
	now := time.Date(2026, time.August, 4, 23, 50, 0, 0, time.FixedZone("MSK", 3*60*60))
	dates := fatSecretSyncDates(SyncTriggerManual, now, false)

	if len(dates) != nutritionSyncDays {
		t.Fatalf("expected %d dates, got %d", nutritionSyncDays, len(dates))
	}
	if got := dates[len(dates)-1].Format("2006-01-02"); got != "2026-05-07" {
		t.Fatalf("expected oldest sync date 2026-05-07, got %s", got)
	}
}

func assertErr(message string) error {
	return &fatSecretTestError{message: message}
}

type fatSecretTestError struct {
	message string
}

func (e *fatSecretTestError) Error() string {
	return e.message
}

func TestPendingBackfillTakesTheHistorySlot(t *testing.T) {
	now := time.Date(2026, 9, 2, 0, 3, 0, 0, time.UTC)

	// Idle: the hot window plus one rotating history day.
	idle := fatSecretSyncDates(SyncTriggerScheduled, now, false)
	if len(idle) != fatSecretScheduledSyncDays+1 {
		t.Fatalf("idle run reads %d days, want %d", len(idle), fatSecretScheduledSyncDays+1)
	}

	// Backfill pending: the redundant rotating history day yields to the targeted
	// repair pass.
	busy := fatSecretSyncDates(SyncTriggerScheduled, now, true)
	if len(busy) != fatSecretScheduledSyncDays {
		t.Fatalf("run with pending backfill reads %d days, want %d", len(busy), fatSecretScheduledSyncDays)
	}
	for i, d := range busy {
		if want := calendarDate(now).AddDate(0, 0, -i); !d.Equal(want) {
			t.Fatalf("day %d = %s, want %s", i, d, want)
		}
	}

	// A manual run is a deliberate full read and is not budgeted.
	manual := fatSecretSyncDates(SyncTriggerManual, now, true)
	if len(manual) != nutritionSyncDays {
		t.Fatalf("manual run reads %d days, want %d", len(manual), nutritionSyncDays)
	}
}
