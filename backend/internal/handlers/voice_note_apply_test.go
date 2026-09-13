package handlers

import (
	"strings"
	"testing"
	"time"
)

func mustJournalDate(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestAIJournalSourceLabelNamesADictatedNote(t *testing.T) {
	if got := aiJournalSourceLabel(voiceNoteSource); got != "(надиктовано)" {
		t.Errorf("voice = %q", got)
	}
	if got := aiJournalSourceLabel(""); got != "(без названия)" {
		t.Errorf("no source = %q", got)
	}
	if got := aiJournalSourceLabel("notion"); got != "(без названия, notion)" {
		t.Errorf("notion = %q", got)
	}
}

func TestTruncateRunesCutsLettersNotBytes(t *testing.T) {
	// Cyrillic is two bytes per letter: a byte cut at 11 would split the sixth.
	if got := truncateRunes("надиктованная заметка", 11); got != "надиктованн..." {
		t.Fatalf("got %q", got)
	}
	if strings.ContainsRune(truncateRunes(strings.Repeat("я", 50), 7), '�') {
		t.Fatal("truncation produced a broken rune")
	}
	if got := truncateRunes("короткая", 100); got != "короткая" {
		t.Errorf("short text was changed: %q", got)
	}
	if got := truncateRunes("без лимита", 0); got != "без лимита" {
		t.Errorf("a zero limit truncated: %q", got)
	}
}

func TestFormatAIJournalEntryShowsADictatedNote(t *testing.T) {
	entry := aiJournalEntry{
		Date:    mustJournalDate(t, "2026-09-13"),
		Content: "сегодня наконец разобрался с местоположением",
		Source:  voiceNoteSource,
	}

	formatted := formatAIJournalEntryWithLimit(entry, 0)
	if !strings.Contains(formatted, "13.09.2026: (надиктовано)") {
		t.Fatalf("entry header = %q", formatted)
	}
	if !strings.Contains(formatted, "сегодня наконец разобрался") {
		t.Fatalf("content missing: %q", formatted)
	}
}
