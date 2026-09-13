package handlers

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// voiceFoodCandidateLimit bounds the food shortlist offered to the model.
const voiceFoodCandidateLimit = 80

// voiceFoodCatalogueLimit bounds what is read out of the diary before the
// shortlist is cut. The account has hundreds of foods and only eighty fit in the
// prompt, so the cut has to be made against the phrase - reading only the eighty
// most frequent means the words actually spoken never get a chance to be found.
const voiceFoodCatalogueLimit = 600

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
	ServingGrams       *float64 `json:"serving_grams,omitempty"`
	// CaloriesPerServing is derived: the diary stores the total for the entry.
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
	Grams     *float64 `json:"grams"`
	Meal      string   `json:"meal"`
	// Cooked marks a portion the person described as prepared - fried, boiled,
	// baked. It matters because they are stating the weight of the cooked food
	// while the catalogue entry is usually the raw one, and the two are not the
	// same amount: chicken loses water, pasta takes it on.
	Cooked bool `json:"cooked"`
	// CookForm says what kind of food it is for the purpose of that conversion.
	// The model names the food; the factor lives here, in one table, so the same
	// phrase converts the same way every time.
	CookForm string `json:"cook_form,omitempty"`
	// RawGrams is what the conversion produced and what is actually logged. Grams
	// stays as it was spoken, so the notification can show both numbers.
	RawGrams *float64 `json:"-"`
}

// voiceRawYields turns a weight of cooked food into the weight of the raw
// product it was made from.
//
// The packaged raw product is the trustworthy end of this: its numbers come off
// the wrapper. A "варёные макароны" entry in the catalogue is someone's hand
// typing, and the two the account already has disagree with each other by 17%.
// So the weight is converted and the manufacturer's product is what gets logged.
//
// The factors are the usual kitchen yields: meat and poultry lose about 30% of
// their weight, fish about 20%, pasta and grains take on water two to three
// times over.
var voiceRawYields = map[string]float64{
	"meat":      1.4,
	"fish":      1.25,
	"pasta":     0.4,
	"rice":      0.34,
	"buckwheat": 0.45,
	"legume":    0.4,
	// Vegetables hold their weight through boiling, so there is nothing to convert
	// - the entry is here to say so rather than to leave the model guessing.
	"vegetable": 1,
}

// voiceCookedMarkers are the words that make a product a different product. The
// list is matched against both what was said and what the catalogue calls the
// item, so a spoken "жареная" can find "Бедро Куриное Жареное".
var voiceCookedMarkers = []string{
	"варен", "варён", "отварн", "жарен", "жарён", "запечен", "запечён",
	"приготовл", "готов", "тушен", "тушён", "гриль", "на пару", "паровые",
}

