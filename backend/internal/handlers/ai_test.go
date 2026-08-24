package handlers

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
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
		"Тренировка 10.04.2026 22:30: Push",
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

func TestFormatAICalendarEventUsesMoscowTimezone(t *testing.T) {
	start := time.Date(2026, 4, 21, 18, 30, 0, 0, time.UTC)
	end := time.Date(2026, 4, 22, 2, 30, 0, 0, time.UTC)

	got := formatAICalendarEvent(start, end, false, "сон", "")

	want := "  21.04 21:30-05:30: сон"
	if got != want {
		t.Fatalf("calendar event = %q, want %q", got, want)
	}
}

func TestFormatAIJournalEntryUntitled(t *testing.T) {
	entry := aiJournalEntry{
		Date:    time.Date(2025, 4, 10, 0, 0, 0, 0, time.UTC),
		Title:   "   ",
		Content: "купить носки",
	}

	got := formatAIJournalEntry(entry)

	if !strings.Contains(got, "10.04.2025: (без названия)") {
		t.Fatalf("expected untitled fallback, got %q", got)
	}
	if !strings.Contains(got, "купить носки") {
		t.Fatalf("expected content in formatted journal entry, got %q", got)
	}
}

func TestFormatAIJournalEntryWithLimitCanKeepFullContent(t *testing.T) {
	content := strings.Repeat("abc", 130)
	entry := aiJournalEntry{
		Date:    time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC),
		Title:   "full note",
		Content: content,
	}

	got := formatAIJournalEntryWithLimit(entry, 0)

	if strings.Contains(got, "...") {
		t.Fatalf("expected untruncated content, got %q", got)
	}
	if !strings.Contains(got, content) {
		t.Fatalf("expected full content in formatted journal entry, got %q", got)
	}
}

func TestWriteAIJournalEntriesWithinBudgetOmitsOverflow(t *testing.T) {
	entries := []aiJournalEntry{
		{
			Date:    time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC),
			Title:   "entry 1",
			Content: strings.Repeat("a", 80),
		},
		{
			Date:    time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC),
			Title:   "entry 2",
			Content: strings.Repeat("b", 80),
		},
	}

	var sb strings.Builder
	written, omitted := writeAIJournalEntriesWithinBudget(&sb, entries, 0, 140, 0)

	if written != 1 {
		t.Fatalf("written = %d, want 1", written)
	}
	if omitted != 1 {
		t.Fatalf("omitted = %d, want 1", omitted)
	}
	if !strings.Contains(sb.String(), "entry 1") {
		t.Fatalf("expected first entry to be included, got %q", sb.String())
	}
	if strings.Contains(sb.String(), "entry 2") {
		t.Fatalf("expected second entry to be omitted, got %q", sb.String())
	}
}

func TestParseAIStreamDataDelta(t *testing.T) {
	delta, done, err := parseAIStreamData(`{"choices":[{"delta":{"content":"Привет"}}]}`)
	if err != nil {
		t.Fatalf("parseAIStreamData returned error: %v", err)
	}
	if done {
		t.Fatalf("expected done=false")
	}
	if delta != "Привет" {
		t.Fatalf("delta = %q, want %q", delta, "Привет")
	}
}

