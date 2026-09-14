package handlers

import (
	"slices"
	"strconv"
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

func gramServing(grams float64) *float64 { return &grams }

func TestPreferGramCapableServingsKeepsTheWeighableOne(t *testing.T) {
	// Both rows are the same pasta, logged against two different servings. Only
	// one of them can answer "450 г".
	candidates := []voiceFoodCandidate{
		{FoodID: "1", ServingID: "a", Name: "Barilla Макароны", ServingDescription: "1 serving Barilla Макароны"},
		{FoodID: "1", ServingID: "b", Name: "Barilla Макароны", ServingDescription: "1 :custom:100г, 100 g", ServingGrams: gramServing(100)},
		{FoodID: "2", ServingID: "c", Name: "Петелинка Куриное Филе", ServingGrams: gramServing(100)},
	}

	kept := preferGramCapableServings(candidates)
	if len(kept) != 2 {
		t.Fatalf("kept %d candidates, want one per food: %+v", len(kept), kept)
	}
	if kept[0].ServingID != "b" {
		t.Errorf("kept the serving without a weight: %+v", kept[0])
	}
	// The order of foods is the order they arrived in, which is most-used first.
	if kept[1].FoodID != "2" {
		t.Errorf("food order changed: %+v", kept)
	}
}

func TestPreferGramCapableServingsKeepsTheFirstWhenNeitherHasWeight(t *testing.T) {
	// Nothing to choose between them, so the most-used one stays.
	candidates := []voiceFoodCandidate{
		{FoodID: "1", ServingID: "a", Name: "Кофе"},
		{FoodID: "1", ServingID: "b", Name: "Кофе"},
	}
	kept := preferGramCapableServings(candidates)
	if len(kept) != 1 || kept[0].ServingID != "a" {
		t.Fatalf("kept = %+v", kept)
	}
}

func TestPreferGramCapableServingsDoesNotDowngrade(t *testing.T) {
	// A weighable serving already kept must not be replaced by a later one
	// without a weight.
	candidates := []voiceFoodCandidate{
		{FoodID: "1", ServingID: "a", ServingGrams: gramServing(100)},
		{FoodID: "1", ServingID: "b"},
	}
	kept := preferGramCapableServings(candidates)
	if len(kept) != 1 || kept[0].ServingID != "a" {
		t.Fatalf("kept = %+v", kept)
	}
}

func TestVoiceNameLooksCookedReadsTheProductName(t *testing.T) {
	// Every name here is one from the diary.
	for _, cooked := range []string{
		"Макфа Макароны Отварные", "Макфа Вареные Макароны", "Бахетле Бедро Куриное Жареное",
		"Ашан Куриная Грудка Вареная", "Мистраль Гречка Отварная", "Курица на пару",
	} {
		if !voiceNameLooksCooked(cooked) {
			t.Errorf("%q was not recognised as cooked", cooked)
		}
	}
	for _, raw := range []string{
		"Петелинка Куриное Филе", "Barilla Макароны", "ВкусВилл Стрипсы Куриные", "Ашан Маффин",
	} {
		if voiceNameLooksCooked(raw) {
			t.Errorf("%q was taken for cooked", raw)
		}
	}
}

func TestCookedWeightWarningNamesOnlyTheMismatched(t *testing.T) {
	// The chicken was described fried and stored against a raw product: that is
	// the entry worth a warning. The pasta found its cooked variant, so it is not.
	entries := []voiceParsedEntry{
		{Name: "Петелинка Куриное Филе", Cooked: true},
		{Name: "Макфа Макароны Отварные", Cooked: true},
		{Name: "Ашан Маффин"},
	}

	warning := cookedWeightWarning(entries)
	if !strings.Contains(warning, "Петелинка Куриное Филе") {
		t.Fatalf("the mismatched entry is not named: %q", warning)
	}
	for _, absent := range []string{"Макфа", "Маффин"} {
		if strings.Contains(warning, absent) {
			t.Errorf("%q should not be warned about: %q", absent, warning)
		}
	}
}

func TestCookedWeightWarningStaysSilentWhenItShould(t *testing.T) {
	if got := cookedWeightWarning(nil); got != "" {
		t.Errorf("empty entries produced %q", got)
	}
	// Nothing was described as cooked, so a raw product is exactly right.
	if got := cookedWeightWarning([]voiceParsedEntry{{Name: "Петелинка Куриное Филе"}}); got != "" {
		t.Errorf("a raw entry produced %q", got)
	}
}

// filler stands in for the hundreds of foods the account has logged that the
// phrase says nothing about.
func filler(count int) []voiceFoodCandidate {
	foods := make([]voiceFoodCandidate, 0, count)
	for i := range count {
		foods = append(foods, voiceFoodCandidate{
			FoodID:    strconv.Itoa(1000 + i),
			ServingID: "s",
			Name:      "Творог " + strconv.Itoa(i) + "%",
		})
	}
	return foods
}

func namesOf(candidates []voiceFoodCandidate) []string {
	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		names = append(names, candidate.Name)
	}
	return names
}

