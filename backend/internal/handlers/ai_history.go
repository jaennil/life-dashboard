package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	authmw "life-dashboard/internal/middleware"
)

const (
	aiHistoryLimit        = 200
	aiHistoryContextLimit = 24
)

type AIHistoryMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *AIHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	now := time.Now().In(aiDisplayLocation)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, aiDisplayLocation)
	dateRange := parseQueryDateRange(r, todayStart.AddDate(0, 0, -29), todayStart)

	rows, err := h.db.Query(ctx, `
		SELECT id, role, content, created_at
		FROM (
			SELECT id, role, content, created_at, message_order
			FROM ai_chat_messages
			WHERE user_id = $1
				AND created_at >= $3
				AND created_at < $4
			ORDER BY message_order DESC
			LIMIT $2
		) history
		ORDER BY message_order ASC
	`, userID, aiHistoryLimit, dateRange.Start, dateRange.EndExclusive)
	if err != nil {
		h.logger.Error().Err(err).Msg("query ai history")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	messages := make([]AIHistoryMessage, 0, aiHistoryLimit)
	for rows.Next() {
		var msg AIHistoryMessage
		if err := rows.Scan(&msg.ID, &msg.Role, &msg.Content, &msg.CreatedAt); err != nil {
			continue
		}
		messages = append(messages, msg)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(messages); err != nil {
		h.logger.Error().Err(err).Msg("write ai history")
	}
}

func (h *AIHandler) ClearHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)

	if _, err := h.db.Exec(ctx, `DELETE FROM ai_chat_messages WHERE user_id = $1`, userID); err != nil {
		h.logger.Error().Err(err).Msg("clear ai history")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AIHandler) loadRecentChatMessages(ctx context.Context, userID string, limit int) ([]ChatMessage, error) {
	if limit <= 0 {
		limit = aiHistoryContextLimit
	}

	rows, err := h.db.Query(ctx, `
		SELECT role, content
		FROM (
			SELECT role, content, message_order
			FROM ai_chat_messages
			WHERE user_id = $1
			ORDER BY message_order DESC
			LIMIT $2
		) history
		ORDER BY message_order ASC
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]ChatMessage, 0, limit)
	for rows.Next() {
		var msg ChatMessage
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			return nil, err
		}
		if normalized := normalizeChatMessage(msg); normalized != nil {
			messages = append(messages, *normalized)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

func (h *AIHandler) storeChatExchange(ctx context.Context, userID, userMessage, assistantMessage string) {
	userMessage = strings.TrimSpace(userMessage)
	assistantMessage = strings.TrimSpace(assistantMessage)
	if userMessage == "" || assistantMessage == "" {
		return
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		h.logger.Warn().Err(err).Msg("begin ai history tx")
		return
	}
	defer tx.Rollback(ctx)

	for _, msg := range []struct {
		role    string
		content string
	}{
		{role: "user", content: userMessage},
		{role: "assistant", content: assistantMessage},
	} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ai_chat_messages (user_id, role, content)
			VALUES ($1, $2, $3)
		`, userID, msg.role, msg.content); err != nil {
			h.logger.Warn().Err(err).Str("role", msg.role).Msg("store ai history message")
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		h.logger.Warn().Err(err).Msg("commit ai history tx")
	}
}
