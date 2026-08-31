package connectors

import (
	"encoding/json"
	"testing"
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
