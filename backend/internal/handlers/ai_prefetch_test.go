package handlers

import (
	"reflect"
	"testing"
)

func TestPlannedSyncSourcesMapsToolsToProviders(t *testing.T) {
	sources := plannedSyncSources([]aiToolCall{
		{Name: aiToolFinanceOverview},
		{Name: aiToolRecentTransactions},
		{Name: aiToolProductivityOverview},
	})

	// ZenMoney stands behind both finance tools and must be pulled once.
	want := []string{"todoist", "vikunja", "zenmoney"}
	if !reflect.DeepEqual(sources, want) {
		t.Fatalf("sources = %v, want %v", sources, want)
	}
}

func TestPlannedSyncSourcesSkipsWhatWeCannotPull(t *testing.T) {
	// Screen Time and Apple Health are pushed to us, the weather is not ours.
	sources := plannedSyncSources([]aiToolCall{
		{Name: aiToolScreenTimeOverview},
		{Name: aiToolWeatherOverview},
	})
	if len(sources) != 0 {
		t.Fatalf("sources = %v, want none", sources)
	}
}

func TestEveryMappedSourceIsASyncableConnector(t *testing.T) {
	// A typo here would be a silent no-op: the source would never match a
	// connector and the refresh would look like it ran.
	known := map[string]bool{
		"zenmoney": true, "todoist": true, "vikunja": true, "strava": true,
		"hevy": true, "habitify": true, "fatsecret": true, "notion": true,
		"google_calendar": true, "zepp": true, "xiaomi_scale": true,
	}
	for tool, sources := range aiToolSyncSources {
		for _, source := range sources {
			if !known[source] {
				t.Errorf("tool %s maps to unknown source %q", tool, source)
			}
		}
	}
}
