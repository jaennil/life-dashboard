package connectors

import (
	"context"
	"errors"
	"testing"
)

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

func TestIsFatSecretThrottledCatchesBothShapes(t *testing.T) {
	// Documented shape.
	if !isFatSecretThrottled(errors.New("api error: User is performing too many actions: please try again later")) {
		t.Fatal("error 12 not recognized as throttling")
	}
	// Observed shape: the server stops answering and the request dies on the
	// client timeout. Missing this kept the backfill burning thirty seconds a day
	// without ever advancing.
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
