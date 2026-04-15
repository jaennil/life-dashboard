package connectors

import (
	"encoding/json"
	"testing"
)

func TestHevyFlexibleStringUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "string", payload: `"superset-1"`, want: "superset-1"},
		{name: "number", payload: `123`, want: "123"},
		{name: "null", payload: `null`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got hevyFlexibleString
			if err := json.Unmarshal([]byte(tt.payload), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.String() != tt.want {
				t.Fatalf("got %q want %q", got.String(), tt.want)
			}
		})
	}
}

func TestHevyRoutineResponseDecodesNumericSupersetID(t *testing.T) {
	t.Parallel()

	payload := []byte(`{
		"page": 1,
		"page_count": 1,
		"routines": [
			{
				"id": "routine-1",
				"title": "Push",
				"folder_id": 1,
				"updated_at": "2026-04-15T18:00:00Z",
				"created_at": "2026-04-10T18:00:00Z",
				"exercises": [
					{
						"index": 0,
						"title": "Bench Press",
						"notes": null,
						"exercise_template_id": "tpl-1",
						"superset_id": 42,
						"rest_seconds": 120,
						"sets": [
							{
								"index": 0,
								"type": "normal",
								"weight_kg": 80,
								"reps": 8,
								"distance_meters": null,
								"duration_seconds": null,
								"custom_metric": null
							}
						]
					}
				]
			}
		]
	}`)

	var decoded hevyRoutinesResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal routines response: %v", err)
	}

	if len(decoded.Routines) != 1 {
		t.Fatalf("got %d routines want 1", len(decoded.Routines))
	}
	if got := decoded.Routines[0].Exercises[0].SupersetID.String(); got != "42" {
		t.Fatalf("got superset_id %q want %q", got, "42")
	}
}
