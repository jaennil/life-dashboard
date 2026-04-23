package handlers

import (
	"testing"
	"time"
)

func TestHydrateHevyWorkoutPreservesRichFieldsAndOrder(t *testing.T) {
	rawPayload := []byte(`{
		"title": "Push Day",
		"description": "Heavy bench focus",
		"created_at": "2026-04-08T18:00:00Z",
		"updated_at": "2026-04-08T19:15:00Z",
		"exercises": [
			{
				"index": 1,
				"title": "Bench Press",
				"notes": "Pause on chest",
				"exercise_template_id": "bench-template",
				"sets": [
					{
						"index": 1,
						"type": "warm-up",
						"weight_kg": 60,
						"reps": 10,
						"rpe": 6.5
					}
				]
			},
			{
				"index": 2,
				"title": "Bench Press",
				"notes": "Top set",
				"exercise_template_id": "bench-template",
				"sets": [
					{
						"index": 1,
						"type": "normal",
						"weight_kg": 100,
						"reps": 5,
						"duration_seconds": 45
					},
					{
						"index": 2,
						"type": "drop set",
						"distance_meters": 250,
						"duration_seconds": 75
					}
				]
			}
		]
	}`)

	workout := Workout{
		ID:        "test-id",
		Source:    "hevy",
		Title:     "Old title",
		Notes:     "",
		StartedAt: time.Date(2026, 4, 8, 18, 30, 0, 0, time.UTC),
	}

	if err := hydrateHevyWorkout(&workout, rawPayload); err != nil {
		t.Fatalf("hydrateHevyWorkout returned error: %v", err)
	}

	if workout.Title != "Push Day" {
		t.Fatalf("expected title from raw payload, got %q", workout.Title)
	}
	if workout.Notes != "Heavy bench focus" {
		t.Fatalf("expected notes from raw payload, got %q", workout.Notes)
	}
	if workout.SourceCreatedAt == nil || workout.SourceUpdatedAt == nil {
		t.Fatal("expected source timestamps to be hydrated")
	}
	if len(workout.Exercises) != 2 {
		t.Fatalf("expected 2 exercise blocks, got %d", len(workout.Exercises))
	}

	first := workout.Exercises[0]
	second := workout.Exercises[1]

	if first.Index != 1 || second.Index != 2 {
		t.Fatalf("expected exercise indexes 1 and 2, got %d and %d", first.Index, second.Index)
	}
	if first.Name != "Bench Press" || second.Name != "Bench Press" {
		t.Fatalf("expected both exercise blocks to keep duplicate names, got %q and %q", first.Name, second.Name)
	}
	if first.Notes != "Pause on chest" || second.Notes != "Top set" {
		t.Fatalf("expected exercise notes to be preserved, got %q and %q", first.Notes, second.Notes)
	}
	if first.TemplateID != "bench-template" || second.TemplateID != "bench-template" {
		t.Fatalf("expected template ids to be preserved, got %q and %q", first.TemplateID, second.TemplateID)
	}

	if len(second.Sets) != 2 {
		t.Fatalf("expected 2 sets in second exercise block, got %d", len(second.Sets))
	}
	if second.Sets[0].DurationSeconds == nil || *second.Sets[0].DurationSeconds != 45 {
		t.Fatalf("expected duration_seconds on second block first set, got %+v", second.Sets[0].DurationSeconds)
	}
	if second.Sets[1].DistanceMeters == nil || *second.Sets[1].DistanceMeters != 250 {
		t.Fatalf("expected distance_meters on drop set, got %+v", second.Sets[1].DistanceMeters)
	}
	if second.Sets[1].SetType != "drop set" {
		t.Fatalf("expected set type to be preserved, got %q", second.Sets[1].SetType)
	}
}

func TestBuildExerciseProgressionsDetectsImprovement(t *testing.T) {
	workouts := []Workout{
		{
			StartedAt: time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC),
			Exercises: []WorkoutExercise{
				{
					Name: "Incline Bench Press",
					Sets: []WorkoutSet{
						{SetType: "normal", WeightKg: float64Ptr(42), Reps: intValuePtr(8)},
					},
				},
			},
		},
		{
			StartedAt: time.Date(2026, 4, 13, 8, 0, 0, 0, time.UTC),
			Exercises: []WorkoutExercise{
				{
					Name: "Incline Bench Press",
					Sets: []WorkoutSet{
						{SetType: "normal", WeightKg: float64Ptr(38), Reps: intValuePtr(8)},
					},
				},
			},
		},
	}

	progressions := buildExerciseProgressions(workouts)
	if len(progressions) != 1 {
		t.Fatalf("expected one progression, got %d", len(progressions))
	}
	if progressions[0].Exercise != "Incline Bench Press" {
		t.Fatalf("unexpected exercise: %+v", progressions[0])
	}
	if progressions[0].Delta != "+4 кг" {
		t.Fatalf("expected weight delta, got %+v", progressions[0])
	}
}

func TestClassifyWorkoutSplitPrefersLegsSignals(t *testing.T) {
	workout := Workout{
		Title: "Lower Strength",
		Exercises: []WorkoutExercise{
			{Name: "Back Squat"},
			{Name: "Romanian Deadlift"},
			{Name: "Leg Press"},
		},
	}

	if got := classifyWorkoutSplit(workout); got != "legs" {
		t.Fatalf("expected legs split, got %q", got)
	}
}

func TestStartOfLocalWeekUsesMondayInMoscow(t *testing.T) {
	got := startOfLocalWeek(time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC))
	want := time.Date(2026, 4, 20, 0, 0, 0, 0, aiDisplayLocation)
	if !got.Equal(want) {
		t.Fatalf("expected week start %s, got %s", want, got)
	}
}

func TestBuildHevyGoldenWeeklyAggregatesByWeek(t *testing.T) {
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, aiDisplayLocation)
	workouts := []Workout{
		{
			StartedAt: time.Date(2026, 4, 22, 7, 0, 0, 0, aiDisplayLocation),
			Title:     "Push day",
			Exercises: []WorkoutExercise{
				{Name: "Bench Press", Sets: []WorkoutSet{{}, {}, {}}},
				{Name: "Triceps Rope Pushdown", Sets: []WorkoutSet{{}, {}}},
			},
		},
		{
			StartedAt: time.Date(2026, 4, 15, 7, 0, 0, 0, aiDisplayLocation),
			Title:     "Legs day",
			Exercises: []WorkoutExercise{
				{Name: "Leg Press", Sets: []WorkoutSet{{}, {}, {}, {}}},
			},
		},
	}

	weekly := buildHevyGoldenWeekly(now, workouts, 2)
	if len(weekly) != 2 {
		t.Fatalf("expected 2 weekly buckets, got %d", len(weekly))
	}
	if weekly[0].Week != "2026-04-13" || weekly[1].Week != "2026-04-20" {
		t.Fatalf("unexpected week starts: %+v", weekly)
	}
	if weekly[0].WorkoutsCount != 1 || weekly[0].SetsCount != 4 || weekly[0].LegsCount != 1 {
		t.Fatalf("unexpected older bucket: %+v", weekly[0])
	}
	if weekly[1].WorkoutsCount != 1 || weekly[1].SetsCount != 5 || weekly[1].PushCount != 1 {
		t.Fatalf("unexpected latest bucket: %+v", weekly[1])
	}
}

func float64Ptr(value float64) *float64 { return &value }
func intValuePtr(value int) *int        { return &value }
