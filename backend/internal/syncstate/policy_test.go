package syncstate

import (
	"testing"
	"time"
)

func TestIsDueUsesHotUserInterval(t *testing.T) {
	now := time.Date(2026, 4, 24, 18, 0, 0, 0, time.UTC)
	lastActive := now.Add(-2 * time.Hour)
	lastSynced := now.Add(-70 * time.Minute)

	if !IsDue(now, "todoist", &lastActive, &lastSynced, nil, 0) {
		t.Fatalf("expected hot todoist sync to be due after one hour")
	}
}

func TestIsDueSkipsDormantUsers(t *testing.T) {
	now := time.Date(2026, 4, 24, 18, 0, 0, 0, time.UTC)
	lastActive := now.Add(-45 * 24 * time.Hour)
	lastSynced := now.Add(-10 * 24 * time.Hour)

	if IsDue(now, "todoist", &lastActive, &lastSynced, nil, 0) {
		t.Fatalf("expected dormant user to be skipped from scheduled sync")
	}
}

func TestIsDueRespectsFailureBackoff(t *testing.T) {
	now := time.Date(2026, 4, 24, 18, 0, 0, 0, time.UTC)
	lastActive := now.Add(-time.Hour)
	lastSynced := now.Add(-3 * time.Hour)
	lastFailed := now.Add(-20 * time.Minute)

	if IsDue(now, "google_calendar", &lastActive, &lastSynced, &lastFailed, 1) {
		t.Fatalf("expected failure backoff to delay sync retry")
	}
}
