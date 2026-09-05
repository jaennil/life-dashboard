package handlers

import (
	"strings"
	"testing"
)

func TestSplitTelegramMessageKeepsShortReportsWhole(t *testing.T) {
	chunks := splitTelegramMessage("  короткий отчёт  ", 100)
	if len(chunks) != 1 || chunks[0] != "короткий отчёт" {
		t.Fatalf("unexpected chunks %#v", chunks)
	}
}

func TestSplitTelegramMessageCutsOnParagraphs(t *testing.T) {
	first := strings.Repeat("а", 40)
	second := strings.Repeat("б", 40)
	third := strings.Repeat("в", 40)

	chunks := splitTelegramMessage(first+"\n\n"+second+"\n\n"+third, 100)

	if len(chunks) < 2 {
		t.Fatalf("expected the report to be split, got %d chunk(s)", len(chunks))
	}
	for _, chunk := range chunks {
		if len([]rune(chunk)) > 100 {
			t.Fatalf("chunk over the limit: %d runes", len([]rune(chunk)))
		}
	}
	// Nothing may be lost or reordered in the split.
	joined := strings.Join(chunks, "\n\n")
	for _, part := range []string{first, second, third} {
		if !strings.Contains(joined, part) {
			t.Fatalf("split dropped a paragraph")
		}
	}
	if strings.Index(joined, first) > strings.Index(joined, second) {
		t.Fatalf("split reordered the report")
	}
}

func TestSplitTelegramMessageHandlesTextWithoutBreaks(t *testing.T) {
	// A wall of text still has to fit: cut at the limit rather than refuse.
	chunks := splitTelegramMessage(strings.Repeat("я", 250), 100)
	if len(chunks) != 3 {
		t.Fatalf("expected three chunks, got %d", len(chunks))
	}
	total := 0
	for _, chunk := range chunks {
		runes := len([]rune(chunk))
		if runes > 100 {
			t.Fatalf("chunk over the limit: %d runes", runes)
		}
		total += runes
	}
	if total != 250 {
		t.Fatalf("expected every rune to survive, got %d", total)
	}
}
