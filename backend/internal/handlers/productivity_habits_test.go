package handlers

import (
	"testing"
	"time"
)

func TestNormalizeManualHabitRoutine(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "morning", want: "morning"},
		{input: " evening ", want: "evening"},
		{input: "other", want: "anytime"},
		{input: "day", want: "anytime"},
		{input: "weird", want: ""},
	}

	for _, tt := range tests {
		if got := normalizeManualHabitRoutine(tt.input); got != tt.want {
			t.Fatalf("normalizeManualHabitRoutine(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCompletedStreak(t *testing.T) {
	target := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	statuses := map[string]string{
		"2026-04-20": "completed",
		"2026-04-19": "completed",
		"2026-04-18": "completed",
		"2026-04-17": "none",
	}

	if got := completedStreak(statuses, target); got != 3 {
		t.Fatalf("completedStreak() = %d, want 3", got)
	}
}

