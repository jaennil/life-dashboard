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

func TestPriorityUserIgnoresDormancy(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	// Not seen for half a year: a normal account is dormant and never syncs.
	longGone := now.AddDate(0, -6, 0)
	lastSynced := now.Add(-2 * time.Hour)

	if IsDueForUser(now, "notion", &longGone, &lastSynced, nil, 0, false) {
		t.Error("a dormant normal user should not be due")
	}
	if !IsDueForUser(now, "notion", &longGone, &lastSynced, nil, 0, true) {
		t.Error("a priority user must stay due regardless of inactivity")
	}
}

func TestPriorityUserNeverSyncedIsDue(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	if !IsDueForUser(now, "zepp", nil, nil, nil, 0, true) {
		t.Error("a priority user with no sync history must be due immediately")
	}
}

func TestPriorityUserIntervalIsCapped(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	active := now.Add(-time.Minute)

	// notion is the slowest source: 12h even when hot. A priority user must not
	// wait longer than the cap.
	justOverCap := now.Add(-(priorityMaxInterval + time.Minute))
	if !IsDueForUser(now, "notion", &active, &justOverCap, nil, 0, true) {
		t.Error("priority user should be due once the capped interval has passed")
	}
	if IsDueForUser(now, "notion", &active, &justOverCap, nil, 0, false) {
		t.Error("a normal hot user still waits notion's own 12h interval")
	}

	// Inside the cap it must still hold off, otherwise every tick would fire.
	justUnderCap := now.Add(-(priorityMaxInterval - time.Minute))
	if IsDueForUser(now, "notion", &active, &justUnderCap, nil, 0, true) {
		t.Error("priority user should not sync more often than the cap")
	}
}

func TestPriorityUserFailureBackoffIsShort(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	active := now.Add(-time.Minute)
	lastSynced := now.Add(-3 * time.Hour)

	// Four consecutive failures normally mean a 12 hour wait.
	failedRecently := now.Add(-20 * time.Minute)
	if IsDueForUser(now, "zepp", &active, &lastSynced, &failedRecently, 4, false) {
		t.Error("a normal user should still be backing off after four failures")
	}
	if !IsDueForUser(now, "zepp", &active, &lastSynced, &failedRecently, 4, true) {
		t.Error("a priority user should retry within the tick-sized backoff")
	}

	// The short backoff is still respected, so a failure does not busy-loop.
	failedJustNow := now.Add(-time.Minute)
	if IsDueForUser(now, "zepp", &active, &lastSynced, &failedJustNow, 4, true) {
		t.Error("priority user should wait out the short backoff before retrying")
	}
}

func TestIsDueKeepsLegacyBehaviour(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	dormant := now.AddDate(0, -6, 0)
	lastSynced := now.Add(-100 * time.Hour)

	// The old signature must behave as it always did: dormant means no sync.
	if IsDue(now, "notion", &dormant, &lastSynced, nil, 0) {
		t.Error("IsDue should still refuse a dormant user")
	}
}
