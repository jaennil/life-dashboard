package connectors

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestParseHomeAssistantValueRejectsAbsence(t *testing.T) {
	// Home Assistant says "no reading" in words, and the phone reports plenty of
	// them: HRV, VO2 max and blood pressure are all unavailable on a phone with
	// no watch.
	for _, absent := range []string{"unavailable", "unknown", "Unknown", "none", "", "   "} {
		if _, ok := parseHomeAssistantValue(absent); ok {
			t.Errorf("%q was read as a measurement", absent)
		}
	}
	// A state that is not a number at all is not a measurement either.
	if _, ok := parseHomeAssistantValue("Built-in Speaker"); ok {
		t.Error("a text state was read as a measurement")
	}

	value, ok := parseHomeAssistantValue("97")
	if !ok || value != 97 {
		t.Fatalf("got %v, %v", value, ok)
	}
	if value, ok := parseHomeAssistantValue("18.1"); !ok || value != 18.1 {
		t.Fatalf("got %v, %v", value, ok)
	}
}

func TestHomeAssistantStampKeepsAMeasurementWhenItHappened(t *testing.T) {
	state := homeAssistantState{LastUpdated: "2026-09-13T07:24:11.000000+00:00"}
	stamp, err := homeAssistantStamp(state, false)
	if err != nil {
		t.Fatal(err)
	}
	if !stamp.Equal(time.Date(2026, 9, 13, 7, 24, 11, 0, time.UTC)) {
		t.Fatalf("stamp = %s", stamp)
	}
}

func TestHomeAssistantStampPutsADailyCounterOnItsOwnDay(t *testing.T) {
	// A counter read at 00:23 Moscow time belongs to that Moscow day, not to the
	// UTC day that was still yesterday.
	state := homeAssistantState{LastUpdated: "2026-09-12T21:23:00+00:00"}
	stamp, err := homeAssistantStamp(state, true)
	if err != nil {
		t.Fatal(err)
	}

	local := stamp.In(homeAssistantLocation)
	if local.Day() != 13 || local.Hour() != 12 {
		t.Fatalf("daily counter landed at %s, want midday of the 13th", local)
	}
}

func TestHomeAssistantReadingsCoverWhatNothingElseHas(t *testing.T) {
	// The point of this connector is the readings no other source provides. If
	// one of them is dropped from the list, this is the test that notices.
	wanted := map[string]bool{"spo2": false, "respiratory_rate": false, "vo2max": false, "hrv": false}
	daily := map[string]bool{}
	for _, reading := range homeAssistantReadings {
		if _, ok := wanted[reading.metric]; ok {
			wanted[reading.metric] = true
		}
		if reading.daily {
			daily[reading.metric] = true
		}
		if reading.entity == "" || reading.metric == "" || reading.unit == "" {
			t.Errorf("incomplete reading: %+v", reading)
		}
	}
	for metric, present := range wanted {
		if !present {
			t.Errorf("%s is no longer collected", metric)
		}
	}
	// Steps and active energy reset at midnight; treating them as measurements
	// would scatter a day's total across a dozen rows.
	if !daily["steps"] || !daily["active_energy"] {
		t.Errorf("counters that reset are not marked daily: %v", daily)
	}
}

func TestHomeAssistantConfiguredNeedsBothHalves(t *testing.T) {
	cases := []struct {
		baseURL, token string
		want           bool
	}{
		{"http://ha:8123", "token", true},
		{"http://ha:8123", "", false},
		{"", "token", false},
		{"  ", "  ", false},
	}
	for _, tc := range cases {
		c := NewHomeAssistant(nil, tc.baseURL, tc.token, "mobile_app_iphone", zerolog.Nop())
		if c.Configured() != tc.want {
			t.Errorf("Configured(%q, %q) = %v", tc.baseURL, tc.token, c.Configured())
		}
	}
}
