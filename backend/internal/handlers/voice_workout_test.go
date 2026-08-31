package handlers

import "testing"

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
