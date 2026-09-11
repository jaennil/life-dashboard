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

func TestLooksLikeQuestionCatchesWhatWouldBeWritten(t *testing.T) {
	// Every phrase here is one the model could plausibly file as food or task,
	// which is exactly the mistake that costs a manual cleanup.
	questions := []string{
		"чем добрать кбжу сейчас?",
		"Чем добрать кбжу сейчас",
		"сколько у меня денег на балансе",
		"что мне сегодня съесть на ужин",
		"че сегодня потренить",
		"посоветуй что приготовить из курицы",
		"стоит ли идти в зал сегодня",
		"хватит ли мне денег до зарплаты",
		"покажи просроченные задачи",
		"купить молоко?",
	}
	for _, phrase := range questions {
		if !looksLikeQuestion(phrase) {
			t.Errorf("looksLikeQuestion(%q) = false", phrase)
		}
	}
}

func TestLooksLikeQuestionLeavesStatementsAlone(t *testing.T) {
	statements := []string{
		"съел борщ 300 грамм",
		"надо забрать посылку на почте",
		"как обычно делал жим лёжа",
		"подтягивания 8 раз три подхода",
		"купить молоко",
		"",
		"   ",
	}
	for _, phrase := range statements {
		if looksLikeQuestion(phrase) {
			t.Errorf("looksLikeQuestion(%q) = true", phrase)
		}
	}
}

func TestRerouteQuestionOnlyOverridesWrites(t *testing.T) {
	// A question the model wanted to log is the case worth guarding.
	if got := rerouteQuestion(voiceDomainFood, "чем добрать кбжу сейчас?"); got != voiceDomainQuestion {
		t.Errorf("food verdict on a question stayed %q", got)
	}
	if got := rerouteQuestion(voiceDomainTask, "что мне сегодня сделать по дому"); got != voiceDomainQuestion {
		t.Errorf("task verdict on a question stayed %q", got)
	}

	// Everything else is left to the model: a workout phrase must survive
	// "как обычно", and an answered note costs nothing to correct.
	if got := rerouteQuestion(voiceDomainWorkout, "сколько раз подтянулся?"); got != voiceDomainWorkout {
		t.Errorf("workout verdict was overridden to %q", got)
	}
	if got := rerouteQuestion(voiceDomainNote, "сколько же дел накопилось"); got != voiceDomainNote {
		t.Errorf("note verdict was overridden to %q", got)
	}
	if got := rerouteQuestion(voiceDomainFood, "съел борщ 300 грамм"); got != voiceDomainFood {
		t.Errorf("a plain meal was rerouted to %q", got)
	}
}

func TestInputNotificationHeaderSeparatesAnswersFromRecords(t *testing.T) {
	cases := []struct {
		result    voiceWorkoutResponse
		wantTitle string
		wantURL   string
	}{
		{voiceWorkoutResponse{Domain: voiceDomainQuestion, Status: "ok"}, "Ответ готов", "/ai"},
		{voiceWorkoutResponse{Domain: voiceDomainQuestion, Status: "failed"}, "Не смог ответить", "/ai"},
		{voiceWorkoutResponse{Domain: voiceDomainFood, Status: "ok"}, "Запись готова", "/input"},
		{voiceWorkoutResponse{Domain: voiceDomainWorkout, Status: "failed"}, "Не удалось обработать запись", "/input"},
	}
	for _, tc := range cases {
		title, url := inputNotificationHeader(tc.result)
		if title != tc.wantTitle || url != tc.wantURL {
			t.Errorf("%s/%s = (%q, %q), want (%q, %q)",
				tc.result.Domain, tc.result.Status, title, url, tc.wantTitle, tc.wantURL)
		}
	}
}
