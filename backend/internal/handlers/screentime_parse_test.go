package handlers

import (
	"testing"
	"time"
)

// Structurally identical to a real "Get App & Website Activity" payload from an
// iPhone on iOS 26, with names and domains replaced. Keeps every edge case the
// real one had: interleaved apps and websites, an unresolved bundle identifier,
// a Cyrillic app name, and a non-breaking space inside a display name.
const screenTimeCombinedFixture = "Instagram (6h 55m)\n" +
	"Safari (1h 13m)\n" +
	"example-shop.com (47m)\n" +
	"sub.example.farm (18m)\n" +
	"Phone (11m)\n" +
	"Маркет (4m)\n" +
	"Yandex Maps (34s)\n" +
	"app.cdn.example.net (33s)\n" +
	"ru.example.mobilebanking.iphone (12s)\n" +
	"example.ru (5s)\n" +
	"Messages (1s)"

func findScreenTimeItem(t *testing.T, items []screenTimeItem, key string) screenTimeItem {
	t.Helper()
	for _, item := range items {
		if item.ItemKey == key {
			return item
		}
	}
	t.Fatalf("item %q not found in %d items", key, len(items))
	return screenTimeItem{}
}

func TestParseScreenTimeBlobCombined(t *testing.T) {
	result := parseScreenTimeBlob(screenTimeCombinedFixture, "")

	if len(result.Unparsed) != 0 {
		t.Fatalf("unexpected unparsed lines: %v", result.Unparsed)
	}
	if len(result.Items) != 11 {
		t.Fatalf("parsed %d items, want 11", len(result.Items))
	}

	tests := []struct {
		key         string
		wantKind    string
		wantSeconds int
		wantName    string
	}{
		{key: "instagram", wantKind: screenTimeKindApp, wantSeconds: 6*3600 + 55*60, wantName: "Instagram"},
		{key: "safari", wantKind: screenTimeKindApp, wantSeconds: 3600 + 13*60, wantName: "Safari"},
		{key: "example-shop.com", wantKind: screenTimeKindWebsite, wantSeconds: 47 * 60, wantName: "example-shop.com"},
		{key: "sub.example.farm", wantKind: screenTimeKindWebsite, wantSeconds: 18 * 60, wantName: "sub.example.farm"},
		{key: "маркет", wantKind: screenTimeKindApp, wantSeconds: 4 * 60, wantName: "Маркет"},
		{key: "app.cdn.example.net", wantKind: screenTimeKindWebsite, wantSeconds: 33, wantName: "app.cdn.example.net"},
		{key: "example.ru", wantKind: screenTimeKindWebsite, wantSeconds: 5, wantName: "example.ru"},
		{key: "messages", wantKind: screenTimeKindApp, wantSeconds: 1, wantName: "Messages"},
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			item := findScreenTimeItem(t, result.Items, tc.key)
			if item.Kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", item.Kind, tc.wantKind)
			}
			if item.Seconds != tc.wantSeconds {
				t.Errorf("seconds = %d, want %d", item.Seconds, tc.wantSeconds)
			}
			if item.DisplayName != tc.wantName {
				t.Errorf("display name = %q, want %q", item.DisplayName, tc.wantName)
			}
			if !item.KindInferred {
				t.Error("kind should be flagged as inferred for a combined payload")
			}
		})
	}
}

func TestParseScreenTimeBlobBundleIDStaysAnApp(t *testing.T) {
	// A reverse-DNS bundle identifier ends in a product name, not a public
	// suffix, so it must not be mistaken for a visited website.
	result := parseScreenTimeBlob(screenTimeCombinedFixture, "")
	item := findScreenTimeItem(t, result.Items, "ru.example.mobilebanking.iphone")
	if item.Kind != screenTimeKindApp {
		t.Fatalf("kind = %q, want %q", item.Kind, screenTimeKindApp)
	}
	if item.Seconds != 12 {
		t.Fatalf("seconds = %d, want 12", item.Seconds)
	}
}

func TestParseScreenTimeBlobNormalizesNonBreakingSpace(t *testing.T) {
	result := parseScreenTimeBlob(screenTimeCombinedFixture, "")
	item := findScreenTimeItem(t, result.Items, "yandex maps")
	if item.DisplayName != "Yandex Maps" {
		t.Fatalf("display name = %q, want %q", item.DisplayName, "Yandex Maps")
	}
	if item.Seconds != 34 {
		t.Fatalf("seconds = %d, want 34", item.Seconds)
	}
}

