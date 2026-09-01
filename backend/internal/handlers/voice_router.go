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

// archivePhrase stores the dictated text before anything interprets it, and
// before it is known which domain it belongs to. That ordering is what makes a
// misclassified phrase recoverable: the wording survives even when the
// interpretation is wrong or the domain is not implemented yet.
func (h *VoiceWorkoutHandler) archivePhrase(ctx context.Context, userID, text string) error {
	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	_, err = h.db.Exec(ctx, `
		INSERT INTO raw_events (source, event_type, payload, user_id)
		VALUES ('voice', 'phrase', $1::jsonb, $2)
	`, payload, userID)
	return err
}
