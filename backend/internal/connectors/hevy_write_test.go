package connectors

import (
	"encoding/json"
	"testing"
)

func TestDecodeCreateWorkoutResponseArrayShape(t *testing.T) {
	// The shape the endpoint actually returns, captured from a real create: one
	// workout, wrapped in an array. Decoding it as an object failed *after* the
	// workout existed, losing the id and setting up a duplicate on retry.
	payload := `{"workout":[{"id":"46adc6ae-ba68-4fb6-94e6-b7edf8a592ec","title":"Плечи и бицепс"}]}`

	var decoded hevyCreateWorkoutResponse
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Workout.ID != "46adc6ae-ba68-4fb6-94e6-b7edf8a592ec" {
		t.Fatalf("id = %q", decoded.Workout.ID)
	}
}

func TestDecodeCreateWorkoutResponseObjectShape(t *testing.T) {
	// Kept working, in case the wrapping ever changes back.
	payload := `{"workout":{"id":"abc","title":"x"}}`

	var decoded hevyCreateWorkoutResponse
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Workout.ID != "abc" {
		t.Fatalf("id = %q", decoded.Workout.ID)
	}
}

func TestDecodeCreateWorkoutResponseEmptyArray(t *testing.T) {
	var decoded hevyCreateWorkoutResponse
	if err := json.Unmarshal([]byte(`{"workout":[]}`), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// No id and no error: the caller decides what an empty answer means, and it
	// refuses to report success without an id.
	if decoded.Workout.ID != "" {
		t.Fatalf("id = %q, want empty", decoded.Workout.ID)
	}
}

func TestDecodeCreateWorkoutResponseTopLevelID(t *testing.T) {
	var decoded hevyCreateWorkoutResponse
	if err := json.Unmarshal([]byte(`{"id":"top-level"}`), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ID != "top-level" {
		t.Fatalf("top-level id = %q", decoded.ID)
	}
}