func TestParseAIStreamDataDone(t *testing.T) {
	delta, done, err := parseAIStreamData(`[DONE]`)
	if err != nil {
		t.Fatalf("parseAIStreamData returned error: %v", err)
	}
	if !done {
		t.Fatalf("expected done=true")
	}
	if delta != "" {
		t.Fatalf("delta = %q, want empty", delta)
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

func TestSelectAIContextScopeTreatsWaterAsNutrition(t *testing.T) {
	scope := selectAIContextScope("сколько воды я пью и добираю ли норму по воде?", nil)

	if !scope.nutrition {
		t.Fatalf("expected nutrition scope to be enabled for water questions")
	}
	if scope.finance || scope.weather {
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

func TestMergeChatHistoryKeepsStoredOrderAndClientTail(t *testing.T) {
	stored := []ChatMessage{
		{Role: "user", Content: "у меня есть 2 грифа по 11кг"},
		{Role: "assistant", Content: "запомнил"},
	}
	client := []ChatMessage{
		{Role: "user", Content: "у меня есть 2 грифа по 11кг"},
		{Role: "assistant", Content: "запомнил"},
		{Role: "user", Content: "и блины 5кг и 2.5кг"},
	}

	merged := mergeChatHistory(stored, client, 10)
	if len(merged) != 3 {
		t.Fatalf("expected deduplicated merged history length 3, got %d: %#v", len(merged), merged)
	}
	if merged[0].Content != "у меня есть 2 грифа по 11кг" || merged[2].Content != "и блины 5кг и 2.5кг" {
		t.Fatalf("expected stored order plus client tail, got %#v", merged)
	}
}

func TestSanitizeChatHistoryTrimsInvalidAndLimits(t *testing.T) {
	history := []ChatMessage{
		{Role: "system", Content: "ignore"},
		{Role: " user ", Content: " first "},
		{Role: "assistant", Content: ""},
		{Role: "assistant", Content: "second"},
		{Role: "user", Content: "third"},
	}

	sanitized := sanitizeChatHistory(history, 2)
	if len(sanitized) != 2 {
		t.Fatalf("expected 2 sanitized messages, got %d: %#v", len(sanitized), sanitized)
	}
	if sanitized[0].Role != "assistant" || sanitized[0].Content != "second" {
		t.Fatalf("expected assistant second as first sanitized message, got %#v", sanitized[0])
	}
	if sanitized[1].Role != "user" || sanitized[1].Content != "third" {
		t.Fatalf("expected user third as second sanitized message, got %#v", sanitized[1])
	}
}

func TestSelectAIContextScopeEnablesRoutinesForTemplateQuestions(t *testing.T) {
	scope := selectAIContextScope("покажи мою hevy routine на pull и плановые веса", nil)

	if !scope.routines {
		t.Fatalf("expected routines scope to be enabled")
	}
}

func TestSelectAIContextScopeEnablesHabitsForRoutineQuestions(t *testing.T) {
	scope := selectAIContextScope("как у меня с привычками todoist? чищу ли я зубы регулярно?", nil)

	if !scope.habits {
		t.Fatalf("expected habits scope to be enabled")
	}
	if scope.finance || scope.weather {
		t.Fatalf("expected unrelated scopes to stay disabled, got %+v", scope)
	}
}

func TestSelectAIContextScopeEnablesHealthForSleepQuestions(t *testing.T) {
	scope := selectAIContextScope("как у меня со сном и пульсом по zepp за неделю?", nil)

	if !scope.health {
		t.Fatalf("expected health scope to be enabled")
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

func TestNormalizeHealthMetricTypeAliases(t *testing.T) {
	cases := map[string]string{
		"Step Count":             "steps",
		"body-mass":              "weight",
		"heart_rate_bpm":         "heart_rate",
		"Resting HeartRate":      "resting_heart_rate",
		"Active Energy Burned":   "active_energy",
		"Heart Rate Variability": "hrv",
	}

	for input, expected := range cases {
		if got := normalizeHealthMetricType(input); got != expected {
			t.Fatalf("expected %q -> %q, got %q", input, expected, got)
		}
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

func TestFallbackToolPlanForRoutineQuestion(t *testing.T) {
	calls := fallbackToolPlan("что у меня в hevy routine push?", nil)

	found := false
	for _, call := range calls {
		if call.Name == aiToolRoutineOverview {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected routine overview tool in fallback plan, got %#v", calls)
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
		"3. Продуктивность и задачи.",
		"4. Активность и тренировки.",
		"6. Привычки.",
		"10. Три конкретных шага на следующий период.",
		"Период отчёта: Checkup за неделю",
		"События Google Calendar — это только план/расписание",
		"Факт тренировки подтверждают только данные из workouts/Hevy",
		"Продуктивность и задачи подтверждаются только данными Todoist",
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

func TestUpstreamBodyOmitsReasoningEffortWhenUnset(t *testing.T) {
	h := NewAI(nil, AIOptions{Model: "some-model"}, nil, nil, zerolog.Nop())

	raw, err := h.upstreamBody([]ChatMessage{{Role: "user", Content: "hi"}}, true)
	if err != nil {
		t.Fatalf("upstreamBody: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Sending the key with an empty value is not the same as omitting it:
	// providers that do not support it reject the request outright.
	if _, present := payload["reasoning_effort"]; present {
		t.Fatal("reasoning_effort present although no effort is configured")
	}
	if payload["stream"] != true {
		t.Fatalf("stream = %v, want true", payload["stream"])
	}
	if payload["model"] != "some-model" {
		t.Fatalf("model = %v", payload["model"])
	}
}

func TestUpstreamBodySendsConfiguredReasoningEffort(t *testing.T) {
	h := NewAI(nil, AIOptions{Model: "some-model", ReasoningEffort: "high"}, nil, nil, zerolog.Nop())

	raw, err := h.upstreamBody([]ChatMessage{{Role: "user", Content: "hi"}}, false)
	if err != nil {
		t.Fatalf("upstreamBody: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["reasoning_effort"] != "high" {
		t.Fatalf("reasoning_effort = %v, want high", payload["reasoning_effort"])
	}
	if payload["stream"] != false {
		t.Fatalf("stream = %v, want false", payload["stream"])
	}
}

func TestNewAIFallsBackToDefaultTimeout(t *testing.T) {
	h := NewAI(nil, AIOptions{Model: "m"}, nil, nil, zerolog.Nop())
	if got := h.upstreamTimeout(); got != aiUpstreamDefaultTimeout {
		t.Fatalf("timeout = %s, want %s", got, aiUpstreamDefaultTimeout)
	}

	h = NewAI(nil, AIOptions{Model: "m", RequestTimeout: 42 * time.Second}, nil, nil, zerolog.Nop())
	if got := h.upstreamTimeout(); got != 42*time.Second {
		t.Fatalf("timeout = %s, want 42s", got)
	}
}

func TestNewAITrimsBaseURLSlash(t *testing.T) {
	h := NewAI(nil, AIOptions{BaseURL: "http://example.test:8000/"}, nil, nil, zerolog.Nop())
	if h.opts.BaseURL != "http://example.test:8000" {
		t.Fatalf("base url = %q", h.opts.BaseURL)
	}
}
