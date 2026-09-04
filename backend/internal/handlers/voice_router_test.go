package handlers

import (
	"strings"
	"testing"
)

func TestResolveVoiceDomainTrustsAClaimedDomain(t *testing.T) {
	for _, domain := range []string{"workout", "food", "note", "weight"} {
		if got := resolveVoiceDomain(domain, false); got != domain {
			t.Errorf("resolveVoiceDomain(%q, false) = %q", domain, got)
		}
	}
	if got := resolveVoiceDomain("  FOOD  ", true); got != voiceDomainFood {
		t.Fatalf("case and spacing broke routing: %q", got)
	}
}

func TestResolveVoiceDomainFallsBackToOpenWorkout(t *testing.T) {
	// The whole reason the router exists: "ещё 8" is meaningless alone but is a
	// set while a workout is in progress.
	if got := resolveVoiceDomain("", true); got != voiceDomainWorkout {
		t.Fatalf("with an open workout, an unlabelled phrase went to %q", got)
	}
	if got := resolveVoiceDomain("непонятно", true); got != voiceDomainWorkout {
		t.Fatalf("with an open workout, an unknown label went to %q", got)
	}
}

func TestResolveVoiceDomainStaysUnknownWithoutContext(t *testing.T) {
	// Without an open workout there is nothing to lean on, and guessing "workout"
	// would silently open one from an unrelated phrase.
	if got := resolveVoiceDomain("", false); got != voiceDomainUnknown {
		t.Fatalf("resolveVoiceDomain(\"\", false) = %q, want unknown", got)
	}
	if got := resolveVoiceDomain("что-то ещё", false); got != voiceDomainUnknown {
		t.Fatalf("unknown label without context = %q, want unknown", got)
	}
}

func TestUnimplementedDomainsHaveAReply(t *testing.T) {
	// A recognized-but-unwired domain must say so rather than fall through
	// silently, otherwise a dictated note looks accepted.
	for _, domain := range []string{voiceDomainNote, voiceDomainWeight} {
		if voiceDomainReplies[domain] == "" {
			t.Errorf("domain %q has no reply", domain)
		}
	}
	// Workout and food are implemented: they report what they actually did, and a
	// canned "not supported" line would contradict the write that just happened.
	for _, domain := range []string{voiceDomainWorkout, voiceDomainFood} {
		if voiceDomainReplies[domain] != "" {
			t.Errorf("implemented domain %q still carries a not-supported reply", domain)
		}
	}
}

func TestComposeVoiceDisplayShowsWorkoutAndFinish(t *testing.T) {
	got := composeVoiceDisplay(voiceWorkoutResponse{
		Heard:    "на бицепс 16 килограмм 11 раз два подхода, закончить тренировку",
		Workout:  "Bicep Curl (Dumbbell): 11×16кг, 11×16кг",
		Finished: true,
		Title:    "Спина и бицепс",
	})

	for _, want := range []string{
		// The transcript comes first: it is the only way to tell an iOS
		// mishearing from a model misparse.
		"Услышал: на бицепс 16 килограмм 11 раз два подхода, закончить тренировку",
		"Bicep Curl (Dumbbell): 11×16кг",
		"Тренировка закончена: Спина и бицепс",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("display missing %q:\n%s", want, got)
		}
	}
}

func TestComposeVoiceDisplayShowsRoutingMessage(t *testing.T) {
	got := composeVoiceDisplay(voiceWorkoutResponse{
		Heard:   "надо купить кроссовки",
		Domain:  voiceDomainNote,
		Message: voiceDomainReplies[voiceDomainNote],
	})

	if !strings.Contains(got, "Дневник пока не подключён") {
		t.Fatalf("display = %q", got)
	}
}

func TestComposeVoiceDisplayLabelsTypedInput(t *testing.T) {
	got := composeVoiceDisplay(voiceWorkoutResponse{
		typed:   true,
		Heard:   "лимонад с витаминами",
		Domain:  voiceDomainFood,
		Message: "Записал в дневник.",
	})

	if !strings.HasPrefix(got, "Введено: лимонад с витаминами\n") {
		t.Fatalf("typed display has the wrong label: %q", got)
	}
}

func TestComposeVoiceDisplayShowsWhatWasEaten(t *testing.T) {
	got := composeVoiceDisplay(voiceWorkoutResponse{
		Heard:   "молоко 200 грамм",
		Domain:  voiceDomainFood,
		Message: "Записал в дневник.",
		Food:    "Молоко 3,2%: 2 × 100 г, 120 ккал [завтрак]",
	})

	for _, want := range []string{"Услышал: молоко 200 грамм", "Записал в дневник.", "120 ккал [завтрак]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("display missing %q:\n%s", want, got)
		}
	}
}

func TestComposeVoiceDisplayReportsUnmatched(t *testing.T) {
	got := composeVoiceDisplay(voiceWorkoutResponse{
		Heard:     "жал что-то непонятное",
		Workout:   "Pull Up: 5",
		Unmatched: []string{"что-то непонятное (неизвестное упражнение)"},
	})

	if !strings.Contains(got, "Не понял: что-то непонятное") {
		t.Fatalf("display = %q", got)
	}
}

func TestComposeVoiceDisplayNeverBlank(t *testing.T) {
	// A blank result sheet is indistinguishable from a broken shortcut.
	got := composeVoiceDisplay(voiceWorkoutResponse{Heard: "мгмгм"})
	if got == "" {
		t.Fatal("display is empty")
	}
	if !strings.Contains(got, "мгмгм") {
		t.Fatalf("display does not echo what was heard: %q", got)
	}
	if !strings.Contains(got, "Ничего не разобрал") {
		t.Fatalf("display does not admit it understood nothing: %q", got)
	}
}

func TestComposeVoiceDisplayLeadsWithTheTranscript(t *testing.T) {
	// The case this is for: iOS heard "3.5" instead of "13.5". The parse is
	// faithful to a wrong transcript, so only the first line reveals whose
	// mistake it was.
	got := composeVoiceDisplay(voiceWorkoutResponse{
		Heard:   "lateral raises по 3.5 килограмма 12 раз",
		Workout: "Lateral Raise (Dumbbell): 12×3.5кг",
	})

	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("display = %q", got)
	}
	if !strings.HasPrefix(lines[0], "Услышал: ") {
		t.Fatalf("first line is not the transcript: %q", lines[0])
	}
	if !strings.Contains(lines[1], "12×3.5кг") {
		t.Fatalf("second line is not the parse: %q", lines[1])
	}
}
