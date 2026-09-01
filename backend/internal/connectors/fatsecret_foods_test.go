package connectors

import (
	"encoding/json"
	"testing"
)

func TestDecodeFoodListArray(t *testing.T) {
	// Captured from the live account.
	payload := `{"foods":{"food":[
	  {"food_id":"6754762","food_name":"Молоко 3,2%","brand_name":"Простоквашино","food_type":"Brand","food_url":"https://x/1"},
	  {"food_id":"52853433","food_name":"Банан","brand_name":"ВкусВилл","food_type":"Brand","food_url":"https://x/2"}
	]}}`

	var decoded fsFoodsResponse
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Foods.Foods) != 2 {
		t.Fatalf("got %d foods", len(decoded.Foods.Foods))
	}
	first := decoded.Foods.Foods[0]
	if first.FoodID != "6754762" || first.FoodName != "Молоко 3,2%" || first.BrandName != "Простоквашино" {
		t.Fatalf("first food = %+v", first)
	}
}

func TestDecodeFoodListSingleObject(t *testing.T) {
	// FatSecret collapses a one-element list into a bare object. Decoding this as
	// a slice fails, which would silently lose the only food in the answer.
	payload := `{"foods":{"food":{"food_id":"6754762","food_name":"Молоко 3,2%","brand_name":"Простоквашино"}}}`

	var decoded fsFoodsResponse
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Foods.Foods) != 1 {
		t.Fatalf("got %d foods, want 1", len(decoded.Foods.Foods))
	}
	if decoded.Foods.Foods[0].FoodID != "6754762" {
		t.Fatalf("food = %+v", decoded.Foods.Foods[0])
	}
}

func TestDecodeFoodListEmpty(t *testing.T) {
	for _, payload := range []string{`{"foods":{}}`, `{"foods":{"food":null}}`, `{}`} {
		var decoded fsFoodsResponse
		if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
			t.Fatalf("unmarshal %s: %v", payload, err)
		}
		if len(decoded.Foods.Foods) != 0 {
			t.Fatalf("%s produced %d foods", payload, len(decoded.Foods.Foods))
		}
	}
}

func TestFatSecretAPIErrorDetectedInA200Body(t *testing.T) {
	// The API answers 200 with an error document, so the status code cannot be
	// trusted on its own.
	err := fatSecretAPIError([]byte(`{"error":{"code":108,"message":"Invalid Type: meal is invalid"}}`))
	if err == nil {
		t.Fatal("error document was not detected")
	}
	if got := err.Error(); got != "fatsecret error 108: Invalid Type: meal is invalid" {
		t.Fatalf("error = %q", got)
	}

	if err := fatSecretAPIError([]byte(`{"foods":{"food":[]}}`)); err != nil {
		t.Fatalf("a good answer reported an error: %v", err)
	}
}

func TestHistorySourceRanksMostEaten(t *testing.T) {
	if got := historySource("foods.get_most_eaten"); got != "most_eaten" {
		t.Fatalf("most_eaten -> %q", got)
	}
	if got := historySource("foods.get_recently_eaten"); got != "recently_eaten" {
		t.Fatalf("recently_eaten -> %q", got)
	}
}

func TestCollectFoodHistoryRanksByPosition(t *testing.T) {
	// The account really has three products called "Банан"; the phrase "банан"
	// has to land on one, and position in the most-eaten list is the only signal
	// that says which.
	catalogue := map[string]*fsCatalogueEntry{}
	collectFoodHistory(catalogue, []fsCatalogueFood{
		{FoodID: "52853433", FoodName: "Банан", BrandName: "ВкусВилл"},
		{FoodID: "6569715", FoodName: "Банан", BrandName: "Пятерочка"},
		{FoodID: "8036794", FoodName: "Банан", BrandName: "Дикси"},
	}, "foods.get_most_eaten", "breakfast")

	if got := catalogue["52853433"].rank; got != 1 {
		t.Fatalf("first food rank = %d, want 1", got)
	}
	if got := catalogue["8036794"].rank; got != 3 {
		t.Fatalf("third food rank = %d, want 3", got)
	}
}

