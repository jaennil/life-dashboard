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
				Name:     aiToolFinanceOverview,
				Section:  "финансы",
				Context:  "=== ФИНАНСЫ ===\nБаланс: 100 ₽",
				Duration: 125 * time.Millisecond,
			},
			{
				Name:     aiToolWorkoutOverview,
				Section:  "тренировки",
				Context:  "=== ТРЕНИРОВКИ ===\n2 тренировки",
				Duration: 42 * time.Millisecond,
			},
		},
	}

	rendered := run.render()
	for _, expected := range []string{
		"Ниже результаты внутренних data-tools",
		"--- TOOL finance_overview | SECTION финансы | 125ms ---",
		"=== ФИНАНСЫ ===",
		"--- TOOL workout_overview | SECTION тренировки | 42ms ---",
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
	}
}
