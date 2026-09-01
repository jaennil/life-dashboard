package handlers

import (
	"context"
	"encoding/json"
	"time"

	"life-dashboard/internal/connectors"
)

// foodWriter is the narrow part of the FatSecret connector this handler needs.
type foodWriter interface {
	CreateFoodEntry(ctx context.Context, userID string, draft connectors.FoodEntryDraft) (string, error)
}

// applyFood validates the parsed entries and writes them straight to the diary.
//
// There is no confirmation step by design: food is dictated one item at a time
// through the day, and confirming each would be worse than the occasional wrong
// entry - especially since food_entry.delete works, so a mistake is one command
// away from gone.
func (h *VoiceWorkoutHandler) applyFood(ctx context.Context, userID, eventID string, interpreted voiceInterpretation, response *voiceWorkoutResponse) {
	now := time.Now()
	kept, rejected := validateParsedEntries(interpreted.Entries, interpreted.Foods, now)
	unmatched := append(append([]string{}, interpreted.Unmatched...), rejected...)

	if len(kept) == 0 {
		response.Unmatched = unmatched
		if len(unmatched) == 0 {
			response.Message = "Похоже на еду, но продукт не распознан."
		}
		return
	}

	if h.food == nil {
		response.Unmatched = unmatched
		response.Message = "Похоже на еду, но запись в FatSecret не настроена."
		return
	}

	written := make([]voiceParsedEntry, 0, len(kept))
	entryIDs := make([]string, 0, len(kept))
	for _, entry := range kept {
		id, err := h.food.CreateFoodEntry(ctx, userID, connectors.FoodEntryDraft{
			FoodID:        entry.FoodID,
			ServingID:     entry.ServingID,
			NumberOfUnits: *entry.Units,
			Meal:          entry.Meal,
			Date:          now,
			Name:          entry.Name,
		})
		if err != nil {
			h.logger.Error().Err(err).Str("food_id", entry.FoodID).Msg("create food entry")
			unmatched = append(unmatched, entry.Name+" (не записалось: "+err.Error()+")")
			continue
		}
		written = append(written, entry)
		entryIDs = append(entryIDs, id)
	}

	// The created ids are kept on the archived phrase: it is the audit trail, and
	// what a later "отмени последнее" would need to undo the write.
	if len(entryIDs) > 0 {
		if err := h.recordFoodEntryIDs(ctx, eventID, entryIDs); err != nil {
			h.logger.Warn().Err(err).Str("event_id", eventID).Msg("record food entry ids")
		}
	}

	response.Food = summarizeFoodEntries(written, interpreted.Foods)
	response.Unmatched = unmatched
	if len(written) > 0 {
		response.Message = "Записал в дневник."
	}
}

func (h *VoiceWorkoutHandler) recordFoodEntryIDs(ctx context.Context, eventID string, ids []string) error {
	if eventID == "" {
		return nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	_, err = h.db.Exec(ctx, `
		UPDATE raw_events SET payload = jsonb_set(payload, '{food_entry_ids}', $2::jsonb)
		WHERE id = $1
	`, eventID, encoded)
	return err
}
