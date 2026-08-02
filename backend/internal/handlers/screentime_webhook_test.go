package handlers

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestEscapeLiteralNewlines(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "literal newline inside string value",
			body: "{\"screen_time\":\"Instagram\n1 hr 23 min\"}",
			want: `{"screen_time":"Instagram\n1 hr 23 min"}`,
		},
		{
			name: "structural newlines are preserved",
			body: "{\n\"a\": 1\n}",
			want: "{\n\"a\": 1\n}",
		},
		{
			name: "already escaped sequences are untouched",
			body: `{"a":"x\ny"}`,
			want: `{"a":"x\ny"}`,
		},
		{
			name: "escaped quote does not flip string state",
			body: "{\"a\":\"say \\\"hi\\\"\nbye\"}",
			want: `{"a":"say \"hi\"\nbye"}`,
		},
		{
			name: "carriage return and tab inside value",
			body: "{\"a\":\"x\r\ty\"}",
			want: `{"a":"x\r\ty"}`,
		},
		{
			name: "trailing backslash does not panic",
			body: `{"a":"x\`,
			want: `{"a":"x\`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(escapeLiteralNewlines([]byte(tc.body)))
			if got != tc.want {
				t.Fatalf("escapeLiteralNewlines() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEscapeLiteralNewlinesMakesShortcutsBodyValid(t *testing.T) {
	// A body shaped like what the iOS Shortcuts "Get Contents of URL" action is
	// expected to send when the Screen Time activity variable is a multi-line blob.
	body := []byte("{\"api_key\":\"deadbeef\",\"day\":\"2026-08-01\",\"screen_time\":\"Telegram — 2 hr 4 min\nSafari — 41 min\"}")
	if json.Valid(body) {
		t.Fatal("fixture should be invalid JSON before repair")
	}

	fixed := escapeLiteralNewlines(body)
	if !json.Valid(fixed) {
		t.Fatalf("repaired body is still invalid JSON: %s", fixed)
	}

	var envelope struct {
		APIKey     string `json:"api_key"`
		Day        string `json:"day"`
		ScreenTime string `json:"screen_time"`
	}
	if err := json.Unmarshal(fixed, &envelope); err != nil {
		t.Fatalf("unmarshal repaired body: %v", err)
	}
	if envelope.APIKey != "deadbeef" || envelope.Day != "2026-08-01" {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
	if envelope.ScreenTime != "Telegram — 2 hr 4 min\nSafari — 41 min" {
		t.Fatalf("line boundaries lost: %q", envelope.ScreenTime)
	}
}

func TestTopLevelKeys(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    []string
	}{
		{name: "empty payload", payload: "", want: []string{}},
		{name: "sorted object keys", payload: `{"screen_time":"x","api_key":"y"}`, want: []string{"api_key", "screen_time"}},
		{name: "array payload", payload: `[1,2]`, want: []string{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := topLevelKeys(json.RawMessage(tc.payload))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("topLevelKeys() = %v, want %v", got, tc.want)
			}
		})
	}
}
