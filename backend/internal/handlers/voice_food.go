package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// voiceFoodCandidateLimit bounds the food shortlist offered to the model.
const voiceFoodCandidateLimit = 80

// Meals FatSecret accepts. A diary entry cannot be created without one.
const (
	mealBreakfast = "breakfast"
	mealLunch     = "lunch"
	mealDinner    = "dinner"
	mealOther     = "other"
)

// voiceFoodCandidate is one food the model may choose, taken from what the
// account has already logged.
//
// The pair matters, not just the food: this key cannot read regional foods back
// with food.get - it answers "Invalid ID" for every Russian id - so the serving
// has to come from the diary too. ServingDescription is what lets a spoken "200
// грамм" be turned into a number of servings.
type voiceFoodCandidate struct {
	FoodID             string   `json:"food_id"`
	ServingID          string   `json:"serving_id"`
	Name               string   `json:"name"`
	ServingDescription string   `json:"serving,omitempty"`
	CaloriesPerServing *float64 `json:"kcal_per_serving,omitempty"`
	UsualUnits         *float64 `json:"usual_units,omitempty"`
	// Rank is how habitual the food is, from the most-eaten lists. Lower is more
	// frequent; zero means it never appeared in one.
	Rank int `json:"rank,omitempty"`
	// Meals it has been logged in, a hint for guessing the meal.
	Meals []string `json:"meals,omitempty"`
}

// voiceParsedEntry is one diary entry the model produced.
type voiceParsedEntry struct {
	FoodID    string   `json:"food_id"`
	ServingID string   `json:"serving_id"`
	Name      string   `json:"name"`
	Units     *float64 `json:"units"`
	Meal      string   `json:"meal"`
}