func TestParseScreenTimeBlobExplicitKindIsAuthoritative(t *testing.T) {
	// A hostname-looking entry from an apps-only field stays an app.
	result := parseScreenTimeBlob("example-shop.com (47m)", screenTimeKindApp)
	if len(result.Items) != 1 {
		t.Fatalf("parsed %d items, want 1", len(result.Items))
	}
	if result.Items[0].Kind != screenTimeKindApp {
		t.Fatalf("kind = %q, want %q", result.Items[0].Kind, screenTimeKindApp)
	}
	if result.Items[0].KindInferred {
		t.Error("kind must not be flagged as inferred when it was supplied")
	}
}

func TestParseScreenTimeBlobEdgeCases(t *testing.T) {
	t.Run("unparsable lines are reported", func(t *testing.T) {
		result := parseScreenTimeBlob("Instagram (6h)\nno duration here\nBroken (nonsense)\n\n", "")
		if len(result.Items) != 1 {
			t.Fatalf("parsed %d items, want 1", len(result.Items))
		}
		if len(result.Unparsed) != 2 {
			t.Fatalf("unparsed = %v, want 2 entries", result.Unparsed)
		}
	})

	t.Run("absurd durations are clamped not dropped", func(t *testing.T) {
		result := parseScreenTimeBlob("Runaway (100h 5m)", screenTimeKindWebsite)
		if len(result.Items) != 1 {
			t.Fatalf("parsed %d items, want 1", len(result.Items))
		}
		if result.Items[0].Seconds != screenTimeMaxSeconds {
			t.Fatalf("seconds = %d, want %d", result.Items[0].Seconds, screenTimeMaxSeconds)
		}
		if !result.Items[0].Clamped {
			t.Error("clamped flag not set")
		}
	})

	t.Run("duplicate names keep the larger duration", func(t *testing.T) {
		result := parseScreenTimeBlob("Instagram (5m)\nInstagram (2h)", screenTimeKindApp)
		if len(result.Items) != 1 {
			t.Fatalf("parsed %d items, want 1", len(result.Items))
		}
		if result.Items[0].Seconds != 2*3600 {
			t.Fatalf("seconds = %d, want %d", result.Items[0].Seconds, 2*3600)
		}
	})

	t.Run("empty blob yields nothing", func(t *testing.T) {
		result := parseScreenTimeBlob("   \n\n", "")
		if len(result.Items) != 0 || len(result.Unparsed) != 0 {
			t.Fatalf("items = %v, unparsed = %v, want both empty", result.Items, result.Unparsed)
		}
	})
}

