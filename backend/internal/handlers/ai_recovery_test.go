package handlers

import (
	"strings"
	"testing"
)

func recoveryFixture() AIRecoveryData {
	// The numbers are the live ones from the band for 12.09.
	return AIRecoveryData{
		Readings: map[string]AIRecoveryReading{
			"hrv":                       {Latest: 86, Average: 79.3, Samples: 3},
			"hrv_baseline":              {Latest: 80, Average: 80.7, Samples: 3},
			"readiness_score":           {Latest: 87, Average: 85, Samples: 3},
			"physical_score":            {Latest: 84, Average: 86, Samples: 3},
			"mental_score":              {Latest: 92, Average: 92.7, Samples: 3},
			"stress_avg":                {Latest: 14, Average: 16.7, Samples: 4},
			"stress_max":                {Latest: 63, Average: 60, Samples: 4},
			"stress_relaxed_share":      {Latest: 95, Average: 93, Samples: 4},
			"respiratory_rate":          {Latest: 19, Average: 19, Samples: 1},
			"oxygen_desaturation_index": {Latest: 0.48, Average: 0.3, Samples: 3},
			"pai_total":                 {Latest: 26.8, Average: 30.3, Samples: 4},
			"sleep_resting_heart_rate":  {Latest: 55, Average: 55.7, Samples: 3},
		},
		HRVTrend: []AIRecoveryDay{
			{Date: "12.09", Value: 86}, {Date: "11.09", Value: 77}, {Date: "10.09", Value: 82},
		},
	}
}

func TestRenderRecoveryTextWithoutData(t *testing.T) {
	rendered := renderRecoveryText(AIRecoveryData{Readings: map[string]AIRecoveryReading{}})
	if !strings.Contains(rendered, "нет данных") {
		t.Fatalf("unexpected render: %q", rendered)
	}
	// Without readings there is nothing to head, and a heading alone reads as
	// data that failed to print.
	if strings.Contains(rendered, "оценки браслета") {
		t.Fatalf("empty data produced a heading: %q", rendered)
	}
}

func TestRenderRecoveryText(t *testing.T) {
	rendered := renderRecoveryText(recoveryFixture())

	for _, want := range []string{
		"HRV: последнее 86 ms, среднее за период 79, личная базовая линия 80 (выше своей нормы)",
		"HRV по ночам (от свежей к старой): 12.09 86, 11.09 77, 10.09 82",
		"Готовность: 87 из 100, среднее за период 85 (физическая 84, ментальная 92)",
		"Стресс: средний за период 17, пиковый 63, в расслабленной зоне 93% времени",
		"Частота дыхания: 19 вдохов в минуту",
		"Эпизоды снижения кислорода во сне: 0.3 в час",
		"PAI: 27 при цели 100",
		"Пульс покоя во сне: 55",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("render is missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderRecoverySkipsWhatIsMissing(t *testing.T) {
	// A band that reports HRV but no stress must not produce an empty stress line.
	data := AIRecoveryData{Readings: map[string]AIRecoveryReading{
		"hrv": {Latest: 70, Average: 70, Samples: 1},
	}}
	rendered := renderRecoveryText(data)

	if !strings.Contains(rendered, "HRV: последнее 70 ms") {
		t.Fatalf("hrv missing: %q", rendered)
	}
	for _, absent := range []string{"Стресс", "Готовность", "PAI", "базовая линия"} {
		if strings.Contains(rendered, absent) {
			t.Errorf("%q was rendered without data behind it:\n%s", absent, rendered)
		}
	}
}

func TestRenderRecoveryIgnoresAReadingWithNoSamples(t *testing.T) {
	// A zero-sample row is how the query reports a metric that is not there at
	// all; rendering it would print a confident zero.
	data := AIRecoveryData{Readings: map[string]AIRecoveryReading{
		"hrv":        {Latest: 70, Average: 70, Samples: 1},
		"stress_avg": {Samples: 0},
	}}
	if strings.Contains(renderRecoveryText(data), "Стресс") {
		t.Fatal("a metric with no samples was rendered")
	}
}

func TestCompareToBaselineNeedsARealDifference(t *testing.T) {
	if got := compareToBaseline(86, 80); got != "выше своей нормы" {
		t.Errorf("86 vs 80 = %q", got)
	}
	if got := compareToBaseline(59, 80); got != "ниже своей нормы" {
		t.Errorf("59 vs 80 = %q", got)
	}
	// A millisecond is not a change in recovery.
	if got := compareToBaseline(81, 80); got != "на уровне своей нормы" {
		t.Errorf("81 vs 80 = %q", got)
	}
	if got := compareToBaseline(70, 0); got != "базовая линия неизвестна" {
		t.Errorf("no baseline = %q", got)
	}
}
