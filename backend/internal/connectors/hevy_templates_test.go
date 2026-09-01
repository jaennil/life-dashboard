package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// Captured from the live API. The point of the test is the field names: the
// write side of this API documents camelCase in places while the read side
// answers in snake_case, so a silent rename here would leave every template
// blank without failing anything.
const hevyTemplatesPayload = `{
  "page": 1,
  "page_count": 5,
  "exercise_templates": [
    {"id":"3BC06AD3","title":"21s Bicep Curl","type":"weight_reps","primary_muscle_group":"biceps","secondary_muscle_groups":[],"equipment":"barbell","is_custom":false},
    {"id":"B4F2FF72","title":"Ab Scissors","type":"reps_only","primary_muscle_group":"abdominals","secondary_muscle_groups":["core"],"equipment":"none","is_custom":true}
  ]
}`

func TestDecodeHevyExerciseTemplates(t *testing.T) {
	var resp hevyExerciseTemplatesResponse
	if err := json.Unmarshal([]byte(hevyTemplatesPayload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.PageCount != 5 {
		t.Fatalf("page_count = %d, want 5", resp.PageCount)
	}
	if len(resp.Templates) != 2 {
		t.Fatalf("templates = %d, want 2", len(resp.Templates))
	}

	first := resp.Templates[0]
	if first.ID != "3BC06AD3" || first.Title != "21s Bicep Curl" {
		t.Fatalf("first template = %+v", first)
	}
	// type drives which set fields are legal downstream, so it must survive.
	if first.Type != "weight_reps" {
		t.Fatalf("type = %q, want weight_reps", first.Type)
	}
	if first.PrimaryMuscleGroup != "biceps" || first.Equipment != "barbell" {
		t.Fatalf("muscle/equipment = %q/%q", first.PrimaryMuscleGroup, first.Equipment)
	}
	if first.IsCustom {
		t.Fatal("built-in template decoded as custom")
	}

	second := resp.Templates[1]
	if !second.IsCustom {
		t.Fatal("custom template decoded as built-in")
	}
	if len(second.SecondaryMuscleGroups) != 1 || second.SecondaryMuscleGroups[0] != "core" {
		t.Fatalf("secondary muscle groups = %v", second.SecondaryMuscleGroups)
	}
}

func TestHevyTemplateOwnerOnlyForCustom(t *testing.T) {
	if owner := hevyTemplateOwner("user-1", false); owner != nil {
		t.Fatalf("built-in template got owner %v", *owner)
	}
	owner := hevyTemplateOwner("user-1", true)
	if owner == nil || *owner != "user-1" {
		t.Fatalf("custom template owner = %v, want user-1", owner)
	}
}

func TestRoutinePageSizeStaysBelowTheTruncationThreshold(t *testing.T) {
	// The endpoint answers 200 and then stops mid-body once the response grows
	// past roughly 12 KB. Measured: pageSize 1 gave 4.3 KB, 2 gave 7.0 KB, 5 gave
	// a truncated 12.5 KB and 10 gave a truncated 13-14 KB. Anything above two is
	// gambling on how large the account's routines happen to be.
	if hevyRoutinePageSize > 2 {
		t.Fatalf("routine page size = %d, large enough to hit the truncation", hevyRoutinePageSize)
	}
	if hevyRoutinePageSize < 1 {
		t.Fatalf("routine page size = %d", hevyRoutinePageSize)
	}
	// Workouts and events are unaffected and should keep their larger pages.
	if hevyPageSize <= hevyRoutinePageSize {
		t.Fatalf("workout page size %d was reduced along with routines", hevyPageSize)
	}
}

func TestTemplatePageSizeStaysSmall(t *testing.T) {
	// Measured against the live endpoint: 100 per page returned 8 KB and then
	// stalled until the client gave up, failing the whole sync. Ten is about 2 KB.
	// The threshold moved between two probes hours apart, so this is a ceiling on
	// optimism rather than a tuned value.
	if hevyTemplatePageSize > 20 {
		t.Fatalf("template page size = %d, large enough to stall", hevyTemplatePageSize)
	}
	// And the refresh has to stay off the hourly path, since a full walk is now
	// dozens of requests.
	if hevyTemplateMaxAge < 12*time.Hour {
		t.Fatalf("template max age = %s, too eager for a catalogue that rarely changes", hevyTemplateMaxAge)
	}
}

func TestIsHevyStallOnlyMatchesWaiting(t *testing.T) {
	// The two forms observed: the request times out waiting for headers, or the
	// body stops mid-stream and the decoder reports the deadline underneath.
	for _, message := range []string{
		`Get "https://api.hevyapp.com/v1/routines?page=2&pageSize=1": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`,
		"decode response: context deadline exceeded",
		"decode response: unexpected EOF",
		"net/http: TLS handshake timeout",
	} {
		if !isHevyStall(errors.New(message)) {
			t.Errorf("not recognized as a stall: %s", message)
		}
	}
	if !isHevyStall(context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded not recognized")
	}

	// Repeating these would fail identically and only burn the retry budget.
	for _, other := range []error{
		nil,
		errors.New("hevy api returned status 404"),
		errors.New("decode response: invalid character 'x'"),
	} {
		if isHevyStall(other) {
			t.Errorf("%v mistaken for a stall", other)
		}
	}
}

func TestHevyRetryBudgetIsBounded(t *testing.T) {
	// Three attempts of twelve seconds stays inside the client's own timeout
	// budget rather than turning one flaky page into a minute of waiting.
	if hevyRequestAttempts < 2 || hevyRequestAttempts > 4 {
		t.Fatalf("attempts = %d", hevyRequestAttempts)
	}
	if hevyAttemptTimeout > 15*time.Second {
		t.Fatalf("attempt timeout = %s, too long to notice a stall", hevyAttemptTimeout)
	}
}
