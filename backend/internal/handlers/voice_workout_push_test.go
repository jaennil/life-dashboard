package handlers

import (
	"testing"
	"time"
)

func TestPushWindowBracketsTheSession(t *testing.T) {
	started := time.Date(2026, 9, 1, 16, 45, 0, 0, time.UTC)
	finished := time.Date(2026, 9, 1, 19, 56, 0, 0, time.UTC)

	start, end := pushWindow(started, &finished, nil, time.Now())

	if !start.Equal(started) || !end.Equal(finished) {
		t.Fatalf("window = %s .. %s", start, end)
	}
}

func TestPushWindowPrefersASpokenDuration(t *testing.T) {
	// One-shot mode: the whole workout is dictated afterwards, so the first
	// phrase is not when training began.
	started := time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)
	finished := time.Date(2026, 9, 1, 20, 2, 0, 0, time.UTC)
	ninety := 90 * 60

	start, end := pushWindow(started, &finished, &ninety, time.Now())

	if !end.Equal(finished) {
		t.Fatalf("end = %s", end)
	}
	if want := finished.Add(-90 * time.Minute); !start.Equal(want) {
		t.Fatalf("start = %s, want %s", start, want)
	}
}

func TestPushWindowNeverCollapses(t *testing.T) {
	// A session dictated in one breath finishes in the same instant it started.
	at := time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)

	start, end := pushWindow(at, &at, nil, time.Now())

	if !start.Before(end) {
		t.Fatalf("window is not a real interval: %s .. %s", start, end)
	}
	if got := end.Sub(start); got != time.Minute {
		t.Fatalf("span = %s, want a minute", got)
	}
}

func TestPushWindowFallsBackToNowWhenUnfinished(t *testing.T) {
	started := time.Date(2026, 9, 1, 19, 0, 0, 0, time.UTC)
	now := time.Date(2026, 9, 1, 19, 30, 0, 0, time.UTC)

	start, end := pushWindow(started, nil, nil, now)

	if !end.Equal(now) || !start.Equal(started) {
		t.Fatalf("window = %s .. %s", start, end)
	}
}

func TestWorkoutTitleFallsBackToADate(t *testing.T) {
	at := time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)

	if got := workoutTitle(nil, at); got != "Тренировка 01.09.2026" {
		t.Fatalf("title = %q", got)
	}
	empty := ""
	if got := workoutTitle(&empty, at); got != "Тренировка 01.09.2026" {
		t.Fatalf("empty title = %q", got)
	}
	given := "Спина и бицепс"
	if got := workoutTitle(&given, at); got != given {
		t.Fatalf("title = %q, want the generated one", got)
	}
}

func TestToHevyExercisesCarriesEverySetField(t *testing.T) {
	draft := []voiceParsedExercise{
		{TemplateID: "422B08F1", Title: "Lateral Raise (Dumbbell)", Sets: []voiceParsedSet{
			{Type: "normal", Reps: vInt(10), WeightKg: vKg(13.5)},
			{Type: "failure", Reps: vInt(8)},
		}},
		{TemplateID: "1B2B1E7C", Title: "Pull Up", Sets: []voiceParsedSet{{Type: "", Reps: vInt(5)}}},
	}

	converted := toHevyExercises(draft)

	if len(converted) != 2 {
		t.Fatalf("exercises = %d", len(converted))
	}
	if converted[0].TemplateID != "422B08F1" || len(converted[0].Sets) != 2 {
		t.Fatalf("first exercise = %+v", converted[0])
	}
	if converted[0].Sets[0].WeightKg == nil || *converted[0].Sets[0].WeightKg != 13.5 {
		t.Fatalf("weight lost: %+v", converted[0].Sets[0])
	}
	if converted[0].Sets[1].Type != "failure" {
		t.Fatalf("set type lost: %+v", converted[0].Sets[1])
	}
	// A reps_only set must not gain a weight on the way out.
	if converted[1].Sets[0].WeightKg != nil {
		t.Fatalf("weight invented: %+v", converted[1].Sets[0])
	}
}
