package handlers

import (
	"strings"
	"testing"
	"time"
)

func TestFormatSleepStagesAveragesPerStage(t *testing.T) {
	// Real proportions from a month of watch data: "awake" is reported on far
	// fewer nights than the sleep stages themselves.
	rendered := formatSleepStages([]sleepStageTotal{
		{Stage: "light", Minutes: 10991, Nights: 43},
		{Stage: "rem", Minutes: 4902, Nights: 42},
		{Stage: "deep", Minutes: 4079, Nights: 42},
		{Stage: "awake", Minutes: 355, Nights: 14},
	})

	for _, want := range []string{
		"лёгкий 256 мин за ночь",
		"REM 117 мин за ночь",
		"глубокий 97 мин за ночь",
		// 355 minutes over its own 14 nights, not over all 43.
		"пробуждения 25 мин за ночь",
		"ночей 14",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("render is missing %q:\n%s", want, rendered)
		}
	}
}

func TestFormatSleepStagesWithoutData(t *testing.T) {
	for _, totals := range [][]sleepStageTotal{
		nil,
		{{Stage: "deep", Minutes: 0, Nights: 0}},
	} {
		if got := formatSleepStages(totals); !strings.Contains(got, "нет данных") {
			t.Fatalf("unexpected render for %#v: %q", totals, got)
		}
	}
}

func TestSleepStageLabel(t *testing.T) {
	cases := map[string]string{"deep": "глубокий", "light": "лёгкий", "rem": "REM", "awake": "пробуждения", "unknown": "unknown"}
	for stage, want := range cases {
		if got := sleepStageLabel(stage); got != want {
			t.Fatalf("sleepStageLabel(%q) = %q, want %q", stage, got, want)
		}
	}
}

func TestFormatAIAge(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Minute:   "30 мин",
		5 * time.Hour:      "5 ч",
		50 * time.Hour:     "2 дн",
		7 * 24 * time.Hour: "7 дн",
	}
	for age, want := range cases {
		if got := formatAIAge(age); got != want {
			t.Fatalf("formatAIAge(%s) = %q, want %q", age, got, want)
		}
	}
}
