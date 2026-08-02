package connectors

import (
	"encoding/json"
	"testing"
)

// Payloads shaped exactly like the eco/scale endpoint returns for a Body
// Composition Scale S400 (yunmai.scales.ms104), trimmed to the fields under
// test and with identifiers anonymised.
const (
	// Account owner, full measurement.
	ownerFullPayload = `{"duid":"1","userType":"1","weight":77.8,"heartRate":"90","status":"0",
		"bfp":19.4,"slm":59.3,"bwp":56.9,"bmc":3.4,"vfl":"8","smm":32.3,"bmi":22.5,
		"bmr":"1724","ma":"20","ffm":62.7,"pm":14.3,"bodyRes":641.2,"bodyRes2":584.2,
		"user":{"name":"owner","uid":"1111111111","accountId":"1111111111","height":"186"}}`

	// Account owner stepping on with shoes: weight only, everything derived is zero.
	ownerWeightOnlyPayload = `{"duid":"1","userType":"1","weight":76.4,"heartRate":"0","status":"1",
		"bfp":0,"slm":0,"bwp":0,"bmc":0,"vfl":"0","smm":0,"bmi":22.1,
		"bmr":"0","ma":"0","ffm":0,"pm":0,"bodyRes":0,"bodyRes2":0,
		"user":{"name":"owner","uid":"1111111111","accountId":"1111111111","height":"186"}}`

	// A second profile on the same scale. Note the identical outer uid: only
	// the inner accountId tells the two people apart.
	familyMemberPayload = `{"duid":"2","userType":"2","weight":57.2,"heartRate":"0","status":"1",
		"bfp":0,"bmi":20.03,
		"user":{"name":"family","uid":"1111111111","accountId":"2222222222","height":"169"}}`
)

const testAccountID = "1111111111"

func decodeScalePayload(t *testing.T, raw string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return payload
}

func TestXiaomiScaleOwnedBy(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    bool
	}{
		{"account owner", ownerFullPayload, true},
		{"family member on the same account", familyMemberPayload, false},
		{"no user block falls back to accepting", `{"weight":70}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := xiaomiScaleOwnedBy(decodeScalePayload(t, tt.payload), testAccountID)
			if got != tt.want {
				t.Errorf("xiaomiScaleOwnedBy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestXiaomiScaleValuesFullMeasurement(t *testing.T) {
	values := xiaomiScaleValues(decodeScalePayload(t, ownerFullPayload))

	got := make(map[string]float64, len(values))
	units := make(map[string]string, len(values))
	for _, v := range values {
		got[v.metricType] = v.value
		units[v.metricType] = v.unit
	}

	want := map[string]float64{
		"weight": 77.8, "bmi": 22.5, "body_fat": 19.4, "lean_body_mass": 62.7,
		"muscle_mass": 59.3, "skeletal_muscle_mass": 32.3, "body_water": 56.9,
		"bone_mass": 3.4, "protein_mass": 14.3, "visceral_fat": 8, "bmr": 1724,
		"metabolic_age": 20, "heart_rate": 90, "impedance_low": 641.2, "impedance_high": 584.2,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d metrics, want %d: %v", len(got), len(want), got)
	}
	for metric, wantValue := range want {
		if got[metric] != wantValue {
			t.Errorf("%s = %v, want %v", metric, got[metric], wantValue)
		}
	}

	// Metrics Xiaomi encodes as strings must still carry their unit.
	if units["bmr"] != "kcal" || units["heart_rate"] != "bpm" {
		t.Errorf("unexpected units: bmr=%q heart_rate=%q", units["bmr"], units["heart_rate"])
	}

	// The 50 kHz band always reads higher than the 250 kHz one; if this flips,
	// the impedance_low/impedance_high labels are the wrong way round.
	if got["impedance_low"] <= got["impedance_high"] {
		t.Errorf("impedance_low (%v) should exceed impedance_high (%v)",
			got["impedance_low"], got["impedance_high"])
	}
}

func TestXiaomiScaleValuesSkipsUnmeasured(t *testing.T) {
	values := xiaomiScaleValues(decodeScalePayload(t, ownerWeightOnlyPayload))

	got := make(map[string]float64, len(values))
	for _, v := range values {
		got[v.metricType] = v.value
	}

	// Only the two things the scale can report without impedance survive.
	if len(got) != 2 || got["weight"] != 76.4 || got["bmi"] != 22.1 {
		t.Fatalf("weight-only weigh-in produced %v, want just weight and bmi", got)
	}
	for _, metric := range []string{"body_fat", "muscle_mass", "bmr", "heart_rate", "impedance_low"} {
		if _, ok := got[metric]; ok {
			t.Errorf("%s should be dropped when the scale reported zero", metric)
		}
	}
}

func TestXiaomiScaleNumber(t *testing.T) {
	payload := decodeScalePayload(t, `{"num":19.4,"str":"1724","blank":"","junk":"abc","null":null,"neg":-1}`)

	tests := []struct {
		key   string
		want  float64
		wantK bool
	}{
		{"num", 19.4, true},
		{"str", 1724, true},
		{"neg", -1, true},
		{"blank", 0, false},
		{"junk", 0, false},
		{"null", 0, false},
		{"missing", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := xiaomiScaleNumber(payload, tt.key)
			if ok != tt.wantK || got != tt.want {
				t.Errorf("xiaomiScaleNumber(%q) = (%v, %v), want (%v, %v)", tt.key, got, ok, tt.want, tt.wantK)
			}
		})
	}
}
