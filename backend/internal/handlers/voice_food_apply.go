package handlers

import (
	"context"
	"encoding/json"
	"strings"
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
func (h *VoiceWorkoutHandler) applyFood(ctx context.Context, userID, eventID, text string, interpreted voiceInterpretation, response *voiceWorkoutResponse) {
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
	if note := fryingOilNote(text, written); note != "" {
		response.Food += "\n" + note
	}
	if warning := cookedWeightWarning(written); warning != "" {
		// Not an "did not understand": the entry is written and usable. It is a
		// number that will read as smaller or larger than what was actually eaten,
		// and the person is the only one who can decide what to do about it.
		response.Food += "\n" + warning
	}
	response.Unmatched = unmatched
	if len(written) > 0 {
		response.Message = "Записал в дневник питания."
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

// fryingOilNote says that the oil is missing from a fried portion.
//
// A weight converted back to the raw product buys the manufacturer's numbers and
// loses whatever went into the pan - a spoonful of oil is about 120 kcal that
// nothing in the entry accounts for. The note does not guess the amount: how
// much oil a person uses is not in the phrase, and inventing it would undo the
// precision the conversion was for.
func fryingOilNote(phrase string, entries []voiceParsedEntry) string {
	if !phraseMentionsFrying(phrase) {
		return ""
	}
	for _, entry := range entries {
		if entry.RawGrams != nil {
			return "Масло от жарки не учтено: записан сырой продукт."
		}
	}
	return ""
}

// cookedWeightWarning names the entries whose weight was given for cooked food
// and stored against a product that is not cooked, and that no yield factor
// could convert - an unfamiliar kind of food, or one the model did not name.
//
// The direction of the error depends on the food - meat sheds water, grains take
// it on - so the warning says that the two weights differ rather than pretending
// to know by how much.
func cookedWeightWarning(entries []voiceParsedEntry) string {
	mismatched := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Cooked && !cookYieldKnown(entry.CookForm) && !voiceNameLooksCooked(entry.Name) {
			mismatched = append(mismatched, entry.Name)
		}
	}
	if len(mismatched) == 0 {
		return ""
	}
	return "Вес готового записан против сырого продукта (" + strings.Join(mismatched, ", ") +
		") - в дневнике он не равен съеденному, поправь в приложении."
}
