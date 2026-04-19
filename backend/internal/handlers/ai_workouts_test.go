package handlers

import (
	"strings"
	"testing"
)

func TestRenderWorkoutOverviewText(t *testing.T) {
	rendered := renderWorkoutOverviewText("=== ТРЕНИРОВКИ ===", AIWorkoutOverviewData{
		WorkoutCount: 2,
		ActiveDays:   2,
		SetCount:     18,
		TopExercises: []AIWorkoutExerciseStat{
			{ExerciseName: "Bench Press", SetCount: 6},
		},
	})

	for _, expected := range []string{
		"=== ТРЕНИРОВКИ ===",
		"2 тренировок",
		"18 подходов",
		"Bench Press: 6 подходов",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected rendered workout overview to contain %q, got:\n%s", expected, rendered)
		}
	}
}

func TestRenderRoutineOverviewTextMarksRoutinesAsPlan(t *testing.T) {
	reps := 8
	weight := 70.0
	rendered := renderRoutineOverviewText("=== HEVY ROUTINES ===", AIRoutineOverviewData{
		Source:     "hevy_routines",
		TotalCount: 1,
		Routines: []AIRoutine{
			{
				ExternalID: "routine-1",
				Title:      "Push",
				Exercises: []AIRoutineExercise{
					{
						Index: 0,
						Name:  "Bench Press",
						Sets: []AIRoutineSet{
							{SetIndex: 1, SetType: "normal", WeightKg: &weight, Reps: &reps},
						},
					},
				},
			},
		},
	})

	for _, expected := range []string{
		"Это шаблоны тренировок из Hevy",
		"Routine: Push",
		"Упражнение 1: Bench Press",
		"Плановый подход 1: 70 кг x 8",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected rendered routine overview to contain %q, got:\n%s", expected, rendered)
		}
	}
}
