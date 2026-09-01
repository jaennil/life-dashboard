package connectors

import "testing"

func TestBackfillPacingIsSlowerThanABurst(t *testing.T) {
	// Guards the reason the constants exist: walking days back to back is what
	// earned the throttle, so the pause has to be real, and one run has to stay
	// small enough that the whole history takes several runs.
	if fatSecretBackfillPause <= 0 {
		t.Fatal("backfill has no pause between requests")
	}
	if fatSecretBackfillDaysPerRun <= 0 || fatSecretBackfillDaysPerRun > 30 {
		t.Fatalf("days per run = %d, want a small bounded batch", fatSecretBackfillDaysPerRun)
	}
	// A run must not itself become a burst: ten days spaced by 400ms is four
	// seconds of traffic, not fifty-seven requests as fast as the network allows.
	if total := fatSecretBackfillPause * (fatSecretBackfillDaysPerRun - 1); total < 2_000_000_000 {
		t.Fatalf("a full run spreads over %s, which is still a burst", total)
	}
}
