package handlers

import (
	"context"
	"strings"
	"time"
)

// voiceNoteSource marks entries that were spoken rather than written in Notion.
// They share a table so that everything the AI reads about the journal is in one
// place, and the source is what tells them apart afterwards.
const voiceNoteSource = "voice"

// applyNote files a dictated thought into the journal.
//
// Nothing is extracted from it and nothing is summarised: a thought said out
// loud is worth exactly its own wording, and a model paraphrasing it would
// replace the one thing the entry is for. The phrase is stored as it was heard.
func (h *VoiceWorkoutHandler) applyNote(ctx context.Context, userID, eventID, text string, response *voiceWorkoutResponse) {
	content := strings.TrimSpace(text)
	if content == "" {
		response.Message = "Похоже на заметку, но записывать нечего."
		return
	}

	// The archived phrase is the natural identity of the entry: one phrase is one
	// entry, and a retried job updates the same row rather than adding a second.
	externalID := strings.TrimSpace(eventID)
	if externalID == "" {
		externalID = voiceNoteSource + "-" + time.Now().UTC().Format(time.RFC3339Nano)
	}

	now := time.Now()
	day := now.In(aiDisplayLocation).Format("2006-01-02")

	if _, err := h.db.Exec(ctx, `
		INSERT INTO journal_entries (user_id, source, external_id, date, content, tags, created_at, updated_at)
		VALUES ($1, $2, $3, $4::date, $5, '{}'::text[], $6, $6)
		ON CONFLICT (user_id, source, external_id) DO UPDATE SET
			content = EXCLUDED.content,
			date = EXCLUDED.date,
			updated_at = EXCLUDED.updated_at,
			ingested_at = NOW()
	`, userID, voiceNoteSource, externalID, day, content, now); err != nil {
		h.logger.Error().Err(err).Str("user_id", userID).Msg("store dictated note")
		response.Message = "Не смог записать в дневник: " + err.Error()
		return
	}

	h.logger.Info().Str("user_id", userID).Int("runes", len([]rune(content))).Msg("note written to the journal")
	response.Note = "Записал в дневник."
}
