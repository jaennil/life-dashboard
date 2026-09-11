package handlers

import (
	"context"
	"errors"
	"strings"
	"time"
)

// voiceAnswerStyle is the whole difference between a dictated question and a
// typed one: the answer is read off a notification on a phone, standing
// somewhere, not scrolled in a browser.
const voiceAnswerStyle = `

Этот вопрос задали голосом, ответ придёт уведомлением на телефон.
- Уложись в 2-4 предложения. Перечисление - через запятую, а не списком.
- Без markdown: никаких заголовков, звёздочек, таблиц и переносов ради красоты.
- Начинай с ответа, а не с пересказа вопроса.
- Отвечай числами из данных, а не общими советами.
- Если нужных данных нет, скажи это одной фразой и не додумывай.`

// AnswerQuestion answers one question with the same data context the chat uses,
// and records the exchange in the chat history: a question asked out loud and
// one typed on the site continue the same conversation.
func (h *AIHandler) AnswerQuestion(ctx context.Context, userID, question string) (string, error) {
	// The same kill switch the chat honours: one flag turns the assistant off on
	// every surface, dictation included.
	if h.unleash != nil && !h.unleash.IsEnabled("ai-chat") {
		return "", errors.New("AI чат временно отключён")
	}

	history := h.buildConversationHistory(ctx, userID, nil)

	dataContext, sectionNames, err := h.buildChatContext(ctx, userID, question, history)
	if err != nil {
		// Same trade as the chat: an answer from whatever loaded beats no answer,
		// and the prompt says outright that the data is missing.
		h.logger.Error().Err(err).Str("user_id", userID).Msg("build voice question context")
		dataContext = "Данные пользователя временно недоступны."
		sectionNames = defaultAIContextScope().sectionNames()
	}

	systemPrompt := buildAISystemPromptWithSections(time.Now(), dataContext, sectionNames) + voiceAnswerStyle
	messages := []ChatMessage{{Role: "system", Content: systemPrompt}}
	messages = append(messages, history...)
	messages = append(messages, ChatMessage{Role: "user", Content: question})

	answer, err := h.complete(ctx, "voice_question", messages)
	if err != nil {
		return "", err
	}

	answer = strings.TrimSpace(answer)
	h.storeChatExchange(ctx, userID, question, answer)
	return answer, nil
}
