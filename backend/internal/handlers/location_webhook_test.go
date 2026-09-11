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

func fixAt(offset time.Duration, lat, lon float64) locationPoint {
	base := time.Date(2026, 9, 12, 22, 0, 0, 0, time.UTC)
	return locationPoint{RecordedAt: base.Add(offset), Latitude: lat, Longitude: lon}
}

func TestThinLocationPointsCollapsesAPhoneLyingStill(t *testing.T) {
	// The shape of the first real batch: a fix every few seconds from one spot.
	points := make([]locationPoint, 0, 60)
	for i := 0; i < 60; i++ {
		points = append(points, fixAt(time.Duration(i)*10*time.Second, 55.755814, 37.617635))
	}

	kept := thinLocationPoints(nil, points)
	// Ten minutes of standing still: the first fix plus a heartbeat every three.
	if len(kept) != 4 {
		t.Fatalf("kept %d of %d fixes, want 4", len(kept), len(points))
	}
	if !kept[0].RecordedAt.Equal(points[0].RecordedAt) {
		t.Error("the first fix of the batch must always be kept")
	}
}

func TestThinLocationPointsKeepsMovement(t *testing.T) {
	// Walking: roughly 40 m every 20 seconds. Every fix is a different place.
	points := []locationPoint{
		fixAt(0, 55.755814, 37.617635),
		fixAt(20*time.Second, 55.756174, 37.617635),
		fixAt(40*time.Second, 55.756534, 37.617635),
	}
	if kept := thinLocationPoints(nil, points); len(kept) != 3 {
		t.Fatalf("kept %d of 3 fixes while moving", len(kept))
	}
}

func TestThinLocationPointsContinuesFromTheStoredFix(t *testing.T) {
	previous := fixAt(0, 55.755814, 37.617635)
	// The next batch starts seconds later from the same spot: nothing new yet.
	points := []locationPoint{fixAt(30*time.Second, 55.755814, 37.617635)}
	if kept := thinLocationPoints(&previous, points); len(kept) != 0 {
		t.Fatalf("kept %d fixes that repeat the stored one", len(kept))
	}
}

func TestThinLocationPointsRespectsPoorAccuracy(t *testing.T) {
	accuracy := 500.0
	previous := fixAt(0, 55.755814, 37.617635)
	// A 100 m "move" inside a 500 m error circle is noise, not a move.
	noisy := fixAt(30*time.Second, 55.756714, 37.617635)
	noisy.Accuracy = &accuracy
	if kept := thinLocationPoints(&previous, []locationPoint{noisy}); len(kept) != 0 {
		t.Fatalf("a move smaller than the fix's own error was stored")
	}
}

func TestMetersBetweenIsARealDistance(t *testing.T) {
	// Red Square to Moscow City is about 6 km.
	got := metersBetween(55.753930, 37.620393, 55.749650, 37.537130)
	if got < 5000 || got > 6500 {
		t.Fatalf("distance = %.0f m, want roughly 6 km", got)
	}
	if metersBetween(55.75, 37.62, 55.75, 37.62) != 0 {
		t.Fatal("a point is not zero metres from itself")
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