func TestRankFoodCandidatesBringsTheSpokenFoodIntoTheShortlist(t *testing.T) {
	// The real shape of the failure: the raw pasta is eaten five times and sits at
	// the front, the boiled one twice and sits past the cut. Frequency alone sent
	// only the raw product to the model, so "варёных макарон" could not match
	// anything else.
	candidates := append([]voiceFoodCandidate{
		{FoodID: "1", ServingID: "a", Name: "Barilla Макароны"},
	}, filler(200)...)
	candidates = append(candidates,
		voiceFoodCandidate{FoodID: "2", ServingID: "b", Name: "Макфа Макароны Отварные"},
		voiceFoodCandidate{FoodID: "3", ServingID: "c", Name: "Ашан Куриная Грудка Вареная"},
		voiceFoodCandidate{FoodID: "4", ServingID: "d", Name: "Петелинка Куриное Филе"})

	shortlist := rankFoodCandidatesForPhrase("450 г варёных макарон и 340 г жареной курицы", candidates, 80)

	if len(shortlist) != 80 {
		t.Fatalf("shortlist of %d, want 80", len(shortlist))
	}
	for _, want := range []string{
		"Макфа Макароны Отварные", "Ашан Куриная Грудка Вареная",
		"Barilla Макароны", "Петелинка Куриное Филе",
	} {
		if !slices.Contains(namesOf(shortlist), want) {
			t.Errorf("%q missing from the shortlist", want)
		}
	}
	// The raw product of a food leads its own cooked entries: it is what a spoken
	// cooked weight gets converted into. The cooked ones stay on the list behind
	// it, for the foods that have nothing raw behind them.
	position := func(name string) int { return slices.Index(namesOf(shortlist), name) }
	if position("Barilla Макароны") > position("Макфа Макароны Отварные") {
		t.Errorf("the cooked pasta outranks the dry one: %v", namesOf(shortlist)[:4])
	}
	if position("Петелинка Куриное Филе") > position("Ашан Куриная Грудка Вареная") {
		t.Errorf("the boiled chicken outranks the raw one: %v", namesOf(shortlist)[:4])
	}
}

func TestRankFoodCandidatesKeepsFrequencyOrderForAnUnrelatedPhrase(t *testing.T) {
	// A workout phrase carries the food catalogue too. Nothing in it matches, and
	// the model must still get the familiar foods, most-eaten first.
	candidates := append([]voiceFoodCandidate{
		{FoodID: "1", ServingID: "a", Name: "Молоко 3,2%"},
	}, filler(200)...)

	shortlist := rankFoodCandidatesForPhrase("жим лежа 5 подходов по 80 кг", candidates, 80)

	if len(shortlist) != 80 || shortlist[0].Name != "Молоко 3,2%" {
		t.Fatalf("shortlist = %v", namesOf(shortlist)[:3])
	}
	if shortlist[1].Name != "Творог 0%" {
		t.Errorf("frequency order broken: %v", namesOf(shortlist)[:3])
	}
}

