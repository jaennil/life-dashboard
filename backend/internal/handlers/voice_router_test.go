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
	// silently, otherwise a dictated meal looks accepted.
	for _, domain := range []string{voiceDomainFood, voiceDomainNote, voiceDomainWeight} {
		if voiceDomainReplies[domain] == "" {
			t.Errorf("domain %q has no reply", domain)
		}
	}
	if voiceDomainReplies[voiceDomainWorkout] != "" {
		t.Error("workout is implemented and must not carry a not-supported reply")
	}
}

func TestComposeVoiceDisplayShowsWorkoutAndFinish(t *testing.T) {
	got := composeVoiceDisplay(voiceWorkoutResponse{
		Heard:    "на бицепс 16 килограмм 11 раз два подхода, закончить тренировку",
		Workout:  "Bicep Curl (Dumbbell): 11×16кг, 11×16кг",
		Finished: true,
		Title:    "Спина и бицепс",
	})

	for _, want := range []string{"Bicep Curl (Dumbbell): 11×16кг", "Тренировка закончена: Спина и бицепс"} {
		if !strings.Contains(got, want) {
			t.Fatalf("display missing %q:\n%s", want, got)
		}
	}
}

func TestComposeVoiceDisplayShowsRoutingMessage(t *testing.T) {
	got := composeVoiceDisplay(voiceWorkoutResponse{
		Heard:   "съел 200 грамм творога",
		Domain:  voiceDomainFood,
		Message: voiceDomainReplies[voiceDomainFood],
	})

	if !strings.Contains(got, "FatSecret") {
		t.Fatalf("display = %q", got)
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
}
