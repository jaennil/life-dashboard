package handlers

import (
	"encoding/json"
	"testing"
)

// Shaped exactly like a real Health Auto Export payload, including the imperial
// units the app emits when the phone is set to US measurements.
const healthAutoExportFixture = `{"data":{
  "metrics":[
    {"name":"step_count","units":"count","data":[
      {"source":"iPhone","qty":196,"date":"2026-01-10 00:00:00 +0300"},
      {"source":"iPhone","qty":8421,"date":"2026-01-11 00:00:00 +0300"}]},
    {"name":"weight_body_mass","units":"lb","data":[
      {"qty":154.3235835294143,"source":"fatsecret","date":"2025-01-31 00:00:00 +0300"}]},
    {"name":"walking_running_distance","units":"mi","data":[
      {"source":"iPhone","date":"2026-01-10 00:00:00 +0300","qty":2}]},
    {"name":"walking_step_length","units":"in","data":[
      {"source":"iPhone","date":"2026-01-11 00:00:00 +0300","qty":30}]},
    {"name":"heart_rate","units":"count/min","data":[
      {"source":"Watch","date":"2026-01-11 14:30:00 +0300","Avg":62.5,"Min":48,"Max":131}]},
    {"name":"body_mass_index","units":"count","data":[
      {"source":"fatsecret","date":"2026-01-31 00:00:00 +0300","qty":22.4}]},
    {"name":"basal_energy_burned","units":"kcal","data":[
      {"source":"iPhone","date":"2026-01-11 00:00:00 +0300","qty":1700}]}],
  "workouts":[{"id":"a","name":"Outdoor Run"},{"id":"b","name":"Walk"}],
  "stateOfMind":[{"id":"c","kind":"momentary_emotion","valence":0.69}]}}`

