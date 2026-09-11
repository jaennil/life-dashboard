package handlers

import "testing"

func TestClassifyPlacesFindsHomeAndWork(t *testing.T) {
	kinds := classifyPlaces([]placeUsage{
		// Slept at, every night.
		{PlaceID: "flat", TotalHours: 90, NightHours: 60, WorkdayHours: 6},
		// Weekdays, business hours, never overnight.
		{PlaceID: "office", TotalHours: 45, NightHours: 0, WorkdayHours: 42},
		// A gym: regular, but neither slept at nor a working day.
		{PlaceID: "gym", TotalHours: 6, NightHours: 0, WorkdayHours: 1},
	})

	if kinds["flat"] != placeKindHome {
		t.Errorf("flat = %q, want home", kinds["flat"])
	}
	if kinds["office"] != placeKindWork {
		t.Errorf("office = %q, want work", kinds["office"])
	}
	if kinds["gym"] != placeKindOther {
		t.Errorf("gym = %q, want other", kinds["gym"])
	}
}

func TestClassifyPlacesLeavesThinHistoryAlone(t *testing.T) {
	// One night somewhere is a hotel, not a home.
	kinds := classifyPlaces([]placeUsage{
		{PlaceID: "hotel", TotalHours: 2, NightHours: 2},
		{PlaceID: "cafe", TotalHours: 1, WorkdayHours: 1},
	})
	for id, kind := range kinds {
		if kind != placeKindOther {
			t.Errorf("%s = %q on two hours of history, want other", id, kind)
		}
	}
}

func TestClassifyPlacesDoesNotMakeHomeWork(t *testing.T) {
	// Working from home: the flat has the weekday hours too, but it is still one
	// place and it is home. Nothing else qualifies as work.
	kinds := classifyPlaces([]placeUsage{
		{PlaceID: "flat", TotalHours: 140, NightHours: 60, WorkdayHours: 50},
	})
	if kinds["flat"] != placeKindHome {
		t.Fatalf("flat = %q, want home", kinds["flat"])
	}
	for id, kind := range kinds {
		if kind == placeKindWork {
			t.Fatalf("%s was labelled work while being home", id)
		}
	}
}

func TestClassifyPlacesIgnoresADaytimeNap(t *testing.T) {
	// A place with plenty of hours but mostly daytime is not home, even if some
	// of those hours are at night.
	kinds := classifyPlaces([]placeUsage{
		{PlaceID: "office", TotalHours: 50, NightHours: 8, WorkdayHours: 40},
		{PlaceID: "flat", TotalHours: 40, NightHours: 35, WorkdayHours: 2},
	})
	if kinds["flat"] != placeKindHome {
		t.Errorf("flat = %q, want home", kinds["flat"])
	}
	if kinds["office"] != placeKindWork {
		t.Errorf("office = %q, want work", kinds["office"])
	}
}

func TestSummarizePlaceUsageCountsANightThatCrossesMidnight(t *testing.T) {
	// Friday evening to Saturday morning: six night hours, no working hours.
	spans := []placeVisitSpan{{
		PlaceID:  "flat",
		Arrived:  mskTime(t, "2026-09-11 21:00"),
		Departed: mskTime(t, "2026-09-12 09:12"),
	}}

	usage := summarizePlaceUsage(spans, aiDisplayLocation)
	if len(usage) != 1 {
		t.Fatalf("got %d places", len(usage))
	}
	if got := usage[0].TotalHours; got < 12.1 || got > 12.3 {
		t.Errorf("total = %.2f h, want about 12.2", got)
	}
	// The whole 00:00-06:00 window falls inside the stay.
	if got := usage[0].NightHours; got < 5.9 || got > 6.1 {
		t.Errorf("night = %.2f h, want 6 - counting by arrival hour would give 0", got)
	}
}

func TestSummarizePlaceUsageCountsWorkdayHoursOnWeekdaysOnly(t *testing.T) {
	spans := []placeVisitSpan{
		// Friday at the office: 10:14-16:40 overlaps the 10-18 window fully.
		{PlaceID: "office", Arrived: mskTime(t, "2026-09-11 10:14"), Departed: mskTime(t, "2026-09-11 16:40")},
		// Saturday at the same address does not count as working hours.
		{PlaceID: "office", Arrived: mskTime(t, "2026-09-12 11:00"), Departed: mskTime(t, "2026-09-12 17:00")},
	}

	usage := summarizePlaceUsage(spans, aiDisplayLocation)
	if got := usage[0].WorkdayHours; got < 6.3 || got > 6.5 {
		t.Errorf("workday = %.2f h, want about 6.43 (the weekend visit must not count)", got)
	}
	if got := usage[0].TotalHours; got < 12.3 || got > 12.5 {
		t.Errorf("total = %.2f h, want about 12.43", got)
	}
}

func TestSummarizePlaceUsageIgnoresZeroLengthVisits(t *testing.T) {
	// A visit whose arrival was never observed is stored with equal timestamps.
	spans := []placeVisitSpan{{
		PlaceID:  "unknown",
		Arrived:  mskTime(t, "2026-09-11 19:02"),
		Departed: mskTime(t, "2026-09-11 19:02"),
	}}
	if usage := summarizePlaceUsage(spans, aiDisplayLocation); len(usage) != 0 {
		t.Fatalf("a zero length visit produced usage: %+v", usage)
	}
}
