package handlers

import (
	"strings"
	"testing"
	"time"
)

func kg(v float64) *float64 { return &v }

var foodTestCandidates = []voiceFoodCandidate{
	{FoodID: "6754762", ServingID: "1", Name: "Молоко 3,2%", ServingDescription: "100 г",
		ServingGrams: kg(100), CaloriesPerServing: kg(60), UsualUnits: kg(2), Rank: 1, Meals: []string{"breakfast"}},
	{FoodID: "52853433", ServingID: "2", Name: "Банан", ServingDescription: "1 шт",
		CaloriesPerServing: kg(95), UsualUnits: kg(1), Rank: 2},
	{FoodID: "86855602", ServingID: "3", Name: "Snickers Сникерс Супер", ServingDescription: "1 батончик",
		CaloriesPerServing: kg(395), UsualUnits: kg(2)},
}

func TestInferVoiceServingFromRegionalDescriptions(t *testing.T) {
	for _, test := range []struct {
		description string
		units       float64
		grams       float64
	}{
		{"0.7 :custom:70 g Мастер Пироговъ Венский Конвертик", 0.7, 100},
		{"1.2 :custom:1.2s x 100г, 120 g Ашан Маффин", 1.2, 100},
		{"2 :custom:2s x 1 батончик (80 g) Snickers", 2, 80},
	} {
		units, grams := inferVoiceServing(test.description)
		if units != test.units || grams != test.grams {
			t.Errorf("inferVoiceServing(%q) = %v, %v; want %v, %v", test.description, units, grams, test.units, test.grams)
		}
	}
}

func TestMealFollowsTheClock(t *testing.T) {
	cases := map[int]string{
		2: mealOther, 4: mealOther, // the small hours are a snack, not breakfast
		7: mealBreakfast, 10: mealBreakfast,
		13: mealLunch, 15: mealLunch,
		17: mealDinner, 20: mealDinner,
		22: mealOther,
	}
	for hour, want := range cases {
		at := time.Date(2026, 9, 1, hour, 0, 0, 0, aiDisplayLocation)
		if got := mealForTime(at); got != want {
			t.Errorf("%02d:00 -> %s, want %s", hour, got, want)
		}
	}
}

func TestSpokenMealOverridesTheClock(t *testing.T) {
	// Dictated at night, but the person says it was breakfast.
	night := time.Date(2026, 9, 1, 23, 0, 0, 0, aiDisplayLocation)

	for spoken, want := range map[string]string{
		"завтрак":   mealBreakfast,
		"на обед":   mealLunch,
		"ужин":      mealDinner,
		"перекус":   mealOther,
		"breakfast": mealBreakfast,
		// Unrecognized wording must not silently pick a wrong meal.
		"":   mealOther,
		"хз": mealOther,
	} {
		if got := resolveMeal(spoken, night); got != want {
			t.Errorf("resolveMeal(%q) = %s, want %s", spoken, got, want)
		}
	}
}

func TestValidateEntriesRejectsAPairNotInTheDiary(t *testing.T) {
	// The API cannot read regional foods back, so a pair the model invented would
	// either be refused or, worse, log a different food.
	parsed := []voiceParsedEntry{
		{FoodID: "999", ServingID: "999", Name: "Лимонад Лапочка", Units: kg(1)},
	}

	kept, rejected := validateParsedEntries(parsed, foodTestCandidates, time.Now())

	if len(kept) != 0 {
		t.Fatalf("kept an invented pair: %+v", kept)
	}
	if len(rejected) != 1 || !strings.Contains(rejected[0], "занеси в приложении") {
		t.Fatalf("rejected = %v", rejected)
	}
}

func TestValidateEntriesFallsBackToTheUsualQuantity(t *testing.T) {
	// "съел сникерс" with no amount: the quantity logged last time beats refusing
	// the entry.
	parsed := []voiceParsedEntry{{FoodID: "86855602", ServingID: "3", Units: nil}}

	kept, rejected := validateParsedEntries(parsed, foodTestCandidates, time.Now())

	if len(rejected) != 0 {
		t.Fatalf("unexpected rejections: %v", rejected)
	}
	if len(kept) != 1 || kept[0].Units == nil || *kept[0].Units != 2 {
		t.Fatalf("kept = %+v", kept)
	}
	// The canonical name replaces whatever the model wrote.
	if kept[0].Name != "Snickers Сникерс Супер" {
		t.Fatalf("name = %q", kept[0].Name)
	}
}

func TestValidateEntriesConvertsExplicitGrams(t *testing.T) {
	parsed := []voiceParsedEntry{{FoodID: "6754762", ServingID: "1", Grams: kg(70)}}

	kept, rejected := validateParsedEntries(parsed, foodTestCandidates, time.Now())

	if len(rejected) != 0 {
		t.Fatalf("unexpected rejections: %v", rejected)
	}
	if len(kept) != 1 || kept[0].Units == nil || *kept[0].Units != 0.7 {
		t.Fatalf("kept = %+v, want 0.7 units", kept)
	}
	if kept[0].Grams == nil || *kept[0].Grams != 70 {
		t.Fatalf("grams were not preserved: %+v", kept[0])
	}
}

func TestValidateEntriesRejectsGramsForUnitServing(t *testing.T) {
	parsed := []voiceParsedEntry{{FoodID: "52853433", ServingID: "2", Grams: kg(70)}}

	kept, rejected := validateParsedEntries(parsed, foodTestCandidates, time.Now())

	if len(kept) != 0 || len(rejected) != 1 || !strings.Contains(rejected[0], "перевести граммы") {
		t.Fatalf("kept = %+v, rejected = %v", kept, rejected)
	}
}

func TestValidateEntriesRejectsAMisheardQuantity(t *testing.T) {
	parsed := []voiceParsedEntry{{FoodID: "52853433", ServingID: "2", Units: kg(300)}}

	kept, rejected := validateParsedEntries(parsed, foodTestCandidates, time.Now())
	if len(kept) != 0 {
		t.Fatalf("kept 300 bananas: %+v", kept)
	}
	if len(rejected) != 1 || !strings.Contains(rejected[0], "количество") {
		t.Fatalf("rejected = %v", rejected)
	}
}

func TestSummarizeFoodEntriesShowsCaloriesAndMeal(t *testing.T) {
	entries := []voiceParsedEntry{
		{FoodID: "6754762", ServingID: "1", Name: "Молоко 3,2%", Units: kg(2), Meal: mealBreakfast},
		{FoodID: "52853433", ServingID: "2", Name: "Банан", Units: kg(1.5), Meal: mealOther},
	}

	summary := summarizeFoodEntries(entries, foodTestCandidates)

	for _, want := range []string{
		"Молоко 3,2%: 2 × 100 г, 120 ккал [завтрак]",
		"Банан: 1.5 × 1 шт, 142 ккал [перекус]",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestSummarizeFoodEntriesShowsExplicitGrams(t *testing.T) {
	entries := []voiceParsedEntry{
		{FoodID: "6754762", ServingID: "1", Name: "Молоко 3,2%", Units: kg(0.7), Grams: kg(70), Meal: mealBreakfast},
	}

	summary := summarizeFoodEntries(entries, foodTestCandidates)
	if !strings.Contains(summary, "70 г, 42 ккал") {
		t.Fatalf("summary = %q", summary)
	}
}
