package connectors

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func zeppSummaryFixture(t *testing.T, summary string) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString([]byte(summary))
}

func TestDecodeZeppSummary(t *testing.T) {
	t.Run("real shape", func(t *testing.T) {
		encoded := zeppSummaryFixture(t, `{"stp":{"ttl":8421,"cal":312,"dis":6100},
			"slp":{"dp":62,"lt":274,"st":1786000000,"ed":1786025000,
			"stage":[{"mode":4,"start":1380,"stop":1440},{"mode":5,"start":1440,"stop":1502},
			         {"mode":8,"start":1502,"stop":1560},{"mode":7,"start":1560,"stop":1565}]},
			"goal":8000,"sn":"ABC123"}`)

		summary, err := decodeZeppSummary(encoded)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if summary.Steps == nil || summary.Steps.Total != 8421 || summary.Steps.Distance != 6100 || summary.Steps.Calories != 312 {
			t.Fatalf("steps = %+v", summary.Steps)
		}
		if summary.Sleep == nil || summary.Sleep.DeepMinutes != 62 || summary.Sleep.LightMinutes != 274 {
			t.Fatalf("sleep = %+v", summary.Sleep)
		}
		if len(summary.Sleep.Stages) != 4 {
			t.Fatalf("stages = %d, want 4", len(summary.Sleep.Stages))
		}
		if summary.Goal != 8000 || summary.Serial != "ABC123" {
			t.Fatalf("goal = %d serial = %q", summary.Goal, summary.Serial)
		}
	})

	t.Run("rejects garbage", func(t *testing.T) {
		for _, bad := range []string{"", "not base64 !!!", zeppSummaryFixture(t, "not json")} {
			if _, err := decodeZeppSummary(bad); err == nil {
				t.Errorf("decodeZeppSummary(%q) returned no error", bad)
			}
		}
	})

	t.Run("a day with no sleep leaves slp nil", func(t *testing.T) {
		summary, err := decodeZeppSummary(zeppSummaryFixture(t, `{"stp":{"ttl":100,"cal":4,"dis":80}}`))
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if summary.Sleep != nil {
			t.Fatalf("sleep = %+v, want nil", summary.Sleep)
		}
	})
}

func TestZeppSleepStageName(t *testing.T) {
	tests := map[int]string{
		zeppStageLight: "light",
		zeppStageDeep:  "deep",
		zeppStageAwake: "awake",
		zeppStageREM:   "rem",
		0:              "",
		99:             "",
	}
	for mode, want := range tests {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			if got := zeppSleepStageName(mode); got != want {
				t.Fatalf("zeppSleepStageName(%d) = %q, want %q", mode, got, want)
			}
		})
	}
}

func TestZeppSleepSpans(t *testing.T) {
	day := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	sleep := &zeppSleep{
		Stages: []zeppSleepSpan{
			{Mode: zeppStageLight, Start: 1380, Stop: 1440}, // 23:00 -> 24:00
			{Mode: zeppStageDeep, Start: 1440, Stop: 1502},  // rolls into the next day
			{Mode: zeppStageREM, Start: 1502, Stop: 1560},
			{Mode: 99, Start: 0, Stop: 30},                // unknown code, dropped
			{Mode: zeppStageAwake, Start: 100, Stop: 100}, // zero length, dropped
			{Mode: zeppStageAwake, Start: 200, Stop: 150}, // negative, dropped
		},
	}

	intervals := zeppSleepSpans(day, sleep)
	if len(intervals) != 3 {
		t.Fatalf("intervals = %d, want 3: %+v", len(intervals), intervals)
	}

	// Offsets past 1440 must roll forward into the next day, not wrap to its start.
	if got := intervals[0].Start; !got.Equal(day.Add(23 * time.Hour)) {
		t.Errorf("first span start = %s, want 23:00", got.Format(time.RFC3339))
	}
	if got := intervals[1].Start; !got.Equal(day.AddDate(0, 0, 1)) {
		t.Errorf("second span start = %s, want next midnight", got.Format(time.RFC3339))
	}
	if intervals[0].Minutes != 60 || intervals[1].Minutes != 62 || intervals[2].Minutes != 58 {
		t.Errorf("minutes = %d/%d/%d, want 60/62/58", intervals[0].Minutes, intervals[1].Minutes, intervals[2].Minutes)
	}

	totals := zeppSleepTotals(intervals)
	if totals["light"] != 60 || totals["deep"] != 62 || totals["rem"] != 58 {
		t.Errorf("totals = %v", totals)
	}
	if _, ok := totals["awake"]; ok {
		t.Errorf("totals should not contain awake: %v", totals)
	}
}

