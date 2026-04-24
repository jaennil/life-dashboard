package connectors

import (
	"strings"
	"testing"
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
	if got := fatSecretSyncDays(SyncTriggerScheduled); got != fatSecretScheduledSyncDays {
		t.Fatalf("expected scheduled sync days %d, got %d", fatSecretScheduledSyncDays, got)
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

func assertErr(message string) error {
	return &fatSecretTestError{message: message}
}

type fatSecretTestError struct {
	message string
}

func (e *fatSecretTestError) Error() string {
	return e.message
}
