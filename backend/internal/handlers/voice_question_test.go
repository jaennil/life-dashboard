package handlers

import (
	"strings"
	"testing"
)

func TestTruncateVoiceAnswerKeepsShortAnswersIntact(t *testing.T) {
	answer := "На балансе 42 300 рублей."
	if got := truncateVoiceAnswer("  " + answer + "\n"); got != answer {
		t.Fatalf("truncateVoiceAnswer trimmed wrong: %q", got)
	}
}

func TestTruncateVoiceAnswerBoundsThePushPayload(t *testing.T) {
	// Cyrillic on purpose: a byte-based cut would slice a rune in half here.
	long := strings.Repeat("я", voiceAnswerDisplayRunes+200)
	got := truncateVoiceAnswer(long)

	if !strings.HasSuffix(got, "Полный ответ в чате.") {
		t.Fatalf("truncation did not say where the rest is: %q", got[len(got)-40:])
	}
	if !strings.HasPrefix(got, strings.Repeat("я", 100)) {
		t.Fatalf("truncation mangled the start: %q", got[:40])
	}
	if strings.ContainsRune(got, '�') {
		t.Fatal("truncation cut a rune in half")
	}
}

func TestComposeVoiceDisplayShowsTheAnswer(t *testing.T) {
	got := composeVoiceDisplay(voiceWorkoutResponse{
		Heard:  "сколько у меня на балансе",
		Domain: voiceDomainQuestion,
		Answer: "На картах 42 300 рублей.",
	})

	if !strings.Contains(got, "Услышал: сколько у меня на балансе") {
		t.Fatalf("transcript missing: %q", got)
	}
	if !strings.Contains(got, "На картах 42 300 рублей.") {
		t.Fatalf("answer missing: %q", got)
	}
	// An answered question is not "nothing understood".
	if strings.Contains(got, "Ничего не разобрал") {
		t.Fatalf("an answer was reported as nothing: %q", got)
	}
}
