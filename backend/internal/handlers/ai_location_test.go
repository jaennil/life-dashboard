package handlers

import (
	"strings"
	"testing"
)

func TestRenderLocationOverviewWithoutData(t *testing.T) {
	rendered := renderLocationOverviewText("=== МЕСТОПОЛОЖЕНИЕ ===", AILocationOverviewData{Days: 14})
	if !strings.Contains(rendered, "Нет данных о местоположении") {
		t.Fatalf("unexpected render: %q", rendered)
	}
	// Without visits, an hours breakdown of all zeros would read as "was nowhere".
	if strings.Contains(rendered, "Время по типам мест") {
		t.Fatalf("empty data rendered a breakdown: %q", rendered)
	}
}

func TestRenderLocationOverviewText(t *testing.T) {
	data := AILocationOverviewData{
		Days: 7, TrackedDays: 6, TotalVisits: 14,
		HomeHours: 92, WorkHours: 38, OtherHours: 6,
		Places: []AILocationPlace{
			{Name: "", Kind: placeKindHome, Hours: 92, Visits: 6, Days: 6},
			{Name: "", Kind: placeKindWork, Hours: 38, Visits: 5, Days: 5},
			{Name: "Ашан", Kind: placeKindOther, Hours: 1.5, Visits: 2, Days: 2},
		},
		RecentDays: []AILocationDay{{
			Date: "2026-09-11",
			Visits: []AILocationVisit{
				{Place: "", Kind: placeKindHome, From: "00:00", To: "09:12", Minutes: 552},
				{Place: "", Kind: placeKindWork, From: "10:14", To: "19:40", Minutes: 566},
				{Place: "Ашан", Kind: placeKindOther, From: "20:05", To: "20:45", Minutes: 40},
				{Place: "", Kind: placeKindHome, From: "21:03", Minutes: 0},
			},
		}},
	}

	rendered := renderLocationOverviewText("=== МЕСТОПОЛОЖЕНИЕ (7 дней) ===", data)

	for _, want := range []string{
		"=== МЕСТОПОЛОЖЕНИЕ (7 дней) ===",
		"Дней с данными: 6, визитов: 14",
		"дом 92 ч, работа 38 ч, прочее 6 ч",
		"  - дом: 92 ч, визитов 6 за 6 дн.",
		"  - Ашан: 2 ч, визитов 2 за 2 дн.",
		"11.09:",
		"  - работа: 10:14-19:40 (9 ч 26 мин)",
		"  - Ашан: 20:05-20:45 (40 мин)",
		// An open visit has no end yet, and saying so beats inventing one.
		"  - дом с 21:03, ещё там",
		// The caveat travels with the data: a quiet day is a quiet phone.
		"пропуск в днях означает",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("render is missing %q:\n%s", want, rendered)
		}
	}
}

func TestFormatAIDurationReadsLikeSpeech(t *testing.T) {
	cases := map[float64]string{
		5:   "5 мин",
		59:  "59 мин",
		60:  "1 ч",
		566: "9 ч 26 мин",
		120: "2 ч",
	}
	for minutes, want := range cases {
		if got := formatAIDuration(minutes); got != want {
			t.Errorf("formatAIDuration(%.0f) = %q, want %q", minutes, got, want)
		}
	}
}

func TestAIPlaceLabelFallsBackToTheKind(t *testing.T) {
	if got := aiPlaceLabel("Ашан", placeKindOther); got != "Ашан" {
		t.Errorf("a named place lost its name: %q", got)
	}
	if got := aiPlaceLabel("", placeKindHome); got != "дом" {
		t.Errorf("unnamed home = %q", got)
	}
	if got := aiPlaceLabel("  ", placeKindWork); got != "работа" {
		t.Errorf("unnamed work = %q", got)
	}
	if got := aiPlaceLabel("", placeKindOther); got != "место без названия" {
		t.Errorf("unnamed other = %q", got)
	}
}

func TestRenderLocationOverviewMarksAnUnknownArrival(t *testing.T) {
	// Real shape from the first day of tracking: iOS reported a departure for a
	// visit whose start it never saw, and the row stores both at the same second.
	rendered := renderLocationOverviewText("=== МЕСТОПОЛОЖЕНИЕ ===", AILocationOverviewData{
		TrackedDays: 1, TotalVisits: 1,
		RecentDays: []AILocationDay{{
			Date:   "2026-09-12",
			Visits: []AILocationVisit{{Place: "Белопольского", Kind: placeKindOther, From: "12:36", To: "12:36", Minutes: 0}},
		}},
	})

	if !strings.Contains(rendered, "ушёл в 12:36, время прихода неизвестно") {
		t.Fatalf("an unknown arrival was not marked:\n%s", rendered)
	}
	if strings.Contains(rendered, "12:36-12:36") {
		t.Fatalf("a stay of no length was reported as a fact:\n%s", rendered)
	}
}
