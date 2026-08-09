package handlers

import (
	"encoding/json"
	"sort"
	"testing"
	"time"
)

func flatFieldsFrom(t *testing.T, body string) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &fields); err != nil {
		t.Fatalf("fixture is not valid JSON: %v", err)
	}
	return fields
}

func TestFlatHealthDay(t *testing.T) {
	now := time.Now().In(aiDisplayLocation)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, aiDisplayLocation)

	tests := []struct {
		name string
		body string
		want time.Time
	}{
		{name: "iso date", body: `{"date":"2026-08-08","steps":1}`, want: time.Date(2026, 8, 8, 0, 0, 0, 0, aiDisplayLocation)},
		{name: "dotted date", body: `{"day":"08.08.2026","steps":1}`, want: time.Date(2026, 8, 8, 0, 0, 0, 0, aiDisplayLocation)},
		{name: "missing date falls back to today", body: `{"steps":1}`, want: today},
		{name: "unparsable date falls back to today", body: `{"date":"whenever","steps":1}`, want: today},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := flatHealthDay(flatFieldsFrom(t, tc.body))
			if !got.Equal(tc.want) {
				t.Fatalf("day = %s, want %s", got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
	}
}

func TestParseFlatHealthMetrics(t *testing.T) {
	day := time.Date(2026, 8, 8, 0, 0, 0, 0, aiDisplayLocation)
	body := `{
		"api_key": "secret", "source": "apple_health", "event_type": "daily",
		"date": "2026-08-08", "timezone": "Europe/Moscow", "device": "iPhone",
		"steps": 12345,
		"step_count": 999,
		"hrv": "42.5",
		"resting_heart_rate": "58",
		"active_energy": 640,
		"time_in_daylight": 73,
		"headphone_audio_exposure": "71.2",
		"note": "ignored"
	}`

	entries, skipped := parseFlatHealthMetrics(flatFieldsFrom(t, body), day)
	if len(skipped) != 0 {
		t.Fatalf("unexpected skipped fields: %v", skipped)
	}

	byType := map[string]healthEntry{}
	for _, e := range entries {
		byType[e.Type] = e
	}

	// Control keys must never become metrics.
	for _, control := range []string{"api_key", "source", "event_type", "date", "timezone", "device", "note"} {
		if _, ok := byType[control]; ok {
			t.Errorf("control key %q leaked in as a metric", control)
		}
	}

	// step_count normalizes onto steps, so both spellings collapse to one type.
	if len(entries) != 7 {
		t.Fatalf("parsed %d entries, want 7: %v", len(entries), entries)
	}

	tests := []struct {
		metric string
		value  float64
		unit   string
	}{
		{metric: "hrv", value: 42.5, unit: "ms"},
		{metric: "resting_heart_rate", value: 58, unit: "bpm"},
		{metric: "active_energy", value: 640, unit: "kcal"},
		{metric: "time_in_daylight", value: 73, unit: ""},
		{metric: "headphone_audio_exposure", value: 71.2, unit: ""},
	}
	for _, tc := range tests {
		t.Run(tc.metric, func(t *testing.T) {
			entry, ok := byType[tc.metric]
			if !ok {
				t.Fatalf("metric %q missing", tc.metric)
			}
			if entry.Value != tc.value {
				t.Errorf("value = %v, want %v", entry.Value, tc.value)
			}
			if entry.Unit != tc.unit {
				t.Errorf("unit = %q, want %q", entry.Unit, tc.unit)
			}
			ts, err := parseHealthEntryTime(entry)
			if err != nil {
				t.Fatalf("timestamp unparsable: %v", err)
			}
			// A fixed stamp per day keeps repeated runs upserting one row rather
			// than piling up, and midday specifically survives the DATE(timestamp)
			// bucketing that existing queries do in the database's own timezone.
			if !ts.Equal(day.Add(12 * time.Hour)) {
				t.Errorf("timestamp = %s, want %s", ts.Format(time.RFC3339), day.Add(12*time.Hour).Format(time.RFC3339))
			}
			if got := ts.UTC().Format("2006-01-02"); got != "2026-08-08" {
				t.Errorf("DATE(timestamp) in UTC = %s, want 2026-08-08", got)
			}
			if got := ts.In(aiDisplayLocation).Format("2006-01-02"); got != "2026-08-08" {
				t.Errorf("DATE(timestamp) in Moscow = %s, want 2026-08-08", got)
			}
		})
	}
}

func TestParseFlatHealthMetricsReportsUnreadable(t *testing.T) {
	day := time.Date(2026, 8, 8, 0, 0, 0, 0, aiDisplayLocation)
	body := `{"date":"2026-08-08","steps":100,"weird":"not a number","empty":"","nested":{"a":1}}`

	entries, skipped := parseFlatHealthMetrics(flatFieldsFrom(t, body), day)
	if len(entries) != 1 || entries[0].Type != "steps" {
		t.Fatalf("entries = %v, want just steps", entries)
	}
	sort.Strings(skipped)
	want := []string{"empty", "nested", "weird"}
	if len(skipped) != len(want) {
		t.Fatalf("skipped = %v, want %v", skipped, want)
	}
	for i := range want {
		if skipped[i] != want[i] {
			t.Fatalf("skipped = %v, want %v", skipped, want)
		}
	}
}

func TestParseFlatHealthSleep(t *testing.T) {
	day := time.Date(2026, 8, 8, 0, 0, 0, 0, aiDisplayLocation)

	t.Run("no sleep fields", func(t *testing.T) {
		if _, ok := parseFlatHealthSleep(flatFieldsFrom(t, `{"steps":100}`), day); ok {
			t.Fatal("reported a sleep session for a payload without sleep fields")
		}
	})

	t.Run("full session", func(t *testing.T) {
		body := `{
			"date": "2026-08-08",
			"sleep_minutes": 431, "deep_sleep_minutes": 62, "rem_sleep_minutes": 95,
			"core_sleep_minutes": 274, "awake_minutes": 18, "in_bed_minutes": 465,
			"sleep_score": 84, "avg_hrv": "41.7", "avg_resting_hr": 55,
			"sleep_start": "2026-08-07T23:41:00", "sleep_end": "2026-08-08T07:12:00"
		}`
		session, ok := parseFlatHealthSleep(flatFieldsFrom(t, body), day)
		if !ok {
			t.Fatal("sleep session not detected")
		}
		if session.Date != "2026-08-08" {
			t.Errorf("date = %q, want 2026-08-08", session.Date)
		}
		if session.TotalSleepMinutes != 431 || session.DeepSleepMinutes != 62 ||
			session.REMSleepMinutes != 95 || session.LightSleepMinutes != 274 ||
			session.AwakeMinutes != 18 || session.TotalMinutes != 465 {
			t.Errorf("minutes wrong: %+v", session)
		}
		if session.SleepScore != 84 || session.AvgHRV != 41.7 || session.AvgRestingHR != 55 {
			t.Errorf("scores wrong: %+v", session)
		}
		if session.StartDate == "" || session.EndDate == "" {
			t.Errorf("start/end missing: %+v", session)
		}
		if _, err := parseDate(session.StartDate); err != nil {
			t.Errorf("start date unparsable by the upsert path: %v", err)
		}
	})

	t.Run("core maps onto light", func(t *testing.T) {
		session, ok := parseFlatHealthSleep(flatFieldsFrom(t, `{"core_sleep_minutes":300}`), day)
		if !ok || session.LightSleepMinutes != 300 {
			t.Fatalf("session = %+v, want light=300", session)
		}
	})
}

func TestHealthFlatNumber(t *testing.T) {
	tests := []struct {
		raw   string
		want  float64
		valid bool
	}{
		{raw: `12345`, want: 12345, valid: true},
		{raw: `42.5`, want: 42.5, valid: true},
		{raw: `"42.5"`, want: 42.5, valid: true},
		{raw: `"42,5"`, want: 42.5, valid: true},
		{raw: `"12 345"`, want: 12345, valid: true},
		{raw: `"12 345"`, want: 12345, valid: true},
		{raw: `""`, valid: false},
		{raw: `"abc"`, valid: false},
		{raw: `null`, valid: false},
		{raw: `{"a":1}`, valid: false},
	}

	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			got, ok := healthFlatNumber(json.RawMessage(tc.raw))
			if ok != tc.valid {
				t.Fatalf("ok = %v, want %v", ok, tc.valid)
			}
			if ok && got != tc.want {
				t.Fatalf("value = %v, want %v", got, tc.want)
			}
		})
	}
}
