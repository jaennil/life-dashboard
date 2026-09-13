package handlers

import (
	"reflect"
	"slices"
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
		"google_calendar": true, "zepp": true, "xiaomi_scale": true, "home_assistant": true,
	}
	for tool, sources := range aiToolSyncSources {
		for _, source := range sources {
			if !known[source] {
				t.Errorf("tool %s maps to unknown source %q", tool, source)
			}
		}
	}
}

func TestCheckupPrefetchCoversEverySyncableSource(t *testing.T) {
	// A report reads every section, so it refreshes every provider rather than
	// the two or three a single question would have planned.
	tools := checkupPrefetchTools()
	if len(tools) != len(aiToolSyncSources) {
		t.Fatalf("checkup prefetches %d tools, want %d", len(tools), len(aiToolSyncSources))
	}

	sources := plannedSyncSources(tools)
	for _, want := range []string{"zenmoney", "hevy", "fatsecret", "vikunja", "todoist", "zepp"} {
		if !slices.Contains(sources, want) {
			t.Errorf("a checkup would not refresh %s: %v", want, sources)
		}
	}
}

func TestPrefetchPaceGivesAReportMoreRoomThanAQuestion(t *testing.T) {
	// Someone is waiting for an answer; nobody is waiting for a report, and the
	// slowest provider takes over a minute.
	if checkupPrefetchPace.total <= answerPrefetchPace.total {
		t.Errorf("checkup total %s is not more patient than an answer's %s",
			checkupPrefetchPace.total, answerPrefetchPace.total)
	}
	if checkupPrefetchPace.perSource > checkupPrefetchPace.total {
		t.Errorf("a single source may outlast the whole refresh: %s > %s",
			checkupPrefetchPace.perSource, checkupPrefetchPace.total)
	}
	if answerPrefetchPace.perSource > answerPrefetchPace.total {
		t.Errorf("a single source may outlast the whole refresh: %s > %s",
			answerPrefetchPace.perSource, answerPrefetchPace.total)
	}
}
