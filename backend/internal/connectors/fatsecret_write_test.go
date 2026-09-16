package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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

func TestFatSecretErrorNeverCarriesTheSignedURL(t *testing.T) {
	// Every FatSecret URL is signed in the query string, so it carries the
	// consumer key, the access token and the signature. One timeout on
	// food_entry.create put all three into the notification and the log store.
	const secretish = "oauth_consumer_key=98d578e177854a3eb4c013345cb82c1c"
	err := fatSecretTransportError("food_entry.create", &url.Error{
		Op:  "Get",
		URL: "https://platform.fatsecret.com/rest/server.api?" + secretish + "&oauth_token=08d19cd3&oauth_signature=znuJ",
		Err: errors.New("context deadline exceeded (Client.Timeout exceeded while awaiting headers)"),
	})

	if strings.Contains(err.Error(), "oauth_") {
		t.Errorf("credentials are still in the error: %s", err)
	}
	for _, want := range []string{"food_entry.create", "deadline exceeded"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error lost %q: %s", want, err)
		}
	}
}

func TestOnlyTransportFailuresAreWorthRepeating(t *testing.T) {
	// A timeout may have delivered the write; a refusal is a decision.
	repeatable := []error{
		&url.Error{Op: "Get", URL: "https://platform.fatsecret.com/rest/server.api?x=1", Err: context.DeadlineExceeded},
		fatSecretTransportError("food_entry.create", &url.Error{Err: errors.New("EOF")}),
		context.DeadlineExceeded,
	}
	for _, err := range repeatable {
		if !isFatSecretTransportFailure(err) {
			t.Errorf("not treated as repeatable: %v", err)
		}
	}

	final := []error{
		errors.New("fatsecret error 8: Invalid signature"),
		fmt.Errorf("create entry returned no id: {}"),
	}
	for _, err := range final {
		if isFatSecretTransportFailure(err) {
			t.Errorf("a refusal would be repeated: %v", err)
		}
	}
}
