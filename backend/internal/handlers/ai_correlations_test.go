package handlers

import (
	"strings"
	"testing"
)

func seriesInt(v int) *int           { return &v }
func seriesFloat(v float64) *float64 { return &v }
func seriesSeconds(v int64) *int64   { return &v }

func TestRenderDailySeriesKeepsMissingCellsEmpty(t *testing.T) {
	data := AIDailySeriesData{
		Days: 2,
		From: "2026-08-23",
		To:   "2026-08-24",
		Rows: []AIDailySeriesRow{
			{Date: "2026-08-23", SleepMinutes: seriesInt(606), Steps: seriesInt(1821), SpentRub: seriesFloat(136)},
			{Date: "2026-08-24", Workouts: seriesInt(0)},
		},
		Coverage: map[string]int{"sleep_minutes": 1, "steps": 1, "spent_rub": 1, "workouts": 1},
	}

	rendered := renderDailySeriesText(data)
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	last := strings.Split(lines[len(lines)-1], "\t")

	if last[0] != "2026-08-24" {
		t.Fatalf("last row starts with %q", last[0])
	}
	// A day with no reading must stay blank. Rendering it as 0 would let the
	// model read "slept zero minutes" out of "no sleep data".
	if last[1] != "" {
		t.Fatalf("missing sleep rendered as %q, want empty", last[1])
	}
	// A real zero must survive as a zero.
	workouts := last[9]
	if workouts != "0" {
		t.Fatalf("zero workouts rendered as %q, want 0", workouts)
	}
	if !strings.Contains(rendered, "Пустая клетка означает отсутствие данных, а не ноль") {
		t.Fatal("render is missing the note about empty cells")
	}
	if !strings.Contains(rendered, "sleep_minutes=1") {
		t.Fatalf("coverage line is missing sleep_minutes, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "habits_done=0") {
		t.Fatal("coverage line must report zero-coverage columns too")
	}
}

func TestRenderDailySeriesHeaderMatchesCellCount(t *testing.T) {
	data := AIDailySeriesData{
		Days:     1,
		Rows:     []AIDailySeriesRow{{Date: "2026-08-24"}},
		Coverage: map[string]int{},
	}

	lines := strings.Split(strings.TrimRight(renderDailySeriesText(data), "\n"), "\n")
	header := strings.Split(lines[len(lines)-2], "\t")
	row := strings.Split(lines[len(lines)-1], "\t")

	if len(header) != len(row) {
		t.Fatalf("header has %d columns, row has %d", len(header), len(row))
	}
	if len(header) != len(aiDailySeriesColumns) {
		t.Fatalf("header has %d columns, want %d", len(header), len(aiDailySeriesColumns))
	}
}

func TestCountCoverageCountsOnlyPresentValues(t *testing.T) {
	coverage := map[string]int{}
	countCoverage(coverage, AIDailySeriesRow{SleepMinutes: seriesInt(400), Steps: nil, Workouts: seriesInt(0)})
	countCoverage(coverage, AIDailySeriesRow{SleepMinutes: nil, Steps: seriesInt(9000)})

	if coverage["sleep_minutes"] != 1 {
		t.Fatalf("sleep coverage = %d, want 1", coverage["sleep_minutes"])
	}
	if coverage["steps"] != 1 {
		t.Fatalf("steps coverage = %d, want 1", coverage["steps"])
	}
	// A present zero is data, so it counts.
	if coverage["workouts"] != 1 {
		t.Fatalf("workouts coverage = %d, want 1", coverage["workouts"])
	}
}

func TestMinutesFromSecondsRoundsToNearest(t *testing.T) {
	if got := minutesFromSeconds(nil); got != nil {
		t.Fatalf("nil seconds became %v", got)
	}
	if got := minutesFromSeconds(seriesSeconds(90)); got == nil || *got != 2 {
		t.Fatalf("90s = %v, want 2 minutes", got)
	}
	if got := minutesFromSeconds(seriesSeconds(29)); got == nil || *got != 0 {
		t.Fatalf("29s = %v, want 0 minutes", got)
	}
}

func TestRoundedFloatKeepsDecimals(t *testing.T) {
	if got := roundedFloat(seriesFloat(77.34), 1); got == nil || *got != 77.3 {
		t.Fatalf("77.34 to 1 decimal = %v", got)
	}
	if got := roundedFloat(seriesFloat(1774.6), 0); got == nil || *got != 1775 {
		t.Fatalf("1774.6 to 0 decimals = %v", got)
	}
	if got := roundedFloat(nil, 1); got != nil {
		t.Fatalf("nil became %v", got)
	}
}

func TestRoundedIntFromNumeric(t *testing.T) {
	if got := roundedInt(seriesFloat(2460.5041736227045)); got == nil || *got != 2461 {
		t.Fatalf("2460.504 = %v, want 2461", got)
	}
	if got := roundedInt(nil); got != nil {
		t.Fatalf("nil became %v", got)
	}
}