func TestRankFoodCandidatesLeavesAShortListAlone(t *testing.T) {
	candidates := []voiceFoodCandidate{{FoodID: "1", Name: "Банан"}, {FoodID: "2", Name: "Молоко 3,2%"}}
	if got := rankFoodCandidatesForPhrase("съел банан", candidates, 80); len(got) != 2 {
		t.Fatalf("shortlist = %v", namesOf(got))
	}
}

func TestFoodNameAffinityMatchesAcrossRussianEndings(t *testing.T) {
	spoken := foodMatchTokens("450 г варёных макарон и 340 г жареной курицы")
	if affinity := foodNameAffinity(spoken, "Макфа Макароны Отварные"); affinity == 0 {
		t.Errorf("pasta scored nothing")
	}
	if affinity := foodNameAffinity(spoken, "Петелинка Куриное Филе"); affinity == 0 {
		t.Errorf("chicken scored nothing")
	}
	if affinity := foodNameAffinity(spoken, "Молоко 3,2%"); affinity != 0 {
		t.Errorf("milk scored %d for a phrase that never mentions it", affinity)
	}
}

func TestValidateEntriesKeepsTheCookedFlag(t *testing.T) {
	// Without this the warning about a cooked weight on a raw product can never
	// fire: the flag is set by the model and dropped in validation.
	entries := []voiceParsedEntry{
		{FoodID: "6754762", ServingID: "1", Grams: kg(200), Cooked: true},
	}
	kept, _ := validateParsedEntries(entries, foodTestCandidates, time.Date(2026, 9, 13, 13, 0, 0, 0, time.UTC))
	if len(kept) != 1 {
		t.Fatalf("kept %d entries", len(kept))
	}
	if !kept[0].Cooked {
		t.Error("the cooked flag was dropped")
	}
}

var rawChicken = voiceFoodCandidate{
	FoodID: "8825547", ServingID: "1", Name: "Петелинка Куриное Филе",
	ServingDescription: "3 :custom:3s  x 100г, 300 g", ServingGrams: kg(100), CaloriesPerServing: kg(100),
}

var boiledChicken = voiceFoodCandidate{
	FoodID: "8305446", ServingID: "2", Name: "Ашан Куриная Грудка Вареная",
	ServingGrams: kg(100), CaloriesPerServing: kg(140),
}

func TestValidateEntriesLogsCookedWeightAsRaw(t *testing.T) {
	// 340 г жареной курицы is 476 г of the breast that went into the pan, and the
	// breast is the one whose numbers came off the wrapper.
	entries := []voiceParsedEntry{
		{FoodID: "8825547", ServingID: "1", Grams: kg(340), Cooked: true, CookForm: "meat"},
	}
	kept, rejected := validateParsedEntries(entries, []voiceFoodCandidate{rawChicken}, time.Date(2026, 9, 13, 21, 0, 0, 0, time.UTC))
	if len(kept) != 1 {
		t.Fatalf("kept %d entries, rejected %v", len(kept), rejected)
	}
	if kept[0].RawGrams == nil || *kept[0].RawGrams != 476 {
		t.Fatalf("raw grams = %v, want 476", kept[0].RawGrams)
	}
	if *kept[0].Units != 4.76 {
		t.Errorf("units = %v, want 4.76", *kept[0].Units)
	}
	// The spoken weight survives: it is what the person can check the entry by.
	if kept[0].Grams == nil || *kept[0].Grams != 340 {
		t.Errorf("spoken grams = %v", kept[0].Grams)
	}
}

func TestValidateEntriesLeavesACookedProductAlone(t *testing.T) {
	// Nothing to convert: both the weight and the product are of cooked food.
	entries := []voiceParsedEntry{
		{FoodID: "8305446", ServingID: "2", Grams: kg(340), Cooked: true, CookForm: "meat"},
	}
	kept, _ := validateParsedEntries(entries, []voiceFoodCandidate{boiledChicken}, time.Now())
	if len(kept) != 1 || kept[0].RawGrams != nil {
		t.Fatalf("converted a cooked product: %+v", kept)
	}
	if *kept[0].Units != 3.4 {
		t.Errorf("units = %v, want 3.4", *kept[0].Units)
	}
}