func TestLooksLikeHealthAutoExport(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "real export", body: healthAutoExportFixture, want: true},
		{name: "flat shortcut payload", body: `{"api_key":"x","steps":100}`, want: false},
		{name: "typed array payload", body: `{"api_key":"x","data":[{"type":"steps","value":1}]}`, want: false},
		{name: "data object without metrics", body: `{"data":{"workouts":[]}}`, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.body), &fields); err != nil {
				t.Fatalf("fixture invalid: %v", err)
			}
			if got := looksLikeHealthAutoExport(fields); got != tc.want {
				t.Fatalf("looksLikeHealthAutoExport = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseHealthAutoExport(t *testing.T) {
	entries, report, err := parseHealthAutoExport([]byte(healthAutoExportFixture))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if report.Metrics != 7 {
		t.Errorf("metrics = %d, want 7", report.Metrics)
	}
	if report.Points != 8 {
		t.Errorf("points = %d, want 8", report.Points)
	}
	if report.UnreadablePoints != 0 {
		t.Errorf("unreadable = %d, want 0", report.UnreadablePoints)
	}
	// Sections without a table of their own must be counted, not silently dropped.
	if report.Workouts != 2 || report.StateOfMind != 1 {
		t.Errorf("ignored sections: workouts=%d stateOfMind=%d, want 2 and 1", report.Workouts, report.StateOfMind)
	}
	if len(report.Converted) != 3 {
		t.Errorf("converted units = %v, want 3 entries (lb, mi, in)", report.Converted)
	}

	byType := map[string]healthEntry{}
	for _, e := range entries {
		if _, seen := byType[e.Type]; !seen || e.Type != "steps" {
			byType[e.Type] = e
		}
	}

	t.Run("imperial units are converted", func(t *testing.T) {
		weight := byType["weight"]
		if weight.Unit != "kg" {
			t.Errorf("unit = %q, want kg", weight.Unit)
		}
		// 154.32 lb is ~70 kg; storing the raw pounds would corrupt the series
		// that the scale connector fills in kilograms.
		if weight.Value < 69.9 || weight.Value > 70.1 {
			t.Errorf("weight = %v kg, want ~70", weight.Value)
		}

		dist := byType["walking_running_distance"]
		if dist.Unit != "m" || dist.Value < 3218 || dist.Value > 3219 {
			t.Errorf("distance = %v %s, want ~3218.7 m", dist.Value, dist.Unit)
		}

		step := byType["walking_step_length"]
		if step.Unit != "cm" || step.Value < 76.1 || step.Value > 76.3 {
			t.Errorf("step length = %v %s, want 76.2 cm", step.Value, step.Unit)
		}
	})

	t.Run("names map onto existing metric types", func(t *testing.T) {
		for _, want := range []string{"steps", "weight", "bmi", "bmr"} {
			if _, ok := byType[want]; !ok {
				t.Errorf("metric type %q missing; got %v", want, keysOf(byType))
			}
		}
	})

	t.Run("discrete metrics fall back to Avg", func(t *testing.T) {
		hr := byType["heart_rate"]
		if hr.Value != 62.5 {
			t.Errorf("heart rate = %v, want 62.5 from Avg", hr.Value)
		}
	})

	t.Run("daily points move to local midday", func(t *testing.T) {
		steps := byType["steps"]
		ts, err := parseHealthEntryTime(steps)
		if err != nil {
			t.Fatalf("timestamp unparsable: %v", err)
		}
		if ts.UTC().Format("2006-01-02") != "2026-01-10" && ts.UTC().Format("2006-01-02") != "2026-01-11" {
			t.Errorf("DATE in UTC = %s, want the payload's own day", ts.UTC().Format("2006-01-02"))
		}
		if ts.Hour() != 12 {
			t.Errorf("hour = %d, want 12 so DATE(timestamp) is stable across timezones", ts.Hour())
		}
	})

	t.Run("intraday points keep their time", func(t *testing.T) {
		hr := byType["heart_rate"]
		ts, _ := parseHealthEntryTime(hr)
		if ts.Hour() != 14 || ts.Minute() != 30 {
			t.Errorf("time = %02d:%02d, want 14:30 untouched", ts.Hour(), ts.Minute())
		}
	})

	t.Run("provenance is kept in metadata", func(t *testing.T) {
		weight := byType["weight"]
		if weight.Metadata["hae_source"] != "fatsecret" {
			t.Errorf("hae_source = %v, want fatsecret", weight.Metadata["hae_source"])
		}
		if weight.Metadata["hae_metric"] != "weight_body_mass" {
			t.Errorf("hae_metric = %v, want weight_body_mass", weight.Metadata["hae_metric"])
		}
	})
}

func keysOf(m map[string]healthEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestParseHealthAutoExportDate(t *testing.T) {
	tests := []struct {
		value    string
		wantHour int
		wantDay  string
		ok       bool
	}{
		{value: "2026-01-10 00:00:00 +0300", wantHour: 12, wantDay: "2026-01-10", ok: true},
		{value: "2026-01-11 14:30:00 +0300", wantHour: 14, wantDay: "2026-01-11", ok: true},
		{value: "2026-01-11T14:30:00+03:00", wantHour: 14, wantDay: "2026-01-11", ok: true},
		{value: "nonsense", ok: false},
		{value: "", ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			got, err := parseHealthAutoExportDate(tc.value)
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
			if !tc.ok {
				return
			}
			if got.Hour() != tc.wantHour {
				t.Errorf("hour = %d, want %d", got.Hour(), tc.wantHour)
			}
			if got.Format("2006-01-02") != tc.wantDay {
				t.Errorf("day = %s, want %s", got.Format("2006-01-02"), tc.wantDay)
			}
		})
	}
}

func TestConvertHealthUnit(t *testing.T) {
	tests := []struct {
		unit      string
		in        float64
		wantUnit  string
		wantValue float64
		converted bool
	}{
		{unit: "lb", in: 100, wantUnit: "kg", wantValue: 45.359237, converted: true},
		{unit: "mi", in: 1, wantUnit: "m", wantValue: 1609.344, converted: true},
		{unit: "in", in: 10, wantUnit: "cm", wantValue: 25.4, converted: true},
		{unit: "mi/hr", in: 10, wantUnit: "km/hr", wantValue: 16.09344, converted: true},
		{unit: "count", in: 5, wantUnit: "count", wantValue: 5},
		{unit: "kcal", in: 5, wantUnit: "kcal", wantValue: 5},
		{unit: "%", in: 5, wantUnit: "%", wantValue: 5},
		{unit: "", in: 5, wantUnit: "", wantValue: 5},
	}

	for _, tc := range tests {
		t.Run(tc.unit, func(t *testing.T) {
			value, unit, converted := convertHealthUnit(tc.in, tc.unit)
			if unit != tc.wantUnit || converted != tc.converted {
				t.Fatalf("unit = %q converted = %v, want %q and %v", unit, converted, tc.wantUnit, tc.converted)
			}
			if diff := value - tc.wantValue; diff > 1e-6 || diff < -1e-6 {
				t.Fatalf("value = %v, want %v", value, tc.wantValue)
			}
		})
	}
}
