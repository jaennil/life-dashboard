package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	authmw "life-dashboard/internal/middleware"
)

const aiHistoryLimit = 200

type AIHistoryMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *AIHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)

	rows, err := h.db.Query(ctx, `
		SELECT id, role, content, created_at
		FROM (
			SELECT id, role, content, created_at
			FROM ai_chat_messages
			WHERE user_id = $1
			ORDER BY created_at DESC
			LIMIT $2
		) history
		ORDER BY
			created_at ASC,
			CASE role
				WHEN 'user' THEN 0
				ELSE 1
			END ASC,
			id ASC
	`, userID, aiHistoryLimit)
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

	createdAt := time.Now().UTC()

	for _, msg := range []struct {
		role    string
		content string
		ts      time.Time
	}{
		{role: "user", content: userMessage, ts: createdAt},
		{role: "assistant", content: assistantMessage, ts: createdAt.Add(time.Millisecond)},
	} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ai_chat_messages (user_id, role, content, created_at)
			VALUES ($1, $2, $3, $4)
		`, userID, msg.role, msg.content, msg.ts); err != nil {
			h.logger.Warn().Err(err).Str("role", msg.role).Msg("store ai history message")
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		h.logger.Warn().Err(err).Msg("commit ai history tx")
	}
}