func TestValidateEntriesSkipsTheConversionItCannotMake(t *testing.T) {
	// A ready meal weighed as sold, or a food the model could not name: the weight
	// goes in as spoken and the warning is what covers it.
	for _, form := range []string{"", "vegetable", "плов"} {
		entries := []voiceParsedEntry{
			{FoodID: "8825547", ServingID: "1", Grams: kg(340), Cooked: true, CookForm: form},
		}
		kept, _ := validateParsedEntries(entries, []voiceFoodCandidate{rawChicken}, time.Now())
		if len(kept) != 1 || kept[0].RawGrams != nil {
			t.Fatalf("cook_form %q converted to %+v", form, kept)
		}
	}
}

func TestCookedWeightWarningStaysSilentAfterAConversion(t *testing.T) {
	converted := []voiceParsedEntry{
		{Name: "Петелинка Куриное Филе", Cooked: true, CookForm: "meat", RawGrams: kg(476)},
	}
	if warning := cookedWeightWarning(converted); warning != "" {
		t.Errorf("warned about a converted entry: %s", warning)
	}
	unconverted := []voiceParsedEntry{{Name: "Петелинка Куриное Филе", Cooked: true}}
	if warning := cookedWeightWarning(unconverted); warning == "" {
		t.Error("no warning for a cooked weight that stayed unconverted")
	}
}

func TestSummarizeShowsBothWeightsAfterAConversion(t *testing.T) {
	entries := []voiceParsedEntry{
		{FoodID: "8825547", ServingID: "1", Name: "Петелинка Куриное Филе",
			Units: kg(4.76), Grams: kg(340), RawGrams: kg(476), Meal: mealDinner},
	}
	summary := summarizeFoodEntries(entries, []voiceFoodCandidate{rawChicken})
	for _, want := range []string{"340 г готового", "476 г сырого", "476 ккал"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary missing %q: %s", want, summary)
		}
	}
}

func TestCookedWeightWarningIsQuietForFoodThatKeepsItsWeight(t *testing.T) {
	// Boiled eggs and boiled vegetables weigh what they weighed. Warning about
	// them would be noise on every entry that says "варёные".
	for _, form := range []string{"egg", "vegetable"} {
		entries := []voiceParsedEntry{{Name: "Окей Яйцо Куриное", Cooked: true, CookForm: form}}
		if warning := cookedWeightWarning(entries); warning != "" {
			t.Errorf("cook_form %q warned: %s", form, warning)
		}
	}
}

func TestShortlistPrefersTheProductOverTheDishMadeOfIt(t *testing.T) {
	// "300 г варёной курицы" put sandwiches at the top and the chicken itself at
	// position thirty, so the model said it could not match anything. The word
	// "курицей" sits inside a sandwich exactly as well as inside a fillet; what
	// tells them apart is how much of the name the phrase leaves unsaid.
	candidates := append([]voiceFoodCandidate{
		{FoodID: "1", ServingID: "a", Name: "White Fox Пан с Курицей и Беконом"},
		{FoodID: "2", ServingID: "b", Name: "ВкусВилл Сендвич Ролл с Курицей и Соусом Дзадзики"},
		{FoodID: "3", ServingID: "c", Name: "Петелинка Куриное Филе"},
	}, filler(200)...)

	shortlist := rankFoodCandidatesForPhrase("300 г варёной курицы", candidates, 80)

	if shortlist[0].Name != "Петелинка Куриное Филе" {
		t.Errorf("shortlist starts with %q", shortlist[0].Name)
	}
	// The dishes stay on the list - a person does eat that sandwich - just behind.
	if !slices.Contains(namesOf(shortlist), "White Fox Пан с Курицей и Беконом") {
		t.Error("the dish fell off the shortlist entirely")
	}
}
