package handlers

import (
	"encoding/json"
	"testing"
	"time"
)

func overlandFixture() overlandBatch {
	accuracy := 8.5
	altitude := 156.0
	speed := 1.4
	course := 270.0
	battery := 0.62
	batch := overlandBatch{Locations: []overlandFeature{{}, {}}}

	// A normal fix.
	batch.Locations[0].Geometry.Coordinates = []float64{37.617635, 55.755814}
	batch.Locations[0].Properties = overlandRawProperties(overlandProperties{
		Timestamp: "2026-09-12T07:31:04Z", HorizontalAccuracy: &accuracy, Altitude: &altitude,
		Speed: &speed, Course: &course, Motion: []string{"Walking", " stationary "},
		BatteryState: "unplugged", BatteryLevel: &battery, Wifi: "home-wifi", DeviceID: "iphone",
	})
	// A fix with no coordinates at all.
	batch.Locations[1].Properties = overlandRawProperties(overlandProperties{Timestamp: "2026-09-12T07:32:04Z"})
	return batch
}

func overlandRawProperties(properties overlandProperties) json.RawMessage {
	encoded, err := json.Marshal(properties)
	if err != nil {
		panic(err)
	}
	return encoded
}

func TestParseOverlandPointsReadsGeoJSONOrder(t *testing.T) {
	points, _ := parseOverlandBatch(overlandFixture())
	if len(points) != 1 {
		t.Fatalf("got %d points, want 1 - a fix without coordinates must be dropped", len(points))
	}

	point := points[0]
	// GeoJSON is [longitude, latitude]: swapping these puts Moscow in Somalia.
	if point.Latitude != 55.755814 || point.Longitude != 37.617635 {
		t.Fatalf("coordinates swapped: lat=%v lon=%v", point.Latitude, point.Longitude)
	}
	if !point.RecordedAt.Equal(time.Date(2026, 9, 12, 7, 31, 4, 0, time.UTC)) {
		t.Fatalf("recorded_at = %s", point.RecordedAt)
	}
	if point.Accuracy == nil || *point.Accuracy != 8.5 {
		t.Fatalf("accuracy = %v", point.Accuracy)
	}
	if len(point.Motion) != 2 || point.Motion[0] != "walking" || point.Motion[1] != "stationary" {
		t.Fatalf("motion = %v", point.Motion)
	}
	if point.BatteryState != "unplugged" || point.DeviceID != "iphone" {
		t.Fatalf("device fields lost: %+v", point)
	}
	if len(point.Raw) == 0 {
		t.Fatal("raw properties were not kept")
	}
}

func TestParseOverlandPointsDropsUnreadableTime(t *testing.T) {
	batch := overlandFixture()
	batch.Locations[0].Properties = overlandRawProperties(overlandProperties{Timestamp: "не время"})
	if points, _ := parseOverlandBatch(batch); len(points) != 0 {
		t.Fatalf("got %d points, want none: a fix without a time answers no question", len(points))
	}
}

func TestParseOverlandTimeAcceptsShippedFormats(t *testing.T) {
	want := time.Date(2026, 9, 12, 7, 31, 4, 0, time.UTC)
	for _, value := range []string{
		"2026-09-12T07:31:04Z",
		"2026-09-12T10:31:04+03:00",
		"2026-09-12T07:31:04.000Z",
	} {
		got, ok := parseOverlandTime(value)
		if !ok || !got.Equal(want) {
			t.Errorf("parseOverlandTime(%q) = %s, %v", value, got, ok)
		}
	}
	if _, ok := parseOverlandTime("  "); ok {
		t.Error("an empty timestamp was accepted")
	}
}

func TestTruncateFieldKeepsValidUTF8(t *testing.T) {
	// Cyrillic SSIDs are two bytes per rune, so a byte cut can split one.
	if got := truncateField("сеть", 3); got != "с" {
		t.Fatalf("truncateField produced %q", got)
	}
	if got := truncateField("  iphone  ", 100); got != "iphone" {
		t.Fatalf("truncateField produced %q", got)
	}
}

