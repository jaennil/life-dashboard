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
