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

func TestSelectAIContextScopePrefersWorkoutContextForEquipmentQuestions(t *testing.T) {
	scope := selectAIContextScope("купил гантельные грифы и блины, какие блины докупить под мои рабочие веса?", nil)

	if !scope.workouts {
		t.Fatalf("expected workouts scope to be enabled")
	}
	if scope.finance || scope.nutrition || scope.weather {
		t.Fatalf("expected unrelated scopes to stay disabled, got %+v", scope)
	}
}

func TestSelectAIContextScopeUsesHistoryForFollowUpQuestions(t *testing.T) {
	history := []ChatMessage{
		{Role: "user", Content: "у меня есть гантельные грифы по 10кг и блины 5кг и 2.5кг, какие ещё блины докупить?"},
		{Role: "assistant", Content: "Нужно посчитать по тренировочным весам."},
	}

	scope := selectAIContextScope("про блины то ты так и не ответил", history)

	if !scope.workouts {
		t.Fatalf("expected workouts scope to be enabled from history, got %+v", scope)
	}
	if scope.finance || scope.calendar {
		t.Fatalf("expected unrelated scopes to stay disabled, got %+v", scope)
	}
}

func TestSelectAIContextScopeDefaultsToAllForGenericSummary(t *testing.T) {
	scope := selectAIContextScope("что меня удивило в последнее время?", nil)

	expected := defaultAIContextScope()
	if scope != expected {
		t.Fatalf("expected default all scope, got %+v", scope)
	}
}

func TestBuildAISystemPromptMentionsDialogCorrections(t *testing.T) {
	prompt := buildAISystemPrompt(time.Date(2026, 4, 13, 15, 4, 0, 0, time.UTC), "=== ТРЕНИРОВКИ ===", aiContextScope{workouts: true})

	for _, expected := range []string{
		"Если пользователь уточнил или исправил тебя",
		"Не отвечай, что данных нет, если нужная информация уже была дана пользователем",
		"Сейчас особенно релевантны разделы данных: тренировки.",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", expected, prompt)
		}
	}
}
