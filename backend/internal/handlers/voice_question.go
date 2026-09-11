package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// A dictated question runs the whole chat pipeline - planner, data tools, then
// the reasoning model - so it cannot live inside the extraction budget one
// attempt gets. It stays under the job lease, so the answer is written before
// another worker may reclaim the job.
const voiceQuestionBudget = 4 * time.Minute

// voiceAnswerDisplayRunes bounds what travels into the push payload. The full
// answer is kept in the chat history, so the cut costs nothing.
const voiceAnswerDisplayRunes = 900

// errVoiceAnswerFailed marks a question that could not be answered. It is
// deliberately not retried: the queue's next attempt is an hour away, and an
// hour-old answer to "что сегодня потренить" is worse than an error the user
// can act on by asking again.
var errVoiceAnswerFailed = errors.New("voice question not answered")

// answerQuestion answers a phrase that turned out to be a question rather than
// something to record.
func (h *VoiceWorkoutHandler) answerQuestion(ctx context.Context, userID, question string, response *voiceWorkoutResponse) error {
	if h.ai == nil {
		response.Message = "Похоже на вопрос, но AI не настроен. Фраза сохранена."
		return nil
	}

	// Detached from the attempt deadline on purpose: see voiceQuestionBudget.
	// Losing the worker's shutdown signal is harmless here - the job is simply
	// reclaimed after its lease and asked again.
	answerCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), voiceQuestionBudget)
	defer cancel()

	answer, err := h.ai.AnswerQuestion(answerCtx, userID, question)
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", userID).Msg("answer dictated question")
		response.Message = "Не смог ответить: " + err.Error()
		return fmt.Errorf("%w: %v", errVoiceAnswerFailed, err)
	}

	response.Answer = truncateVoiceAnswer(answer)
	h.logger.Info().Str("user_id", userID).Int("answer_runes", utf8.RuneCountInString(answer)).
		Msg("dictated question answered")
	return nil
}

func truncateVoiceAnswer(answer string) string {
	answer = strings.TrimSpace(answer)
	runes := []rune(answer)
	if len(runes) <= voiceAnswerDisplayRunes {
		return answer
	}
	return strings.TrimRight(strings.TrimSpace(string(runes[:voiceAnswerDisplayRunes])), ",.;:-") +
		"…\nПолный ответ в чате."
}

// voiceQuestionOpeners are the words a phrase starts with when it asks for
// something rather than reports it. They are deliberately narrow: "как" opens
// both a question and "как обычно делал жим", so it is not here.
var voiceQuestionOpeners = []string{
	"сколько", "чем ", "чего ", "почему", "зачем", "когда ", "где ", "куда ",
	"какой", "какая", "какое", "какие", "каков",
	"стоит ли", "надо ли", "нужно ли", "можно ли", "успею ли", "хватит ли",
	"что мне", "что сегодня", "что лучше", "что выбрать", "что делать",
	"че сегодня", "чё сегодня", "че мне", "чё мне", "че лучше", "чё лучше",
	"подскажи", "посоветуй", "посчитай", "проанализируй", "оцени", "сравни",
	"покажи", "расскажи",
}
