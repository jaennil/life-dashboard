package handlers

import (
	"encoding/json"
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

// fillMissingSetMetrics uses the latest workout only for values the user did
// not say. Explicit values always win. Set positions are matched in order; if
// the new phrase names more sets than the previous workout had, its last set is
// the least surprising fallback for the remainder.
func fillMissingSetMetrics(parsed []voiceParsedExercise, candidates []voiceExerciseCandidate) []voiceParsedExercise {
	byID := make(map[string]voiceExerciseCandidate, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.TemplateID] = candidate
	}

	for exerciseIndex := range parsed {
		exercise := &parsed[exerciseIndex]
		candidate, known := byID[exercise.TemplateID]
		if !known || len(candidate.LastSets) == 0 {
			continue
		}
		if len(exercise.Sets) == 0 {
			exercise.Sets = []voiceParsedSet{{Type: "normal"}}
		}
		for setIndex := range exercise.Sets {
			previousIndex := setIndex
			if previousIndex >= len(candidate.LastSets) {
				previousIndex = len(candidate.LastSets) - 1
			}
			set := &exercise.Sets[setIndex]
			previous := candidate.LastSets[previousIndex]
			if voiceRepsTypes[candidate.Type] && set.Reps == nil {
				set.Reps = cloneInt(previous.Reps)
			}
			if voiceWeightTypes[candidate.Type] && set.WeightKg == nil {
				set.WeightKg = cloneFloat(previous.WeightKg)
			}
		}
	}
	return parsed
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

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
			// "Сгибание на бицепс с гантели 16 кг" names a weight and no reps, and
			// the exercise was rejected as "не понял подходы" - which reads as a
			// parsing failure when the phrase simply never said how many.
			reason := "не понял подходы"
			if !mentionsAnyCount(exercise.Sets) {
				reason = "не назвал повторения"
			}
			rejected = append(rejected, describeRejectedExercise(exercise, reason))
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

// UnmarshalJSON accepts the shapes a model actually produces for a set, not only
// the one it was asked for. A weight can arrive as 13.5, as "13.5", or - after a
// phrase dictated in Russian, where the decimal separator is a comma - as "13,5".
// Strict decoding would fail the whole phrase over one field, losing sets that
// were understood perfectly, so each field is coerced on its own and an
// unreadable one simply stays absent.
func (s *voiceParsedSet) UnmarshalJSON(data []byte) error {
	var lenient struct {
		Type            string          `json:"type"`
		Reps            json.RawMessage `json:"reps"`
		WeightKg        json.RawMessage `json:"weight_kg"`
		DurationSeconds json.RawMessage `json:"duration_seconds"`
	}
	if err := json.Unmarshal(data, &lenient); err != nil {
		return err
	}

	s.Type = lenient.Type
	s.Reps = lenientInt(lenient.Reps)
	s.WeightKg = lenientFloat(lenient.WeightKg)
	s.DurationSeconds = lenientInt(lenient.DurationSeconds)
	return nil
}

// lenientFloat reads a number that may be quoted and may use a comma as the
// decimal separator. Anything unreadable returns nil rather than zero: a weight
// of zero is a claim, absence is not.
func lenientFloat(raw json.RawMessage) *float64 {
	text := numericText(raw)
	if text == "" {
		return nil
	}
	value, err := strconv.ParseFloat(strings.Replace(text, ",", ".", 1), 64)
	if err != nil {
		return nil
	}
	return &value
}

func lenientInt(raw json.RawMessage) *int {
	text := numericText(raw)
	if text == "" {
		return nil
	}
	// Reps arriving as "10.0" is still ten reps, so the value is parsed as a
	// float and then rounded rather than rejected.
	value, err := strconv.ParseFloat(strings.Replace(text, ",", ".", 1), 64)
	if err != nil {
		return nil
	}
	rounded := int(value + 0.5)
	if value < 0 {
		rounded = int(value - 0.5)
	}
	return &rounded
}

// numericText unwraps a raw JSON value down to the digits it carries, whether it
// arrived quoted or bare.
func numericText(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	if strings.HasPrefix(trimmed, `"`) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return ""
		}
		return strings.TrimSpace(text)
	}
	return trimmed
}

// mentionsAnyCount reports whether the model heard any countable quantity at all.
// It separates "the phrase did not say how many" from "the numbers made no sense".
func mentionsAnyCount(sets []voiceParsedSet) bool {
	for _, set := range sets {
		if set.Reps != nil || set.DurationSeconds != nil {
			return true
		}
	}
	return false
}
