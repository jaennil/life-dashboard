package handlers

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLooksLikeWorkoutFinishAcceptsSpokenVariants(t *testing.T) {
	finishing := []string{
		"закончить тренировку",
		"Закончить тренировку.",
		"всё, закончил тренировку",
		"тренировка закончена!",
		"конец тренировки",
		"finish workout",
		// Dictation inserts the exotic spaces seen in the Screen Time payloads.
		"закончить тренировку",
		// And sometimes breaks the phrase across lines.
		"закончить\nтренировку",
	}
	for _, text := range finishing {
		if !looksLikeWorkoutFinish(text) {
			t.Errorf("expected %q to end the workout", text)
		}
	}
}

func TestLooksLikeWorkoutFinishIgnoresExercisePhrases(t *testing.T) {
	// The dangerous false positive: an ordinary phrase that happens to mention
	// finishing something would cut the workout short mid-session.
	continuing := []string{
		"5 подтягиваний нейтральным хватом",
		"закончил подход, было тяжело",
		"20 отжиманий от брусьев",
		"lateral raises с двумя гантелями по 13.5кг каждая",
		"на бицепс с гантелей 16кг 11 раз, 2 подхода",
	}
	for _, text := range continuing {
		if looksLikeWorkoutFinish(text) {
			t.Errorf("expected %q to keep the workout open", text)
		}
	}
}

func TestNormalizeVoiceTextCollapsesWhitespaceAndMarks(t *testing.T) {
	cases := map[string]string{
		"  5 подтягиваний  ":       "5 подтягиваний",
		"lateral raises по 13.5кг": "lateral raises по 13.5кг",
		"‎закончить тренировку":    "закончить тренировку",
		"20 отжиманий\nот брусьев": "20 отжиманий от брусьев",
		"3 подхода по 12":          "3 подхода по 12",
	}
	for input, want := range cases {
		if got := normalizeVoiceText(input); got != want {
			t.Errorf("normalizeVoiceText(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeVoiceTextEmptyStaysEmpty(t *testing.T) {
	for _, input := range []string{"", "   ", "​⁠"} {
		if got := normalizeVoiceText(input); got != "" {
			t.Errorf("normalizeVoiceText(%q) = %q, want empty", input, got)
		}
	}
}

func TestStripFinishPhraseKeepsExercisesSpokenAlongsideIt(t *testing.T) {
	cases := map[string]string{
		// The dangerous case: the last phrase carries both a set and the finish.
		"5 подтягиваний, закончить тренировку":           "5 подтягиваний",
		"закончить тренировку":                           "",
		"Закончить тренировку.":                          "",
		"20 отжиманий от брусьев и закончить тренировку": "20 отжиманий от брусьев",
		"3 подхода по 12":                                "3 подхода по 12",
	}
	for input, want := range cases {
		if got := stripFinishPhrase(normalizeVoiceText(input)); got != want {
			t.Errorf("stripFinishPhrase(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSanitizeWorkoutTitleTrimsChattyAnswers(t *testing.T) {
	cases := map[string]string{
		"Спина и бицепс":    "Спина и бицепс",
		"  \"Push-день\"  ": "Push-день",
		"Спина и бицепс\nЭто название по группам": "Спина и бицепс",
		"«Плечи и руки».": "Плечи и руки",
		"":                "",
	}
	for input, want := range cases {
		if got := sanitizeWorkoutTitle(input); got != want {
			t.Errorf("sanitizeWorkoutTitle(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSanitizeWorkoutTitleKeepsCyrillicIntactWhenTruncating(t *testing.T) {
	long := strings.Repeat("плечи ", 40)
	title := sanitizeWorkoutTitle(long)

	if len(title) > 120 {
		t.Fatalf("title is %d bytes, want at most 120", len(title))
	}
	if !utf8.ValidString(title) {
		t.Fatalf("truncation split a rune: %q", title)
	}
}
