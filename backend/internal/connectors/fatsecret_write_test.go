package connectors

import (
	"encoding/json"
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