func TestZeppSleepSpansAnchorToReportedStart(t *testing.T) {
	// Real numbers from the account: the night labelled 2026-08-10 started at
	// 03:08 Moscow time, and its first span offset was 1628 minutes. Treating the
	// offsets as minutes past the labelled date put every span 27 hours late.
	start := time.Date(2026, 8, 10, 0, 8, 0, 0, time.UTC) // 03:08 MSK
	sleep := &zeppSleep{
		Start: start.Unix(),
		Stages: []zeppSleepSpan{
			{Mode: zeppStageLight, Start: 1628, Stop: 1642},
			{Mode: zeppStageDeep, Start: 1642, Stop: 1655},
			{Mode: zeppStageAwake, Start: 1695, Stop: 1712},
		},
	}

	// The labelled day is deliberately wrong-footed: it must not influence the
	// result once slp.st is available.
	intervals := zeppSleepSpans(time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC), sleep)
	if len(intervals) != 3 {
		t.Fatalf("intervals = %d, want 3", len(intervals))
	}
	if !intervals[0].Start.Equal(start) {
		t.Fatalf("first span starts %s, want the reported sleep start %s",
			intervals[0].Start.Format(time.RFC3339), start.Format(time.RFC3339))
	}
	if got := intervals[1].Start.Sub(start); got != 14*time.Minute {
		t.Errorf("second span offset = %s, want 14m after the start", got)
	}
	if got := intervals[2].End.Sub(start); got != 84*time.Minute {
		t.Errorf("last span ends %s after the start, want 84m", got)
	}
}

func TestZeppSleepSpansFallBackToDayWithoutStart(t *testing.T) {
	day := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	sleep := &zeppSleep{Stages: []zeppSleepSpan{{Mode: zeppStageLight, Start: 60, Stop: 90}}}

	intervals := zeppSleepSpans(day, sleep)
	if len(intervals) != 1 {
		t.Fatalf("intervals = %d, want 1", len(intervals))
	}
	if !intervals[0].Start.Equal(day.Add(time.Hour)) {
		t.Fatalf("start = %s, want the day plus the raw offset", intervals[0].Start.Format(time.RFC3339))
	}
}

func TestZeppEarliestSpanOffset(t *testing.T) {
	_, ok := zeppEarliestSpanOffset(nil)
	if ok {
		t.Error("no spans should report no offset")
	}

	// Unknown codes and zero-length spans must not win the minimum.
	spans := []zeppSleepSpan{
		{Mode: 99, Start: 10, Stop: 20},
		{Mode: zeppStageAwake, Start: 30, Stop: 30},
		{Mode: zeppStageDeep, Start: 100, Stop: 140},
		{Mode: zeppStageLight, Start: 80, Stop: 100},
	}
	got, ok := zeppEarliestSpanOffset(spans)
	if !ok || got != 80 {
		t.Fatalf("earliest = %d (ok=%v), want 80", got, ok)
	}
}

func TestZeppSleepSpansNilSafe(t *testing.T) {
	if got := zeppSleepSpans(time.Now(), nil); got != nil {
		t.Fatalf("zeppSleepSpans(nil) = %v, want nil", got)
	}
}

