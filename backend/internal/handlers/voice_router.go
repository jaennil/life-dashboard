package handlers

import (
	"context"
	"encoding/json"
	"strings"
)

// Domains a dictated phrase can belong to. Only workout is implemented; the rest
// exist so that a phrase about food or a stray thought is recognized as such and
// answered honestly instead of being forced into a workout.
const (
	voiceDomainWorkout = "workout"
	voiceDomainFood    = "food"
	voiceDomainNote    = "note"
	voiceDomainWeight  = "weight"
	voiceDomainUnknown = "unknown"
)

var voiceKnownDomains = map[string]bool{
	voiceDomainWorkout: true,
	voiceDomainFood:    true,
	voiceDomainNote:    true,
	voiceDomainWeight:  true,
}

// voiceDomainReplies explain a domain that is recognized but not yet wired up.
// The phrase is archived either way, so nothing spoken is lost.
var voiceDomainReplies = map[string]string{
	voiceDomainFood:   "Похоже на еду. Запись в FatSecret пока не подключена, фраза сохранена.",
	voiceDomainNote:   "Похоже на заметку. Дневник пока не подключён, фраза сохранена.",
	voiceDomainWeight: "Похоже на вес. Он и так приходит с весов, фраза сохранена.",
}

// resolveVoiceDomain decides where a phrase goes.
//
// An open workout session is a strong prior and the reason this router exists at
// all: "ещё 8" or "12 по 30" says nothing on its own, but means a set while a
// workout is in progress. A per-domain webhook could never use that context.
func resolveVoiceDomain(claimed string, workoutOpen bool) string {
	domain := strings.ToLower(strings.TrimSpace(claimed))
	if voiceKnownDomains[domain] {
		return domain
	}
	if workoutOpen {
		return voiceDomainWorkout
	}
	return voiceDomainUnknown
}

// archivePhrase stores the dictated phrase before anything interprets it and
// before its domain is known. That ordering is what makes a misclassified phrase
// recoverable: the wording survives even when the interpretation is wrong or the
// domain is not implemented yet.
//
// Both forms are kept. The normalized text is what the parser saw; the raw body
// is what the phone actually sent, which is the only way to tell later whether a
// surprise came from the phone, from normalization, or from the model.
func (h *VoiceWorkoutHandler) archivePhrase(ctx context.Context, userID, text string, rawBody []byte) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"text": text,
		"raw":  string(rawBody),
	})
	if err != nil {
		return "", err
	}

	var id string
	err = h.db.QueryRow(ctx, `
		INSERT INTO raw_events (source, event_type, payload, user_id)
		VALUES ('voice', 'phrase', $1::jsonb, $2)
		RETURNING id
	`, payload, userID).Scan(&id)
	return id, err
}

// recordPhraseDomain writes the routing verdict onto the archived phrase. Without
// it the archive says what was said but not where it went, which is exactly what
// has to be auditable once a phrase can end up in someone's food diary.
func (h *VoiceWorkoutHandler) recordPhraseDomain(ctx context.Context, eventID, domain string) error {
	if eventID == "" {
		return nil
	}
	_, err := h.db.Exec(ctx, `
		UPDATE raw_events SET payload = jsonb_set(payload, '{domain}', to_jsonb($2::text))
		WHERE id = $1
	`, eventID, domain)
	return err
}

// composeVoiceDisplay builds what the phone shows.
//
// It opens with what iOS actually recognized, because two very different
// failures look identical otherwise: iOS mishearing a word or a number, and the
// model misreading a correct transcript. Only the first line separates them, and
// only the first is unrecoverable - the audio is never kept, so a misheard number
// has to be caught while the set is still fresh.
//
// It also exists so the Shortcut needs exactly one dictionary key: choosing
// between the parse, a routing message and an error inside Shortcuts would mean
// nested If actions configured by hand on a phone, and that logic belongs
// somewhere it can be tested.
func composeVoiceDisplay(response voiceWorkoutResponse) string {
	var parts []string
	if response.Heard != "" {
		parts = append(parts, "Услышал: "+response.Heard)
	}
	heardOnly := len(parts)

	switch {
	case response.ParseError != "":
		parts = append(parts, "Не разобрал: "+response.ParseError)
	case response.Message != "":
		parts = append(parts, response.Message)
	}

	if response.Workout != "" {
		parts = append(parts, response.Workout)
	}
	if len(response.Unmatched) > 0 {
		parts = append(parts, "Не понял: "+strings.Join(response.Unmatched, "; "))
	}
	if response.Finished {
		finished := "Тренировка закончена"
		if response.Title != "" {
			finished += ": " + response.Title
		}
		parts = append(parts, finished)
	}

	if len(parts) == heardOnly {
		// Recognized but understood as nothing: say so rather than show only the
		// transcript, which reads as a shortcut that half worked.
		parts = append(parts, "Ничего не разобрал.")
	}
	return strings.Join(parts, "\n")
}
