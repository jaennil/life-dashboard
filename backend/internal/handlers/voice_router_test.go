package handlers

import "testing"

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