func TestCollectFoodHistoryKeepsTheBestRankAcrossMeals(t *testing.T) {
	catalogue := map[string]*fsCatalogueEntry{}
	// Fifth at breakfast, first at lunch: a habit either way.
	collectFoodHistory(catalogue, []fsCatalogueFood{
		{FoodID: "a"}, {FoodID: "b"}, {FoodID: "c"}, {FoodID: "d"}, {FoodID: "target"},
	}, "foods.get_most_eaten", "breakfast")
	collectFoodHistory(catalogue, []fsCatalogueFood{
		{FoodID: "target"},
	}, "foods.get_most_eaten", "lunch")

	entry := catalogue["target"]
	if entry.rank != 1 {
		t.Fatalf("rank = %d, want the best position 1", entry.rank)
	}
	if got := entry.mealList(); len(got) != 2 || got[0] != "breakfast" || got[1] != "lunch" {
		t.Fatalf("meals = %v, want breakfast then lunch", got)
	}
}

func TestCollectFoodHistoryIgnoresRecentOrderAsFrequency(t *testing.T) {
	// Position in a recently-eaten list means recency, not frequency, so it must
	// not masquerade as a rank.
	catalogue := map[string]*fsCatalogueEntry{}
	collectFoodHistory(catalogue, []fsCatalogueFood{
		{FoodID: "fresh", FoodName: "Онигири"},
	}, "foods.get_recently_eaten", "other")

	entry := catalogue["fresh"]
	if entry.rank != 0 {
		t.Fatalf("rank = %d, want 0 for a recently-eaten-only food", entry.rank)
	}
	if entry.source != "recently_eaten" {
		t.Fatalf("source = %q", entry.source)
	}
}

func TestCollectFoodHistoryPromotesSourceToMostEaten(t *testing.T) {
	catalogue := map[string]*fsCatalogueEntry{}
	collectFoodHistory(catalogue, []fsCatalogueFood{{FoodID: "x"}}, "foods.get_recently_eaten", "other")
	collectFoodHistory(catalogue, []fsCatalogueFood{{FoodID: "x"}}, "foods.get_most_eaten", "dinner")

	entry := catalogue["x"]
	if entry.source != "most_eaten" {
		t.Fatalf("source = %q, want most_eaten to win", entry.source)
	}
	if entry.rank != 1 {
		t.Fatalf("rank = %d, want 1", entry.rank)
	}
	if len(entry.mealList()) != 2 {
		t.Fatalf("meals = %v, want both", entry.mealList())
	}
}

func TestCollectFoodHistorySkipsFoodsWithoutID(t *testing.T) {
	catalogue := map[string]*fsCatalogueEntry{}
	collectFoodHistory(catalogue, []fsCatalogueFood{
		{FoodID: "", FoodName: "мусор"}, {FoodID: "real", FoodName: "Яйцо"},
	}, "foods.get_most_eaten", "breakfast")

	if len(catalogue) != 1 {
		t.Fatalf("catalogue = %d entries, want 1", len(catalogue))
	}
	// The skipped entry must not consume a rank position either.
	if catalogue["real"].rank != 2 {
		t.Fatalf("rank = %d, want the true position 2", catalogue["real"].rank)
	}
}

func TestDecodeDiaryEntryKeepsProviderIdentifiers(t *testing.T) {
	// Captured from food_entries.get. The connector used to declare neither
	// food_id nor serving_id, so both were silently discarded - and they are
	// exactly what food_entry.create needs to write a meal back.
	payload := `{"food_entry_id":"123","food_entry_name":"Snickers Сникерс Супер",
	  "food_id":"86855602","serving_id":"69898085","number_of_units":"2.000",
	  "meal":"Other","calories":"440","protein":"7","carbohydrate":"55","fat":"21"}`

	var entry fsFoodEntry
	if err := json.Unmarshal([]byte(payload), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if entry.FoodID != "86855602" {
		t.Fatalf("food_id = %q", entry.FoodID)
	}
	if entry.ServingID != "69898085" {
		t.Fatalf("serving_id = %q", entry.ServingID)
	}
	if entry.NumberOfUnits != "2.000" {
		t.Fatalf("number_of_units = %q", entry.NumberOfUnits)
	}
	// The diary name is the brand and product glued together, which is why a
	// name-based join against the split catalogue names matches almost nothing.
	if entry.FoodEntryName != "Snickers Сникерс Супер" {
		t.Fatalf("food_entry_name = %q", entry.FoodEntryName)
	}
}

func TestParseFloatOnProviderDecimalStrings(t *testing.T) {
	// number_of_units and the macros all arrive as strings.
	if got := parseFloat("2.000"); got != 2 {
		t.Fatalf(`parseFloat("2.000") = %v`, got)
	}
	if got := parseFloat("0.820"); got != 0.82 {
		t.Fatalf(`parseFloat("0.820") = %v`, got)
	}
	if got := parseFloat(""); got != 0 {
		t.Fatalf(`parseFloat("") = %v, want 0`, got)
	}
}
