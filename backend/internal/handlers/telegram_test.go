package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
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

func TestFormatTelegramTextRendersTheReportMarkup(t *testing.T) {
	// Exactly the shapes a checkup produces.
	report := strings.Join([]string{
		"## 1. Короткий итог",
		"День прошёл при минимальной активности (292 шага).",
		"",
		"## 2. Финансы",
		"- **Текущий баланс:** 35 592 ₽.",
		"- **Доходы:** 2 000 ₽; **расходы:** 1 272 ₽.",
		"* Свободных средств ~33 764 ₽.",
	}, "\n")

	got := formatTelegramText(report, true)

	for _, want := range []string{
		"<b>1. Короткий итог</b>",
		"<b>2. Финансы</b>",
		"• <b>Текущий баланс:</b> 35 592 ₽.",
		"• <b>Доходы:</b> 2 000 ₽; <b>расходы:</b> 1 272 ₽.",
		"• Свободных средств ~33 764 ₽.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	// Nothing of the raw markup may survive.
	for _, unwanted := range []string{"##", "**", "\n- "} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("raw markup %q survived:\n%s", unwanted, got)
		}
	}
}

func TestFormatTelegramTextEscapesBeforeAddingTags(t *testing.T) {
	got := formatTelegramText("**Итог:** 5 < 10 & <b>чужой тег</b>", true)

	if !strings.Contains(got, "<b>Итог:</b>") {
		t.Fatalf("our own tag is missing: %q", got)
	}
	if !strings.Contains(got, "5 &lt; 10 &amp;") {
		t.Fatalf("text was not escaped: %q", got)
	}
	// A tag that came from the text must arrive as text, not as markup.
	if strings.Contains(got, "<b>чужой тег</b>") {
		t.Fatalf("a tag from the content survived: %q", got)
	}
}

func TestFormatTelegramTextLeavesOddMarkupAlone(t *testing.T) {
	// An unpaired ** is literal text, not a broken tag.
	got := formatTelegramText("итог ** без пары", true)
	if strings.Contains(got, "<b>") {
		t.Fatalf("unpaired markup produced a tag: %q", got)
	}
}

func TestFormatTelegramTextPlainFallbackDropsMarkup(t *testing.T) {
	got := formatTelegramText("## Итог\n- **Баланс:** 10 ₽\n`code`", false)

	for _, unwanted := range []string{"<b>", "##", "**", "`"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("plain fallback kept %q: %q", unwanted, got)
		}
	}
	for _, want := range []string{"Итог", "• Баланс: 10 ₽", "code"} {
		if !strings.Contains(got, want) {
			t.Fatalf("plain fallback lost %q: %q", want, got)
		}
	}
}

func TestIsTelegramParseError(t *testing.T) {
	parse := errors.New(`telegram sendMessage failed: Bad Request: can't parse entities: Unsupported start tag "b" at byte offset 42`)
	if !isTelegramParseError(parse) {
		t.Fatalf("expected a parse failure to be retryable as plain text")
	}

	// A revoked token or a blocked chat fails identically the second time.
	for _, err := range []error{
		errors.New("telegram sendMessage failed: Unauthorized"),
		errors.New("telegram sendMessage failed: Forbidden: bot was blocked by the user"),
		nil,
	} {
		if isTelegramParseError(err) {
			t.Fatalf("unexpectedly treated %v as a parse failure", err)
		}
	}
}

func TestTelegramErrorNeverCarriesTheToken(t *testing.T) {
	// The token sits in the path of every Telegram URL, so a plain transport
	// error prints it. It reached the log store this way, six times in a morning,
	// from nothing worse than a timeout.
	const token = "8000000000:AAHsecretsecretsecretsecretsecretsec"

	client := &telegramClient{
		// A port nothing listens on: the request fails inside the transport, which
		// is the path that used to print the URL.
		baseURL: "http://127.0.0.1:1",
		token:   token,
		http:    &http.Client{Timeout: 2 * time.Second},
	}

	err := client.call(context.Background(), "getUpdates", map[string]any{"timeout": 1}, nil)
	if err == nil {
		t.Fatal("expected the call to fail")
	}
	if strings.Contains(err.Error(), token) {
		t.Errorf("the token is in the error: %s", err)
	}
	if !strings.Contains(err.Error(), "getUpdates") {
		t.Errorf("the error no longer says what failed: %s", err)
	}
}
