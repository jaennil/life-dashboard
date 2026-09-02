package connectors

import (
	"context"
	"errors"
	"testing"
)

func TestBackfillBatchBalancesSpeedAndSafety(t *testing.T) {
	// Thirty days reduces a multi-day repair to a few scheduled runs without
	// turning one transient API failure into a restart of the entire history.
	if fatSecretBackfillDaysPerRun != 30 {
		t.Fatalf("days per run = %d, want accelerated batch of 30", fatSecretBackfillDaysPerRun)
	}
	if fatSecretBackfillPause <= 0 {
		t.Fatal("backfill has no pause between requests")
	}
	// A full run remains paced instead of sending all requests back to back.
	if total := fatSecretBackfillPause * (fatSecretBackfillDaysPerRun - 1); total < 2_000_000_000 {
		t.Fatalf("a full run spreads over %s, which is still a burst", total)
	}
}

func TestIsFatSecretThrottledCatchesBothShapes(t *testing.T) {
	// Documented shape.
	if !isFatSecretThrottled(errors.New("api error: User is performing too many actions: please try again later")) {
		t.Fatal("error 12 not recognized as throttling")
	}
	// A timeout is also a stop signal for the resumable batch. It can be a stale
	// connection or provider trouble, but continuing would only multiply delays.
	for _, message := range []string{
		`api request: Get "https://platform.fatsecret.com/rest/server.api?...": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`,
		"context deadline exceeded",
		"net/http: TLS handshake timeout",
	} {
		if !isFatSecretThrottled(errors.New(message)) {
			t.Errorf("not recognized as throttling: %s", message)
		}
	}
	if !isFatSecretThrottled(context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded not recognized")
	}

	for _, other := range []error{
		nil,
		errors.New("api error: Invalid signature"),
		errors.New("decode response: unexpected end of JSON input"),
	} {
		if isFatSecretThrottled(other) {
			t.Errorf("%v mistaken for throttling", other)
		}
	}
}

func TestScheduledWindowLeavesRoomForTheBackfill(t *testing.T) {
	// Keep the recurring hot window small because the backfill separately repairs
	// older history and the third regular request would duplicate that work.
	if fatSecretScheduledSyncDays > 3 {
		t.Fatalf("scheduled window = %d days, too wide to leave the backfill anything",
			fatSecretScheduledSyncDays)
	}
	// And the full history has to stay reachable some other way.
	if nutritionSyncDays <= fatSecretScheduledSyncDays {
		t.Fatalf("history depth %d does not exceed the hot window %d",
			nutritionSyncDays, fatSecretScheduledSyncDays)
	}
}