// loadFoodCandidates builds the shortlist from the diary: every food the account
// has logged, with the serving it was logged against and the quantity it usually
// takes.
func (h *VoiceWorkoutHandler) loadFoodCandidates(ctx context.Context, userID string) ([]voiceFoodCandidate, error) {
	rows, err := h.db.Query(ctx, `
		WITH logged AS (
			SELECT i.food_id,
			       i.serving_id,
			       i.food_name,
			       i.serving_description,
			       i.calories,
			       i.number_of_units,
			       d.date,
			       ROW_NUMBER() OVER (
			           PARTITION BY i.food_id, i.serving_id ORDER BY d.date DESC
			       ) AS recency,
			       COUNT(*) OVER (PARTITION BY i.food_id, i.serving_id) AS times
			FROM nutrition_items i
			JOIN nutrition_daily d ON d.id = i.daily_id
			WHERE d.user_id = $1 AND i.food_id IS NOT NULL AND i.serving_id IS NOT NULL
		)
		SELECT logged.food_id, logged.serving_id, logged.food_name,
		       COALESCE(logged.serving_description, ''), logged.calories,
		       logged.number_of_units, COALESCE(c.most_eaten_rank, 0),
		       COALESCE(c.meals, ARRAY[]::text[]), logged.times
		FROM logged
		LEFT JOIN fatsecret_foods c ON c.user_id = $1 AND c.food_id = logged.food_id
		WHERE logged.recency = 1
		ORDER BY logged.times DESC, logged.food_name
		LIMIT $2
	`, userID, voiceFoodCandidateLimit)
	if err != nil {
		return nil, fmt.Errorf("load food candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]voiceFoodCandidate, 0, voiceFoodCandidateLimit)
	for rows.Next() {
		var (
			c     voiceFoodCandidate
			times int
		)
		if err := rows.Scan(&c.FoodID, &c.ServingID, &c.Name, &c.ServingDescription,
			&c.CaloriesPerServing, &c.UsualUnits, &c.Rank, &c.Meals, &times); err != nil {
			return nil, fmt.Errorf("scan food candidate: %w", err)
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

// mealForTime guesses the meal from the clock, which is what keeps the phrase
// short: naming the meal every time would be noise, and the clock is right most
// of the time.
func mealForTime(at time.Time) string {
	switch hour := at.In(aiDisplayLocation).Hour(); {
	case hour < 5:
		// Food at two in the morning is a night snack, not breakfast.
		return mealOther
	case hour < 11:
		return mealBreakfast
	case hour < 16:
		return mealLunch
	case hour < 21:
		return mealDinner
	default:
		return mealOther
	}
}

// spokenMeals maps what a person says onto what the API accepts.
var spokenMeals = map[string]string{
	"завтрак":   mealBreakfast,
	"обед":      mealLunch,
	"ужин":      mealDinner,
	"перекус":   mealOther,
	"полдник":   mealOther,
	"снек":      mealOther,
	"breakfast": mealBreakfast,
	"lunch":     mealLunch,
	"dinner":    mealDinner,
	"snack":     mealOther,
}

// resolveMeal lets the spoken meal win over the clock, and falls back to the
// clock for anything unrecognized.
func resolveMeal(spoken string, at time.Time) string {
	normalized := strings.ToLower(strings.TrimSpace(spoken))
	switch normalized {
	case mealBreakfast, mealLunch, mealDinner, mealOther:
		return normalized
	}
	for word, meal := range spokenMeals {
		if strings.Contains(normalized, word) {
			return meal
		}
	}
	return mealForTime(at)
}

// validateParsedEntries keeps only entries whose food and serving came from the
// shortlist, and whose quantity is plausible. An invented pair would be rejected
// by the API at best, and at worst would log a different food.
func validateParsedEntries(parsed []voiceParsedEntry, candidates []voiceFoodCandidate, at time.Time) ([]voiceParsedEntry, []string) {
	byPair := make(map[string]voiceFoodCandidate, len(candidates))
	for _, candidate := range candidates {
		byPair[candidate.FoodID+"/"+candidate.ServingID] = candidate
	}

	kept := make([]voiceParsedEntry, 0, len(parsed))
	rejected := make([]string, 0)

	for _, entry := range parsed {
		candidate, known := byPair[entry.FoodID+"/"+entry.ServingID]
		if !known {
			name := strings.TrimSpace(entry.Name)
			if name == "" {
				name = entry.FoodID
			}
			rejected = append(rejected, name+" (нет в дневнике, занеси в приложении)")
			continue
		}

		units := entry.Units
		if units == nil || *units <= 0 {
			// The quantity was not spoken: the amount logged last time is a better
			// guess than refusing the entry, and it is what the person eats anyway.
			units = candidate.UsualUnits
		}
		if units == nil || *units <= 0 || *units > voiceMaxFoodUnits {
			rejected = append(rejected, candidate.Name+" (не понял количество)")
			continue
		}

		value := *units
		kept = append(kept, voiceParsedEntry{
			FoodID:    candidate.FoodID,
			ServingID: candidate.ServingID,
			Name:      candidate.Name,
			Units:     &value,
			Meal:      resolveMeal(entry.Meal, at),
		})
	}
	return kept, rejected
}

// voiceMaxFoodUnits catches a misheard quantity: fifty servings of anything is a
// recognition error, not a meal.
const voiceMaxFoodUnits = 50

// summarizeFoodEntries renders what was logged, with the calories, so a wrong
// match is visible immediately.
func summarizeFoodEntries(entries []voiceParsedEntry, candidates []voiceFoodCandidate) string {
	calories := make(map[string]*float64, len(candidates))
	servings := make(map[string]string, len(candidates))
	for _, candidate := range candidates {
		calories[candidate.FoodID+"/"+candidate.ServingID] = candidate.CaloriesPerServing
		servings[candidate.FoodID+"/"+candidate.ServingID] = candidate.ServingDescription
	}

	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		line := entry.Name
		if entry.Units != nil {
			line += fmt.Sprintf(": %s", formatUnits(*entry.Units))
			if serving := servings[entry.FoodID+"/"+entry.ServingID]; serving != "" {
				line += " × " + strings.TrimSpace(serving)
			}
			if kcal := calories[entry.FoodID+"/"+entry.ServingID]; kcal != nil {
				line += fmt.Sprintf(", %.0f ккал", *kcal**entry.Units)
			}
		}
		lines = append(lines, line+" ["+mealLabel(entry.Meal)+"]")
	}
	return strings.Join(lines, "\n")
}

func formatUnits(units float64) string {
	if units == float64(int64(units)) {
		return fmt.Sprintf("%d", int64(units))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", units), "0"), ".")
}

var mealLabels = map[string]string{
	mealBreakfast: "завтрак",
	mealLunch:     "обед",
	mealDinner:    "ужин",
	mealOther:     "перекус",
}

func mealLabel(meal string) string {
	if label, ok := mealLabels[meal]; ok {
		return label
	}
	return meal
}
