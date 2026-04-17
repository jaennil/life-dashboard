package handlers

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestAIToolRunRenderIncludesToolMetadata(t *testing.T) {
	run := aiToolRun{
		Results: []aiToolResult{
			{
				Name:          aiToolFinanceOverview,
				Section:       "финансы",
				ContextFormat: "plain_text_v1",
				ContextText:   "=== ФИНАНСЫ ===\nБаланс: 100 ₽",
				DurationMs:    125,
			},
			{
				Name:          aiToolWorkoutOverview,
				Section:       "тренировки",
				ContextFormat: "plain_text_v1",
				ContextText:   "=== ТРЕНИРОВКИ ===\n2 тренировки",
				DurationMs:    42,
			},
		},
	}

	rendered := run.render()
	for _, expected := range []string{
		"Ниже результаты внутренних data-tools в JSON",
		"\"tool_results\":",
		"\"tool\": \"finance_overview\"",
		"\"section\": \"финансы\"",
		"\"context_format\": \"plain_text_v1\"",
		"\"duration_ms\": 125",
		"=== ФИНАНСЫ ===",
		"\"tool\": \"workout_overview\"",
		"\"section\": \"тренировки\"",
		"\"duration_ms\": 42",
		"=== ТРЕНИРОВКИ ===",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected rendered output to contain %q, got:\n%s", expected, rendered)
		}
	}
}

func TestCheckupToolExecutionsOrder(t *testing.T) {
	handler := &AIHandler{}
	window := checkupWindow{
		RequestedPeriod: checkupPeriodWeek,
		Start:           time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		End:             time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC),
	}

	executions := handler.checkupToolExecutions(context.Background(), "user-1", window)
	if len(executions) != 9 {
		t.Fatalf("expected 9 executions, got %d", len(executions))
	}

	expected := []struct {
		name    aiToolName
		section string
	}{
		{aiToolFinanceOverview, "финансы"},
		{aiToolProductivityOverview, "продуктивность"},
		{aiToolHealthOverview, "здоровье"},
		{aiToolActivityOverview, "активности"},
		{aiToolWorkoutOverview, "тренировки"},
		{aiToolNutritionOverview, "питание"},
		{aiToolHabitOverview, "привычки"},
		{aiToolJournalOverview, "дневник"},
		{aiToolCalendarOverview, "календарь"},
	}

	for i, item := range expected {
		if executions[i].Name != item.name {
			t.Fatalf("expected execution %d to be %q, got %q", i, item.name, executions[i].Name)
		}
		if executions[i].Section != item.section {
			t.Fatalf("expected execution %d section %q, got %q", i, item.section, executions[i].Section)
		}
		if executions[i].Run == nil {
			t.Fatalf("expected execution %d to have run func", i)
		}
		if executions[i].RequestedPeriod != checkupPeriodWeek {
			t.Fatalf("expected execution %d requested period %q, got %q", i, checkupPeriodWeek, executions[i].RequestedPeriod)
		}
		if executions[i].Start == nil || !executions[i].Start.Equal(window.Start) {
			t.Fatalf("expected execution %d start to be propagated", i)
		}
		if executions[i].End == nil || !executions[i].End.Equal(window.End) {
			t.Fatalf("expected execution %d end to be propagated", i)
		}
	}
}
