package handlers

import (
	"strings"
	"testing"
	"time"
)

func TestFormatAIWorkoutContextIncludesHevyDetails(t *testing.T) {
	reps := 8
	durationSeconds := 75
	weight := 82.5
	rpe := 8.5

	workout := Workout{
		Title:     "Push",
		Notes:     "Heavy bench focus",
		StartedAt: time.Date(2026, 4, 10, 19, 30, 0, 0, time.UTC),
		Exercises: []WorkoutExercise{
			{
				Index: 1,
				Name:  "Bench Press",
				Notes: "Pause on chest",
				Sets: []WorkoutSet{
					{
						SetIndex:        1,
						WeightKg:        &weight,
						Reps:            &reps,
						DurationSeconds: &durationSeconds,
						RPE:             &rpe,
						SetType:         "drop set",
					},
				},
			},
		},
	}

	context := formatAIWorkoutContext(workout)

	for _, expected := range []string{
		"Тренировка 10.04.2026 19:30: Push",
		"Заметки: Heavy bench focus",
		"Bench Press [блок 1]",
		"Заметки: Pause on chest",
		"Подход 1: 82.5 кг x 8, 75 сек, RPE 8.5 [drop set]",
	} {
		if !strings.Contains(context, expected) {
			t.Fatalf("expected context to contain %q, got:\n%s", expected, context)
		}
	}
}