func visitFeature(t *testing.T, properties map[string]any) overlandFeature {
	t.Helper()
	encoded, err := json.Marshal(properties)
	if err != nil {
		t.Fatal(err)
	}
	feature := overlandFeature{Properties: encoded}
	feature.Geometry.Coordinates = []float64{37.534392, 55.708392}
	return feature
}

func TestParseOverlandBatchReadsVisits(t *testing.T) {
	batch := overlandBatch{Locations: []overlandFeature{visitFeature(t, map[string]any{
		"timestamp":           "2026-09-12T19:02:11Z",
		"action":              "visit",
		"arrival_date":        "2026-09-12T18:20:00Z",
		"departure_date":      "2026-09-12T18:47:30Z",
		"horizontal_accuracy": 65,
		"wifi":                "shop-guest",
		"device_id":           "iphone",
	})}}

	points, visits := parseOverlandBatch(batch)
	if len(points) != 0 {
		t.Fatalf("a visit was also stored as %d plain fixes", len(points))
	}
	if len(visits) != 1 {
		t.Fatalf("got %d visits, want 1", len(visits))
	}

	visit := visits[0]
	if !visit.ArrivedAt.Equal(time.Date(2026, 9, 12, 18, 20, 0, 0, time.UTC)) {
		t.Errorf("arrived_at = %s", visit.ArrivedAt)
	}
	if visit.DepartedAt == nil || !visit.DepartedAt.Equal(time.Date(2026, 9, 12, 18, 47, 30, 0, time.UTC)) {
		t.Errorf("departed_at = %v", visit.DepartedAt)
	}
	if visit.Latitude != 55.708392 || visit.Longitude != 37.534392 {
		t.Errorf("coordinates = %v %v", visit.Latitude, visit.Longitude)
	}
}

func TestParseOverlandBatchHandlesOpenAndUnstartedVisits(t *testing.T) {
	// Still there: iOS reports the arrival now and the departure later.
	_, visits := parseOverlandBatch(overlandBatch{Locations: []overlandFeature{visitFeature(t, map[string]any{
		"timestamp": "2026-09-12T19:02:11Z", "action": "visit",
		"arrival_date": "2026-09-12T18:20:00Z", "departure_date": nil,
	})}})
	if len(visits) != 1 || visits[0].DepartedAt != nil {
		t.Fatalf("an open visit was not kept open: %+v", visits)
	}

	// Tracking started while already at the place: the event's own time stands in.
	_, visits = parseOverlandBatch(overlandBatch{Locations: []overlandFeature{visitFeature(t, map[string]any{
		"timestamp": "2026-09-12T19:02:11Z", "action": "visit",
		"arrival_date": nil, "departure_date": "2026-09-12T19:02:00Z",
	})}})
	if len(visits) != 1 {
		t.Fatalf("a visit without an arrival was dropped")
	}
	// The arrival is genuinely unknown, so the row is anchored on the departure
	// rather than on a made-up arrival after it.
	if !visits[0].ArrivedAt.Equal(time.Date(2026, 9, 12, 19, 2, 0, 0, time.UTC)) {
		t.Errorf("arrived_at = %s, want the departure time", visits[0].ArrivedAt)
	}
	if visits[0].DepartedAt == nil || !visits[0].DepartedAt.Equal(visits[0].ArrivedAt) {
		t.Errorf("departed_at = %v", visits[0].DepartedAt)
	}
}

func TestParseOverlandBatchRejectsContradictoryVisit(t *testing.T) {
	_, visits := parseOverlandBatch(overlandBatch{Locations: []overlandFeature{visitFeature(t, map[string]any{
		"timestamp": "2026-09-12T19:02:11Z", "action": "visit",
		"arrival_date": "2026-09-12T18:20:00Z", "departure_date": "2026-09-12T17:00:00Z",
	})}})
	if len(visits) != 0 {
		t.Fatalf("a visit that ended before it started was stored: %+v", visits)
	}
}
