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
	points := parseOverlandPoints(overlandFixture())
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
	if points := parseOverlandPoints(batch); len(points) != 0 {
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
