package handlers

import (
	"fmt"
	"strconv"
	"strings"
)

// Hevy accepts only these set classifications.
var voiceSetTypes = map[string]bool{
	"normal":  true,
	"warmup":  true,
	"failure": true,
	"dropset": true,
}

// Which set fields a template type actually supports. Sending a weight for a
// reps_only exercise does not fail loudly - it quietly puts a number in the
// history that never happened.
var (
	voiceWeightTypes = map[string]bool{
		"weight_reps":           true,
		"bodyweight_weighted":   true,
		"short_distance_weight": true,
	}
	voiceRepsTypes = map[string]bool{
		"weight_reps":           true,
		"bodyweight_weighted":   true,
		"short_distance_weight": true,
		"reps_only":             true,
		"bodyweight_assisted":   true,
	}
	voiceDurationTypes = map[string]bool{
		"duration":        true,
		"floors_duration": true,
		"steps_duration":  true,
	}
)

// validateParsedExercises keeps only what can actually be written to Hevy and
// reports everything it dropped.
//
// The model is not trusted on two points in particular: the template id, which
// must be one it was offered rather than one it invented, and the set fields,
// which have to match the exercise's measurement type.
func validateParsedExercises(parsed []voiceParsedExercise, candidates []voiceExerciseCandidate) ([]voiceParsedExercise, []string) {
	byID := make(map[string]voiceExerciseCandidate, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.TemplateID] = candidate
	}

	kept := make([]voiceParsedExercise, 0, len(parsed))
	rejected := make([]string, 0)

	for _, exercise := range parsed {
		candidate, known := byID[exercise.TemplateID]
		if !known {
			rejected = append(rejected, describeRejectedExercise(exercise, "неизвестное упражнение"))
			continue
		}

		sets := make([]voiceParsedSet, 0, len(exercise.Sets))
		dropped := 0
		for _, set := range exercise.Sets {
			cleaned, ok := cleanVoiceSet(set, candidate.Type)
			if !ok {
				dropped++
				continue
			}
			sets = append(sets, cleaned)
		}
		if len(sets) == 0 {
			rejected = append(rejected, describeRejectedExercise(exercise, "не понял подходы"))
			continue
		}
		// Losing some sets while keeping others has to be said out loud. Silently
		// writing three of four sets looks like a correct answer, and the set count
		// is the one thing the person dictating cannot check later.
		if dropped > 0 {
			rejected = append(rejected, fmt.Sprintf("%s (%d подход(ов) не понял)", candidate.Title, dropped))
		}

		kept = append(kept, voiceParsedExercise{
			// Canonical Hevy spelling, so the draft and the echo back to the phone
			// name the exercise the same way the app does.
			TemplateID: candidate.TemplateID,
			Title:      candidate.Title,
			Sets:       sets,
		})
	}
	return kept, rejected
}

// cleanVoiceSet strips fields the exercise type does not support and rejects
// values outside plausible bounds. The bounds are about mishearing, not about
// training: "13.5" heard as "135" has to be caught here.
func cleanVoiceSet(set voiceParsedSet, templateType string) (voiceParsedSet, bool) {
	cleaned := voiceParsedSet{Type: strings.ToLower(strings.TrimSpace(set.Type))}
	if !voiceSetTypes[cleaned.Type] {
		cleaned.Type = "normal"
	}

	if voiceRepsTypes[templateType] && set.Reps != nil {
		if *set.Reps <= 0 || *set.Reps > voiceMaxReps {
			return cleaned, false
		}
		reps := *set.Reps
		cleaned.Reps = &reps
	}

	if voiceWeightTypes[templateType] && set.WeightKg != nil {
		if *set.WeightKg < 0 || *set.WeightKg > voiceMaxWeightKg {
			return cleaned, false
		}
		weight := *set.WeightKg
		cleaned.WeightKg = &weight
	}

	if voiceDurationTypes[templateType] && set.DurationSeconds != nil {
		if *set.DurationSeconds <= 0 {
			return cleaned, false
		}
		duration := *set.DurationSeconds
		cleaned.DurationSeconds = &duration
	}

	// A set has to carry at least one measurement, otherwise it says nothing.
	if cleaned.Reps == nil && cleaned.DurationSeconds == nil {
		return cleaned, false
	}
	return cleaned, true
}

func describeRejectedExercise(exercise voiceParsedExercise, reason string) string {
	title := strings.TrimSpace(exercise.Title)
	if title == "" {
		title = exercise.TemplateID
	}
	return fmt.Sprintf("%s (%s)", title, reason)
}

// mergeVoiceDraft folds a phrase into the workout so far. Sets for an exercise
// already in the draft are appended to it rather than starting a second entry,
// which is what dictating a few sets at a time produces.
func mergeVoiceDraft(draft, addition []voiceParsedExercise) []voiceParsedExercise {
	merged := make([]voiceParsedExercise, len(draft))
	copy(merged, draft)

	index := make(map[string]int, len(merged))
	for i, exercise := range merged {
		index[exercise.TemplateID] = i
	}

	for _, exercise := range addition {
		if at, exists := index[exercise.TemplateID]; exists {
			merged[at].Sets = append(merged[at].Sets, exercise.Sets...)
			continue
		}
		index[exercise.TemplateID] = len(merged)
		merged = append(merged, exercise)
	}
	return merged
}

// summarizeVoiceDraft renders the draft for the reply the phone shows. It is the
// only feedback available while dictating, so it has to be readable at a glance
// and show the numbers that were understood, not just the exercise names.
func summarizeVoiceDraft(exercises []voiceParsedExercise) string {
	if len(exercises) == 0 {
		return ""
	}

	lines := make([]string, 0, len(exercises))
	for _, exercise := range exercises {
		sets := make([]string, 0, len(exercise.Sets))
		for _, set := range exercise.Sets {
			sets = append(sets, describeVoiceSet(set))
		}
		lines = append(lines, fmt.Sprintf("%s: %s", exercise.Title, strings.Join(sets, ", ")))
	}
	return strings.Join(lines, "\n")
}

func describeVoiceSet(set voiceParsedSet) string {
	var parts []string
	if set.Reps != nil {
		parts = append(parts, strconv.Itoa(*set.Reps))
	}
	if set.DurationSeconds != nil {
		parts = append(parts, strconv.Itoa(*set.DurationSeconds)+"с")
	}
	if set.WeightKg != nil {
		parts = append(parts, strconv.FormatFloat(*set.WeightKg, 'f', -1, 64)+"кг")
	}
	described := strings.Join(parts, "×")
	if set.Type != "" && set.Type != "normal" {
		described += " (" + set.Type + ")"
	}
	return described
}
