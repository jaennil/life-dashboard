package handlers

import "testing"

func TestToolSourcesDisabled(t *testing.T) {
	disabled := map[string]bool{"google_calendar": true, "todoist": true, "zepp": true}

	cases := []struct {
		name aiToolName
		want bool
	}{
		{aiToolCalendarOverview, true},
		// Vikunja is still on, so tasks are still read.
		{aiToolProductivityOverview, false},
		// Apple Health and the scale still feed the health section.
		{aiToolHealthOverview, false},
		// Tools without a source of their own are never skipped.
		{aiToolWeatherOverview, false},
		{aiToolDailySeries, false},
	}
	for _, tc := range cases {
		if got := toolSourcesDisabled(tc.name, disabled); got != tc.want {
			t.Errorf("toolSourcesDisabled(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}

	all := map[string]bool{"zepp": true, "xiaomi_scale": true, "home_assistant": true, "apple_health": true}
	if !toolSourcesDisabled(aiToolHealthOverview, all) {
		t.Error("health with every source off must be skipped")
	}
}
