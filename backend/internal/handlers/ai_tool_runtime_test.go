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
				Data: map[string]any{
					"current_balance_rub": 100.0,
					"transaction_count":   3,
				},
				DurationMs: 125,
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
		"\"data\": {",
		"\"current_balance_rub\": 100",
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
	if len(executions) != 13 {
		t.Fatalf("expected 13 executions, got %d", len(executions))
	}
	// Financial literacy is judged over its own six months, so like the daily
	// series it does not claim the report window.
	literacy := executions[len(executions)-1]
	if literacy.Name != aiToolFinanceLiteracy || literacy.RequestedPeriod != "" {
		t.Fatalf("expected finance literacy last and without a period, got %q %q", literacy.Name, literacy.RequestedPeriod)
	}

	// The daily series is the one execution that does not follow the report
	// window: it always spans its own 90 days, so it carries neither the
	// requested period nor the window boundaries.
	if executions[0].Name != aiToolDailySeries {
		t.Fatalf("expected the daily series first, got %q", executions[0].Name)
	}
	if executions[0].Section != "сводный дневной ряд" {
		t.Fatalf("daily series section = %q", executions[0].Section)
	}
	if executions[0].Days != aiDailySeriesDefaultDays {
		t.Fatalf("daily series days = %d, want %d", executions[0].Days, aiDailySeriesDefaultDays)
	}
	if executions[0].RequestedPeriod != "" {
		t.Fatalf("daily series must not claim the report period, got %q", executions[0].RequestedPeriod)
	}
	// The window is anchored to now rather than to the report period, so the
	// assertion is on its span: the test period itself sits in the past.
	if executions[0].Start == nil || executions[0].End == nil {
		t.Fatal("daily series is missing its window")
	}
	span := executions[0].End.Sub(*executions[0].Start)
	wantSpan := time.Duration(aiDailySeriesDefaultDays-1) * 24 * time.Hour
	if span < wantSpan-time.Hour || span > wantSpan+time.Hour {
		t.Fatalf("daily series spans %s, want about %s", span, wantSpan)
	}
	if executions[0].Run == nil || executions[0].Data == nil {
		t.Fatal("daily series is missing its run or data func")
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
		{aiToolScreenTimeOverview, "экранное время"},
		{aiToolLocationOverview, "местоположение"},
		{aiToolJournalOverview, "дневник"},
		{aiToolCalendarOverview, "календарь"},
	}

	for offset, item := range expected {
		i := offset + 1
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

func TestDailyCheckupSkipsFinanceLiteracy(t *testing.T) {
	handler := &AIHandler{}
	for _, period := range []string{checkupPeriodToday, checkupPeriodYesterday} {
		executions := handler.checkupToolExecutions(context.Background(), "user-1", checkupWindow{RequestedPeriod: period})
		for _, execution := range executions {
			if execution.Name == aiToolFinanceLiteracy {
				t.Fatalf("%s checkup must not judge financial habits", period)
			}
		}
	}
}

func TestAIToolRunRenderSupportsStructuredDataWithoutText(t *testing.T) {
	run := aiToolRun{
		Results: []aiToolResult{
			{
				Name:          aiToolProductivityOverview,
				Section:       "продуктивность",
				ContextFormat: "none",
				Data: map[string]any{
					"active_total":  5,
					"overdue_total": 2,
				},
				DurationMs: 15,
			},
		},
	}

	rendered := run.render()
	for _, expected := range []string{
		"\"tool\": \"productivity_overview\"",
		"\"context_format\": \"none\"",
		"\"active_total\": 5",
		"\"overdue_total\": 2",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected rendered output to contain %q, got:\n%s", expected, rendered)
		}
	}
}
