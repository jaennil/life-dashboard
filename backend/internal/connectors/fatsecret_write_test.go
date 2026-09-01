package connectors

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeCreateEntryValueEnvelope(t *testing.T) {
	// The real answer, captured from a live create: the id arrives wrapped in a
	// {"value": ...} object rather than as a scalar.
	var decoded fsCreateEntryResponse
	if err := json.Unmarshal([]byte(`{"food_entry_id": {"value": "24691181560"}}`), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.FoodEntryID.Value != "24691181560" {
		t.Fatalf("id = %q", decoded.FoodEntryID.Value)
	}
}

func TestDecodeValueEnvelopeToleratesBareScalars(t *testing.T) {
	for _, payload := range []string{`{"food_entry_id":"123"}`, `{"food_entry_id":123}`} {
		var decoded fsCreateEntryResponse
		if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
			t.Fatalf("unmarshal %s: %v", payload, err)
		}
		if decoded.FoodEntryID.Value != "123" {
			t.Fatalf("%s -> %q", payload, decoded.FoodEntryID.Value)
		}
	}
}

func TestDecodeDeleteConfirmation(t *testing.T) {
	var decoded fsDeleteEntryResponse
	if err := json.Unmarshal([]byte(`{"success": {"value": "1"}}`), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Success.Value != "1" {
		t.Fatalf("success = %q", decoded.Success.Value)
	}
}

func TestOAuth1EscapeFollowsRFC3986(t *testing.T) {
	// The three differences from form encoding that matter, and the one that
	// actually broke a live write.
	cases := map[string]string{
		"Простоквашино Молоко 2.5 %": "%D0%9F%D1%80%D0%BE%D1%81%D1%82%D0%BE%D0%BA%D0%B2%D0%B0%D1%88%D0%B8%D0%BD%D0%BE%20%D0%9C%D0%BE%D0%BB%D0%BE%D0%BA%D0%BE%202.5%20%25",
		"a b":               "a%20b",
		"~tilde":            "~tilde",
		"food_entry.create": "food_entry.create",
	}
	for input, want := range cases {
		if got := oauth1Escape(input); got != want {
			t.Errorf("oauth1Escape(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestOAuth1EscapeNeverProducesAPlus(t *testing.T) {
	// A "+" in a signature base string is the exact bug that produced
	// "Invalid signature" on the first food name with a space in it.
	for _, input := range []string{"два слова", "a b c", "  ", "Snickers Сникерс Супер"} {
		if got := oauth1Escape(input); strings.Contains(got, "+") {
			t.Errorf("oauth1Escape(%q) = %q, still form-encoded", input, got)
		}
	}
}