func TestZeppUnixTime(t *testing.T) {
	want := time.Unix(1786000000, 0)
	if got := zeppUnixTime(1786000000); !got.Equal(want) {
		t.Errorf("seconds: got %s, want %s", got, want)
	}
	if got := zeppUnixTime(1786000000000); !got.Equal(want) {
		t.Errorf("milliseconds: got %s, want %s", got, want)
	}
}

func TestZeppOxygenValue(t *testing.T) {
	tests := []struct {
		name  string
		item  zeppOxygenItem
		want  float64
		valid bool
	}{
		{name: "click reading", item: zeppOxygenItem{Extra: `{"spo2":97}`}, want: 97, valid: true},
		{name: "fractional", item: zeppOxygenItem{Extra: `{"spo2":"96.5"}`}, want: 96.5, valid: true},
		{name: "empty extra", item: zeppOxygenItem{}, valid: false},
		{name: "not json", item: zeppOxygenItem{Extra: `nope`}, valid: false},
		{name: "zero is not a reading", item: zeppOxygenItem{Extra: `{"spo2":0}`}, valid: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := zeppOxygenValue(tc.item)
			if ok != tc.valid {
				t.Fatalf("ok = %v, want %v", ok, tc.valid)
			}
			if ok && got != tc.want {
				t.Fatalf("value = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestZeppStressAndPAIDecode(t *testing.T) {
	var stress zeppStressItem
	body := `{"timestamp":1786000000000,"minStress":18,"maxStress":81,"avgStress":37,
		"relaxProportion":41,"normalProportion":38,"mediumProportion":16,"highProportion":5}`
	if err := json.Unmarshal([]byte(body), &stress); err != nil {
		t.Fatalf("stress decode: %v", err)
	}
	if stress.Avg != 37 || stress.High != 5 || stress.Timestamp == 0 {
		t.Fatalf("stress = %+v", stress)
	}

	var pai zeppPAIItem
	paiBody := `{"timestamp":1786000000000,"totalPai":118.4,"dailyPai":12.7,"maxHr":168,"restHr":54,
		"lowZonePai":6.1,"mediumZonePai":4.4,"highZonePai":2.2}`
	if err := json.Unmarshal([]byte(paiBody), &pai); err != nil {
		t.Fatalf("pai decode: %v", err)
	}
	if pai.TotalPAI != 118.4 || pai.RestHR != 54 || pai.HighZone != 2.2 {
		t.Fatalf("pai = %+v", pai)
	}
}

func TestIsZeppAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "401 triggers relogin", err: &zeppHTTPError{status: http.StatusUnauthorized}, want: true},
		{name: "403 triggers relogin", err: &zeppHTTPError{status: http.StatusForbidden}, want: true},
		{name: "429 does not", err: &zeppHTTPError{status: http.StatusTooManyRequests}, want: false},
		{name: "500 does not", err: &zeppHTTPError{status: http.StatusInternalServerError}, want: false},
		{name: "wrapped 401 still counts", err: fmt.Errorf("band data: %w", &zeppHTTPError{status: 401}), want: true},
		{name: "plain error", err: fmt.Errorf("boom"), want: false},
		{name: "nil", err: nil, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isZeppAuthError(tc.err); got != tc.want {
				t.Fatalf("isZeppAuthError = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDecodeZeppHeartRate(t *testing.T) {
	day := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)

	t.Run("one byte per minute", func(t *testing.T) {
		// Verified against a live response: 1440 bytes for the day, 254 marking the
		// minutes the watch recorded nothing.
		raw := make([]byte, 1440)
		for i := range raw {
			raw[i] = 254
		}
		raw[0] = 67    // 00:00
		raw[61] = 133  // 01:01
		raw[1439] = 80 // 23:59
		raw[500] = 0   // zero also means absent

		samples, err := decodeZeppHeartRate(base64.StdEncoding.EncodeToString(raw), day)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if len(samples) != 3 {
			t.Fatalf("samples = %d, want 3: %+v", len(samples), samples)
		}
		if samples[0].BPM != 67 || !samples[0].At.Equal(day) {
			t.Errorf("first = %d bpm at %s", samples[0].BPM, samples[0].At.Format(time.RFC3339))
		}
		if samples[1].BPM != 133 || !samples[1].At.Equal(day.Add(61*time.Minute)) {
			t.Errorf("second = %d bpm at %s", samples[1].BPM, samples[1].At.Format(time.RFC3339))
		}
		if !samples[2].At.Equal(day.Add(1439 * time.Minute)) {
			t.Errorf("last at %s, want 23:59", samples[2].At.Format(time.RFC3339))
		}
	})

	t.Run("empty blob is not an error", func(t *testing.T) {
		samples, err := decodeZeppHeartRate("", day)
		if err != nil || samples != nil {
			t.Fatalf("samples = %v, err = %v", samples, err)
		}
	})

	t.Run("garbage base64 is an error", func(t *testing.T) {
		if _, err := decodeZeppHeartRate("!!!not base64!!!", day); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestZeppSummaryExtraFields(t *testing.T) {
	// Fields discovered on the live account that the reference implementation does
	// not mention: hr.maxHr, slp.rhr and the running breakdown inside stp.
	encoded := zeppSummaryFixture(t, `{"stp":{"ttl":8421,"cal":312,"dis":6100,"runDist":1200,"runCal":95},
		"slp":{"dp":62,"lt":274,"rhr":54,"st":1786000000,"ed":1786025000},
		"hr":{"maxHr":{"hr":124,"ts":1786305257}},"sn":"ABC123"}`)

	summary, err := decodeZeppSummary(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if summary.Heart == nil || summary.Heart.MaxHR == nil || summary.Heart.MaxHR.HR != 124 {
		t.Errorf("max hr = %+v, want 124", summary.Heart)
	}
	if summary.Sleep == nil || summary.Sleep.RestingHR != 54 {
		t.Errorf("resting hr = %+v, want 54", summary.Sleep)
	}
	if summary.Steps == nil || summary.Steps.RunDistance != 1200 || summary.Steps.RunCalories != 95 {
		t.Errorf("running = %+v", summary.Steps)
	}
}

func TestZeppHeadersCarryClientIdentity(t *testing.T) {
	// Without these the API answers 500 for every request, so their presence is
	// worth asserting rather than trusting.
	h := zeppHeaders("tok", "req-1", "Europe/Moscow", "RU")
	for _, name := range []string{
		"apptoken", "appname", "appplatform", "channel", "country", "cv",
		"hm-privacy-ceip", "hm-privacy-diagnostics", "lang", "timezone",
		"user-agent", "v", "vb", "vn", "x-request-id",
	} {
		if h[name] == "" {
			t.Errorf("header %q missing", name)
		}
	}
	if h["apptoken"] != "tok" || h["x-request-id"] != "req-1" {
		t.Errorf("per-call values not threaded through: %v", h)
	}
}

func TestNewZeppRequestIDIsUniqueHex(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id := newZeppRequestID()
		if len(id) != 32 {
			t.Fatalf("id = %q, want 32 hex chars", id)
		}
		if seen[id] {
			t.Fatalf("duplicate request id %q", id)
		}
		seen[id] = true
	}
}

func TestZeppHTTPErrorTruncatesBody(t *testing.T) {
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'x'
	}
	err := &zeppHTTPError{status: 500, body: string(long)}
	if len(err.Error()) > 260 {
		t.Fatalf("error message not truncated: %d chars", len(err.Error()))
	}
}

func TestFirstPositive(t *testing.T) {
	if got := firstPositive(0, 0, 42, 7); got != 42 {
		t.Fatalf("firstPositive = %d, want 42", got)
	}
	if got := firstPositive(0, 0); got != 0 {
		t.Fatalf("firstPositive = %d, want 0", got)
	}
}