// voiceNameLooksCooked reports whether a product name already says it is cooked.
func voiceNameLooksCooked(name string) bool {
	lowered := strings.ToLower(name)
	for _, marker := range voiceCookedMarkers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

var (
	voiceLeadingUnitsPattern = regexp.MustCompile(`^\s*([0-9]+[.,]?[0-9]*)`)
	voiceTimesGramsPattern   = regexp.MustCompile(`(?i)[xх×]\s*([0-9]+[.,]?[0-9]*)s?\s*[gг]`)
	voiceParenGramsPattern   = regexp.MustCompile(`(?i)\(\s*([0-9]+[.,]?[0-9]*)s?\s*[gг]\s*\)`)
	voiceGramsPattern        = regexp.MustCompile(`(?i)([0-9]+[.,]?[0-9]*)s?\s*[gг]`)
)

// inferVoiceServing recovers provider units and the weight of one unit from
// regional descriptions such as "0.7 :custom:70 g" and
// "1.2 :custom:1.2s x 100г, 120 g". The separate number_of_units field is
// absent or rounded in those FatSecret responses.
func inferVoiceServing(description string) (float64, float64) {
	parse := func(match []string) float64 {
		if len(match) < 2 {
			return 0
		}
		value, _ := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", "."), 64)
		return value
	}

	timesMatch := voiceTimesGramsPattern.FindStringSubmatch(description)
	custom := strings.Contains(description, ":custom:")
	units := 0.0
	if custom || len(timesMatch) > 0 {
		units = parse(voiceLeadingUnitsPattern.FindStringSubmatch(description))
	}

	if grams := parse(timesMatch); grams > 0 {
		return units, grams
	}
	if grams := parse(voiceParenGramsPattern.FindStringSubmatch(description)); grams > 0 {
		return units, grams
	}

	gramsMatches := voiceGramsPattern.FindAllStringSubmatch(description, -1)
	if custom && units > 0 && len(gramsMatches) > 0 {
		totalGrams := parse(gramsMatches[len(gramsMatches)-1])
		if perUnit := totalGrams / units; perUnit > 0 {
			return units, perUnit
		}
	}
	return units, 0
}

// loadFoodCandidates builds the shortlist from the diary: the foods the account
// has logged, with the serving they were logged against and the quantity they
// usually take.
//
// The phrase decides which of them travel to the model. Frequency alone put
// "Barilla Макароны" in the prompt and left "Макфа Макароны Отварные" out of it,
// so a dictated "варёные макароны" could only ever match the raw product - the
// cooked one was never on the list to choose from.
func (h *VoiceWorkoutHandler) loadFoodCandidates(ctx context.Context, userID, phrase string) ([]voiceFoodCandidate, error) {
	rows, err := h.db.Query(ctx, `
		WITH logged AS (
			SELECT i.food_id,
			       i.serving_id,
			       i.food_name,
			       i.serving_description,
			       i.calories,
			       i.number_of_units,
			       d.date,
			       -- The same serving is written down differently from day to day:
			       -- "1 :custom:100г, 100 g" one time and a bare "1 serving" the
			       -- next. Only the first spells out the weight, so it wins over the
			       -- newer one - otherwise a food logged in grams all week comes back
			       -- as "не смог перевести граммы в порцию".
			       ROW_NUMBER() OVER (
			           PARTITION BY i.food_id, i.serving_id
			           ORDER BY (i.serving_description ~ '[0-9]\s*g([^a-z]|$)') DESC NULLS LAST,
			                    d.date DESC
			       ) AS recency,
			       COUNT(*) OVER (PARTITION BY i.food_id, i.serving_id) AS times
			FROM nutrition_items i
			JOIN nutrition_daily d ON d.id = i.daily_id
			WHERE d.user_id = $1 AND i.food_id IS NOT NULL AND i.serving_id IS NOT NULL
		)
		SELECT logged.food_id, logged.serving_id, logged.food_name,
		       COALESCE(logged.serving_description, ''),
		       -- Keep the entry total. Some regional responses omit or round
		       -- number_of_units, so Go first recovers its lossless value from the
		       -- provider description and only then derives calories per serving.
		       logged.calories,
		       logged.number_of_units, COALESCE(c.most_eaten_rank, 0),
		       COALESCE(c.meals, ARRAY[]::text[]), logged.times
		FROM logged
		LEFT JOIN fatsecret_foods c ON c.user_id = $1 AND c.food_id = logged.food_id
		WHERE logged.recency = 1
		ORDER BY logged.times DESC, logged.food_name
		LIMIT $2
	`, userID, voiceFoodCatalogueLimit)
	if err != nil {
		return nil, fmt.Errorf("load food candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]voiceFoodCandidate, 0, voiceFoodCandidateLimit)
	for rows.Next() {
		var (
			c             voiceFoodCandidate
			totalCalories *float64
			times         int
		)
		if err := rows.Scan(&c.FoodID, &c.ServingID, &c.Name, &c.ServingDescription,
			&totalCalories, &c.UsualUnits, &c.Rank, &c.Meals, &times); err != nil {
			return nil, fmt.Errorf("scan food candidate: %w", err)
		}
		units, servingGrams := inferVoiceServing(c.ServingDescription)
		if units > 0 {
			c.UsualUnits = &units
		}
		if servingGrams > 0 {
			c.ServingGrams = &servingGrams
		}
		if totalCalories != nil {
			perServing := *totalCalories
			if c.UsualUnits != nil && *c.UsualUnits > 0 {
				perServing /= *c.UsualUnits
			}
			c.CaloriesPerServing = &perServing
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rankFoodCandidatesForPhrase(phrase, preferGramCapableServings(candidates), voiceFoodCandidateLimit), nil
}

// rankFoodCandidatesForPhrase puts the foods the phrase is about at the front of
// the shortlist and cuts it to what fits in the prompt.
//
// Everything that scores nothing keeps its place behind them, most-eaten first,
// so a phrase whose words match nothing - or one that is not about food at all -
// still leaves the model the familiar catalogue it had before.
func rankFoodCandidatesForPhrase(phrase string, candidates []voiceFoodCandidate, limit int) []voiceFoodCandidate {
	if limit <= 0 || len(candidates) <= limit {
		return candidates
	}

	spoken := foodMatchTokens(phrase)
	cookedSaid := phraseMentionsCooked(phrase)

	type ranked struct {
		candidate voiceFoodCandidate
		score     int
		position  int
	}
	scored := make([]ranked, 0, len(candidates))
	for i, candidate := range candidates {
		score := foodNameAffinity(spoken, candidate.Name)
		if score > 0 && cookedSaid && voiceNameLooksCooked(candidate.Name) {
			// "Отварные" shares no prefix with "варёных" and "жареная" shares none
			// with "гриль", so the words that name the cooking cannot be matched
			// like the rest. They are the whole reason the right product is a
			// different product, so they are scored separately.
			score += 2
		}
		scored = append(scored, ranked{candidate: candidate, score: score, position: i})
	}

	sort.SliceStable(scored, func(a, b int) bool {
		if scored[a].score != scored[b].score {
			return scored[a].score > scored[b].score
		}
		return scored[a].position < scored[b].position
	})

	kept := make([]voiceFoodCandidate, 0, limit)
	for _, item := range scored[:limit] {
		kept = append(kept, item.candidate)
	}
	return kept
}

// foodStopWords are the words a food phrase is made of that say nothing about
// which product it is.
var foodStopWords = map[string]bool{
	"грамм": true, "грамма": true, "граммов": true, "грамов": true,
	"штук": true, "штуки": true, "штука": true, "порция": true, "порции": true,
	"съел": true, "съела": true, "поел": true, "ещё": true, "еще": true,
	"было": true, "утром": true, "днем": true, "вечером": true, "сегодня": true,
}

// foodMatchTokens reduces a phrase to the words worth looking for in a product
// name. Short words carry no product in them and would match everything.
func foodMatchTokens(phrase string) []string {
	fields := strings.FieldsFunc(normalizeFoodText(phrase), func(r rune) bool {
		return !unicode.IsLetter(r)
	})
	tokens := make([]string, 0, len(fields))
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		if len([]rune(field)) < 4 || foodStopWords[field] || seen[field] {
			continue
		}
		seen[field] = true
		tokens = append(tokens, field)
	}
	return tokens
}

// foodNameAffinity scores how much of the phrase a product name accounts for.
//
// The match is by prefix because Russian inflects the ending and the catalogue
// does not agree with speech about it: "курицы" has to find "Куриное", "макарон"
// has to find "Макароны".
func foodNameAffinity(spoken []string, name string) int {
	if len(spoken) == 0 {
		return 0
	}
	nameTokens := foodMatchTokens(name)
	total := 0
	for _, token := range spoken {
		best := 0
		for _, nameToken := range nameTokens {
			if affinity := tokenAffinity(token, nameToken); affinity > best {
				best = affinity
			}
		}
		total += best
	}
	return total
}

func tokenAffinity(spoken, name string) int {
	if spoken == name {
		return 3
	}
	switch shared := commonPrefixRunes(spoken, name); {
	case shared >= 5:
		return 2
	case shared >= 4:
		return 1
	default:
		return 0
	}
}

func commonPrefixRunes(a, b string) int {
	first, second := []rune(a), []rune(b)
	shared := 0
	for shared < len(first) && shared < len(second) && first[shared] == second[shared] {
		shared++
	}
	return shared
}

// phraseMentionsCooked reports whether the person said the food was prepared.
func phraseMentionsCooked(phrase string) bool {
	return voiceNameLooksCooked(phrase)
}

// normalizeFoodText folds case and the letter that Russian writes both ways, so
// that a dictated "варёных" and a catalogue "Вареные" are the same word.
func normalizeFoodText(text string) string {
	return strings.ReplaceAll(strings.ToLower(text), "ё", "е")
}

// preferGramCapableServings keeps one serving per food, choosing the one that
// carries a weight.
//
// The same food is logged against several servings over time - "1 serving" one
// day, "100 г" another - and each becomes its own candidate. A serving with a
// weight can express both a spoken weight and a spoken portion; a serving
// without one can only do portions. Offering both invites the model to pick the
// weaker one, which is how "450 г варёных макарон" came back as "не смог
// перевести граммы в порцию" for a food logged in grams all week.
func preferGramCapableServings(candidates []voiceFoodCandidate) []voiceFoodCandidate {
	best := make(map[string]int, len(candidates))
	kept := make([]voiceFoodCandidate, 0, len(candidates))

	for _, candidate := range candidates {
		position, seen := best[candidate.FoodID]
		if !seen {
			best[candidate.FoodID] = len(kept)
			kept = append(kept, candidate)
			continue
		}
		// The list arrives most-used first, so the incumbent only loses when it
		// cannot do something the newcomer can.
		if hasServingGrams(kept[position]) || !hasServingGrams(candidate) {
			continue
		}
		kept[position] = candidate
	}
	return kept
}

func hasServingGrams(candidate voiceFoodCandidate) bool {
	return candidate.ServingGrams != nil && *candidate.ServingGrams > 0
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
		var rawGrams *float64
		if entry.Grams != nil {
			if *entry.Grams <= 0 || *entry.Grams > voiceMaxFoodGrams || candidate.ServingGrams == nil || *candidate.ServingGrams <= 0 {
				rejected = append(rejected, candidate.Name+" (не смог перевести граммы в порцию)")
				continue
			}
			grams := *entry.Grams
			if raw, ok := rawWeightOf(grams, entry, candidate); ok {
				grams = raw
				rawGrams = &raw
			}
			converted := grams / *candidate.ServingGrams
			units = &converted
		}
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
			Grams:     entry.Grams,
			RawGrams:  rawGrams,
			CookForm:  entry.CookForm,
			Meal:      resolveMeal(entry.Meal, at),
			// Carried over deliberately: it is what tells the person their weight
			// of cooked food went against a raw product, and dropping it here made
			// that warning silently unreachable.
			Cooked: entry.Cooked,
		})
	}
	return kept, rejected
}

// rawWeightOf converts a spoken weight of cooked food into the weight of the
// raw product the entry is written against.
//
// It only fires when all three things line up: the person said the food was
// cooked, named a kind of food with a known yield, and the product chosen is the
// raw one. A ready meal - shawarma, lasagne, a sandwich - is weighed as sold and
// has nothing raw behind it, and a catalogue entry that already says "Отварные"
// is cooked on both sides of the sum.
func rawWeightOf(grams float64, entry voiceParsedEntry, candidate voiceFoodCandidate) (float64, bool) {
	if !entry.Cooked || voiceNameLooksCooked(candidate.Name) {
		return 0, false
	}
	factor, known := voiceRawYields[strings.ToLower(strings.TrimSpace(entry.CookForm))]
	if !known || factor == 1 {
		return 0, false
	}
	// Rounded to whole grams: the factor is a kitchen average, and a converted
	// weight printed to four decimals would claim a precision it does not have.
	converted := math.Round(grams * factor)
	if converted <= 0 || converted > voiceMaxFoodGrams {
		return 0, false
	}
	return converted, true
}

// voiceMaxFoodUnits catches a misheard quantity: fifty servings of anything is a
// recognition error, not a meal.
const voiceMaxFoodUnits = 50

const voiceMaxFoodGrams = 10000

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
			if entry.Grams != nil {
				// Both weights are shown when they differ: the one that was spoken is
				// the only thing the person can check the entry against, and the one
				// that was logged is the only thing that explains the calories.
				if entry.RawGrams != nil {
					line += fmt.Sprintf(": %s г готового = %s г сырого",
						formatUnits(*entry.Grams), formatUnits(*entry.RawGrams))
				} else {
					line += fmt.Sprintf(": %s г", formatUnits(*entry.Grams))
				}
			} else {
				line += fmt.Sprintf(": %s", formatUnits(*entry.Units))
				if serving := servings[entry.FoodID+"/"+entry.ServingID]; serving != "" {
					line += " × " + strings.TrimSpace(serving)
				}
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
