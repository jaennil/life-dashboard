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

func TestSanitizeAIToolPlanCapsAndDeduplicates(t *testing.T) {
	plan := aiToolPlan{
		Tools: []aiToolCall{
			{Name: aiToolFinanceOverview, Days: 500},
			{Name: aiToolFinanceOverview, Days: 30},
			{Name: aiToolRecentTransactions, Days: 0, Limit: 99},
			{Name: aiToolWorkoutOverview, Days: 14},
			{Name: aiToolRecentWorkouts, Limit: 0},
			{Name: aiToolNutritionOverview, Days: 7},
		},
	}

	calls := sanitizeAIToolPlan(plan)
	if len(calls) != aiPlannerMaxTools {
		t.Fatalf("expected %d calls after cap, got %d", aiPlannerMaxTools, len(calls))
	}
	if calls[0].Days != 365 {
		t.Fatalf("expected finance days to be capped at 365, got %d", calls[0].Days)
	}
	if calls[1].Limit != 20 {
		t.Fatalf("expected recent transactions limit to be capped at 20, got %d", calls[1].Limit)
	}
	if calls[3].Limit != 4 {
		t.Fatalf("expected recent workouts default limit 4, got %d", calls[3].Limit)
	}
}

func TestFallbackToolPlanForWorkoutQuestion(t *testing.T) {
	calls := fallbackToolPlan("норм тренировка? что улучшить по последней pull тренировке", nil)

	if len(calls) < 2 {
		t.Fatalf("expected at least 2 tool calls, got %d", len(calls))
	}
	if calls[0].Name != aiToolWorkoutOverview {
		t.Fatalf("expected workout overview first, got %s", calls[0].Name)
	}
	if calls[1].Name != aiToolRecentWorkouts {
		t.Fatalf("expected recent workouts second, got %s", calls[1].Name)
	}
}

func TestBuildAISystemPromptMentionsDialogCorrections(t *testing.T) {
	prompt := buildAISystemPrompt(time.Date(2026, 4, 13, 15, 4, 0, 0, time.UTC), "=== ТРЕНИРОВКИ ===", aiContextScope{workouts: true})

	for _, expected := range []string{
		"Если пользователь уточнил или исправил тебя",
		"Не отвечай, что данных нет, если нужная информация уже была дана пользователем",
		"Сейчас особенно релевантны разделы данных: тренировки.",
		"Календарь — это только план из Google Calendar",
		"Питание — это только залогированные записи",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", expected, prompt)
		}
	}
}

func TestResolveCheckupWindowFallsBackToWeekWhenNoPreviousReport(t *testing.T) {
	now := time.Date(2026, 4, 15, 18, 30, 0, 0, time.UTC)

	window, err := resolveCheckupWindow(now, checkupPeriodSinceLast, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if window.RequestedPeriod != checkupPeriodSinceLast {
		t.Fatalf("expected requested period %q, got %q", checkupPeriodSinceLast, window.RequestedPeriod)
	}
	if window.EffectivePeriod != checkupPeriodWeek {
		t.Fatalf("expected effective period %q, got %q", checkupPeriodWeek, window.EffectivePeriod)
	}
	if !strings.Contains(window.Note, "последние 7 дней") {
		t.Fatalf("expected fallback note to mention 7 days, got %q", window.Note)
	}
}

func TestResolveCheckupWindowUsesLastReportTimestamp(t *testing.T) {
	now := time.Date(2026, 4, 15, 18, 30, 0, 0, time.UTC)
	lastReport := time.Date(2026, 4, 12, 9, 15, 0, 0, time.UTC)

	window, err := resolveCheckupWindow(now, checkupPeriodSinceLast, &lastReport)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !window.Start.Equal(lastReport) {
		t.Fatalf("expected start %v, got %v", lastReport, window.Start)
	}
	if !window.End.Equal(now) {
		t.Fatalf("expected end %v, got %v", now, window.End)
	}
	if window.Note != "" {
		t.Fatalf("expected empty note, got %q", window.Note)
	}
}

func TestBuildAICheckupPromptMentionsStructuredReport(t *testing.T) {
	now := time.Date(2026, 4, 15, 18, 30, 0, 0, time.UTC)
	window := checkupWindow{
		RequestedPeriod: checkupPeriodWeek,
		EffectivePeriod: checkupPeriodWeek,
		Title:           "Checkup за неделю",
		UserLabel:       "за последние 7 дней",
		Start:           time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC),
		End:             now,
	}

	prompt := buildAICheckupPrompt(now, window, "=== ФИНАНСЫ ===")
	for _, expected := range []string{
		"1. Короткий итог",
		"3. Активность и тренировки.",
		"Три конкретных шага на следующий период.",
		"Период отчёта: Checkup за неделю",
		"События Google Calendar — это только план/расписание",
		"Факт тренировки подтверждают только данные из workouts/Hevy",
		"Питание отражает только залогированные записи",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", expected, prompt)
		}
	}
}

func TestFormatAIMealTypesTranslatesKnownMeals(t *testing.T) {
	labels := formatAIMealTypes([]string{"lunch", "breakfast", "snack", "lunch", "evening snack"})

	expected := []string{"вечерний перекус", "завтрак", "обед", "перекус"}
	if len(labels) != len(expected) {
		t.Fatalf("expected %d labels, got %d: %#v", len(expected), len(labels), labels)
	}
	for i, label := range expected {
		if labels[i] != label {
			t.Fatalf("expected label %q at index %d, got %q", label, i, labels[i])
		}
	}
}