func TestParseScreenTimeDuration(t *testing.T) {
	tests := []struct {
		value string
		want  int
		ok    bool
	}{
		{value: "6h 55m", want: 6*3600 + 55*60, ok: true},
		{value: "1h 13m", want: 3600 + 13*60, ok: true},
		{value: "47m", want: 47 * 60, ok: true},
		{value: "52s", want: 52, ok: true},
		{value: "1s", want: 1, ok: true},
		{value: "2h", want: 7200, ok: true},
		{value: "1h 2m 3s", want: 3723, ok: true},
		{value: "1 hr 30 min", want: 5400, ok: true},
		{value: "2 ч 5 мин", want: 2*3600 + 5*60, ok: true},
		{value: "", ok: false},
		{value: "nonsense", ok: false},
		{value: "12", ok: false},
		{value: "5 parsecs", ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			got, ok := parseScreenTimeDuration(tc.value)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("seconds = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestLooksLikeHostname(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "example.com", want: true},
		{name: "account.example.com", want: true},
		{name: "example.ru", want: true},
		{name: "sub.example.farm", want: true},
		{name: "app.cdn.example.net", want: true},
		{name: "ru.example.mobilebanking.iphone", want: false},
		{name: "Instagram", want: false},
		{name: "App Store", want: false},
		{name: "Yandex Mail", want: false},
		{name: "InCallService", want: false},
		{name: "Маркет", want: false},
		{name: "trailing.", want: false},
		{name: ".leading", want: false},
		{name: "https://example.com", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeHostname(tc.name); got != tc.want {
				t.Fatalf("looksLikeHostname(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestCollectScreenTimeItems(t *testing.T) {
	t.Run("screen_time is combined when websites field is absent", func(t *testing.T) {
		items, unparsed := collectScreenTimeItems(screenTimeEnvelope{ScreenTime: screenTimeCombinedFixture})
		if len(unparsed) != 0 {
			t.Fatalf("unparsed = %v", unparsed)
		}
		website := findScreenTimeItem(t, items, "example-shop.com")
		if website.Kind != screenTimeKindWebsite || !website.KindInferred {
			t.Fatalf("kind = %q inferred = %v, want website/true", website.Kind, website.KindInferred)
		}
	})

	t.Run("screen_time is apps-only when websites field is present", func(t *testing.T) {
		items, _ := collectScreenTimeItems(screenTimeEnvelope{
			ScreenTime: "Instagram (6h 55m)\nexample-shop.com (47m)",
			Websites:   "other.example.com (3m)",
		})
		if len(items) != 3 {
			t.Fatalf("parsed %d items, want 3", len(items))
		}
		misfiled := findScreenTimeItem(t, items, "example-shop.com")
		if misfiled.Kind != screenTimeKindApp || misfiled.KindInferred {
			t.Fatalf("kind = %q inferred = %v, want app/false", misfiled.Kind, misfiled.KindInferred)
		}
		site := findScreenTimeItem(t, items, "other.example.com")
		if site.Kind != screenTimeKindWebsite || site.KindInferred {
			t.Fatalf("kind = %q inferred = %v, want website/false", site.Kind, site.KindInferred)
		}
	})

	t.Run("apps and websites fields are both read", func(t *testing.T) {
		items, _ := collectScreenTimeItems(screenTimeEnvelope{
			Apps:     "Instagram (6h 55m)",
			Websites: "example.com (3m)",
		})
		if len(items) != 2 {
			t.Fatalf("parsed %d items, want 2", len(items))
		}
	})
}

func TestSummarizeScreenTimeItems(t *testing.T) {
	items, _ := collectScreenTimeItems(screenTimeEnvelope{ScreenTime: screenTimeCombinedFixture})
	summary := summarizeScreenTimeItems(items)

	if summary.appCount != 7 {
		t.Errorf("appCount = %d, want 7", summary.appCount)
	}
	if summary.websiteCount != 4 {
		t.Errorf("websiteCount = %d, want 4", summary.websiteCount)
	}
	// Websites are a subset of browser time, so they are summed separately and
	// must not appear in appSeconds.
	wantApps := 6*3600 + 55*60 + 3600 + 13*60 + 11*60 + 4*60 + 34 + 12 + 1
	if summary.appSeconds != wantApps {
		t.Errorf("appSeconds = %d, want %d", summary.appSeconds, wantApps)
	}
	wantSites := 47*60 + 18*60 + 33 + 5
	if summary.websiteSeconds != wantSites {
		t.Errorf("websiteSeconds = %d, want %d", summary.websiteSeconds, wantSites)
	}
	if summary.clamped {
		t.Error("clamped should be false for a sane payload")
	}
}

func TestSummarizeScreenTimeItemsDetectsAggregateWindow(t *testing.T) {
	// What a Shortcut asking for thisMonth instead of specifiedDay produces: a
	// per-item value that is individually plausible but a total no single day can
	// reach. The handler refuses these, so the summary has to surface it.
	items, _ := collectScreenTimeItems(screenTimeEnvelope{
		Apps: "Instagram (20h)\nSafari (18h)\nTelegram (9h)",
	})
	summary := summarizeScreenTimeItems(items)

	if summary.appSeconds <= screenTimeMaxSeconds {
		t.Fatalf("appSeconds = %d, want more than %d", summary.appSeconds, screenTimeMaxSeconds)
	}
	if summary.clamped {
		t.Error("no single item exceeded 24h, so clamped should be false")
	}
}

func TestResolveScreenTimeDay(t *testing.T) {
	now := time.Now().In(aiDisplayLocation)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, aiDisplayLocation)
	yesterday := today.AddDate(0, 0, -1)

	tests := []struct {
		name          string
		envelope      screenTimeEnvelope
		want          time.Time
		wantIsPartial bool
	}{
		{name: "default is yesterday", envelope: screenTimeEnvelope{}, want: yesterday},
		{name: "during=yesterday", envelope: screenTimeEnvelope{During: "yesterday"}, want: yesterday},
		{name: "during=today marks partial", envelope: screenTimeEnvelope{During: "Today"}, want: today, wantIsPartial: true},
		{
			name:     "explicit day wins over during",
			envelope: screenTimeEnvelope{Day: "2026-07-14", During: "today"},
			want:     time.Date(2026, 7, 14, 0, 0, 0, 0, aiDisplayLocation),
		},
		{
			name:          "explicit day equal to today is partial",
			envelope:      screenTimeEnvelope{Day: today.Format("2006-01-02")},
			want:          today,
			wantIsPartial: true,
		},
		{name: "unparsable day falls back", envelope: screenTimeEnvelope{Day: "not a date"}, want: yesterday},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			day, isPartial := resolveScreenTimeDay(tc.envelope)
			if !day.Equal(tc.want) {
				t.Errorf("day = %s, want %s", day.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
			if isPartial != tc.wantIsPartial {
				t.Errorf("isPartial = %v, want %v", isPartial, tc.wantIsPartial)
			}
		})
	}
}
