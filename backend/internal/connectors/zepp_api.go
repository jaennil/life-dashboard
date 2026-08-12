package connectors

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// The Zepp cloud API is undocumented. Everything here was verified against a live
// account: the endpoints, the header set they require, and the field names below.
//
// The header set is not optional. With only an apptoken the API answers 500
// {"code":-50000} for every request, which reads like an auth failure but is not:
// it is the absence of the client identification headers that Zepp expects.
const (
	zeppAPIHost = "https://api-mifit.zepp.com"
	// Client identification, taken from the Zepp Android app build these headers
	// were captured from. Bumping the app version means bumping all four together.
	zeppAppName    = "com.huami.midong"
	zeppAppVersion = "9.12.5"
	zeppClientVer  = "151689_9.12.5"
	zeppBuildVer   = "202509151347"
	zeppChannel    = "a100900101016"
	zeppUserAgent  = "Zepp/9.12.5 (Pixel 4; Android 12; Density/2.75)"
)

// Sleep stage codes inside the base64 summary blob.
const (
	zeppStageLight = 4
	zeppStageDeep  = 5
	zeppStageAwake = 7
	zeppStageREM   = 8
)

// Per-minute heart rate samples use 254 as "no reading". Anything at or above the
// cutoff is treated as absent rather than as an implausible pulse.
const zeppHeartRateAbsent = 250

type zeppBandDataResponse struct {
	Data []zeppBandDay `json:"data"`
}

type zeppBandDay struct {
	DateTime string `json:"date_time"`
	// Summary is base64-encoded JSON, not JSON.
	Summary string `json:"summary"`
	// DataHR is base64 of 1440 bytes: one heart rate sample per minute of the day.
	DataHR string `json:"data_hr"`
}

type zeppSummary struct {
	Steps  *zeppSteps `json:"stp"`
	Sleep  *zeppSleep `json:"slp"`
	Heart  *zeppHeart `json:"hr"`
	Goal   int        `json:"goal"`
	Serial string     `json:"sn"`
}

type zeppSteps struct {
	Total       int `json:"ttl"`
	Calories    int `json:"cal"`
	Distance    int `json:"dis"`
	RunDistance int `json:"runDist"`
	RunCalories int `json:"runCal"`
}

type zeppHeart struct {
	MaxHR *struct {
		HR        int   `json:"hr"`
		Timestamp int64 `json:"ts"`
	} `json:"maxHr"`
}

// zeppSleep is the summary block. The field names were mapped by comparing three
// nights against both the stage spans and what the Zepp app itself displays:
//
//	dp + lt + dt matched ed - st to within a minute, and matched the app's total,
//	while the spans came out 3-6% short because merging loses minutes. The summary
//	is therefore the authority and the spans are only a fallback.
//
//	dt  is REM ("dream time"), not daytime sleep: 84/142/87 against 82/131/79 REM
//	    minutes derived from mode-8 spans.
//	wk  is awake minutes: 17/6/1 against 16/5/0 from mode-7 spans.
//	ss  is the sleep score the app shows.
type zeppSleep struct {
	DeepMinutes  int             `json:"dp"`
	LightMinutes int             `json:"lt"`
	REMMinutes   int             `json:"dt"`
	AwakeMinutes int             `json:"wk"`
	SleepScore   int             `json:"ss"`
	RestingHR    int             `json:"rhr"`
	Start        int64           `json:"st"`
	End          int64           `json:"ed"`
	Stages       []zeppSleepSpan `json:"stage"`
}

// zeppSleepSpan carries minutes-from-midnight offsets, not timestamps.
type zeppSleepSpan struct {
	Mode  int `json:"mode"`
	Start int `json:"start"`
	Stop  int `json:"stop"`
}

type zeppEventsResponse struct {
	Items []json.RawMessage `json:"items"`
}

type zeppStressItem struct {
	Timestamp int64 `json:"timestamp"`
	Min       int   `json:"minStress"`
	Max       int   `json:"maxStress"`
	Avg       int   `json:"avgStress"`
	Relax     int   `json:"relaxProportion"`
	Normal    int   `json:"normalProportion"`
	Medium    int   `json:"mediumProportion"`
	High      int   `json:"highProportion"`
}

type zeppPAIItem struct {
	Timestamp int64   `json:"timestamp"`
	TotalPAI  float64 `json:"totalPai"`
	DailyPAI  float64 `json:"dailyPai"`
	MaxHR     int     `json:"maxHr"`
	RestHR    int     `json:"restHr"`
	LowZone   float64 `json:"lowZonePai"`
	MedZone   float64 `json:"mediumZonePai"`
	HighZone  float64 `json:"highZonePai"`
}

type zeppOxygenItem struct {
	Timestamp int64  `json:"timestamp"`
	SubType   string `json:"subType"`
	// Extra is a JSON document embedded as a string.
	Extra string `json:"extra"`
}

// zeppHeaders builds the client identification the API insists on. x-request-id
// is supplied per call by the caller so each request is individually traceable.
func zeppHeaders(appToken, requestID, timezone, country string) map[string]string {
	return map[string]string{
		"apptoken":               appToken,
		"appname":                zeppAppName,
		"appplatform":            "android_phone",
		"channel":                zeppChannel,
		"country":                country,
		"cv":                     zeppClientVer,
		"hm-privacy-ceip":        "true",
		"hm-privacy-diagnostics": "false",
		"lang":                   "en_US",
		"timezone":               timezone,
		"user-agent":             zeppUserAgent,
		"v":                      "2.0",
		"vb":                     zeppBuildVer,
		"vn":                     zeppAppVersion,
		"x-request-id":           requestID,
	}
}

// decodeZeppSummary unwraps the base64-encoded JSON and returns both the parsed
// view and the raw document, so callers can archive what they did not model.
func decodeZeppSummary(encoded string) (zeppSummary, []byte, error) {
	var summary zeppSummary
	if encoded == "" {
		return summary, nil, fmt.Errorf("empty summary")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return summary, nil, fmt.Errorf("base64: %w", err)
	}
	if err := json.Unmarshal(decoded, &summary); err != nil {
		return summary, decoded, fmt.Errorf("json: %w", err)
	}
	return summary, decoded, nil
}

type zeppHeartRateSample struct {
	At  time.Time
	BPM int
}

// decodeZeppHeartRate expands the per-minute blob into samples. The blob holds one
// byte per minute starting at midnight local time; 0 and values at or above the
// absent cutoff mean the watch recorded nothing for that minute.
func decodeZeppHeartRate(encoded string, dayStart time.Time) ([]zeppHeartRateSample, error) {
	if encoded == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64: %w", err)
	}

	samples := make([]zeppHeartRateSample, 0, 256)
	for minute, value := range raw {
		if value == 0 || value >= zeppHeartRateAbsent {
			continue
		}
		samples = append(samples, zeppHeartRateSample{
			At:  dayStart.Add(time.Duration(minute) * time.Minute),
			BPM: int(value),
		})
	}
	return samples, nil
}

// zeppSleepStageName maps a stage code onto the vocabulary sleep_stages uses.
func zeppSleepStageName(mode int) string {
	switch mode {
	case zeppStageLight:
		return "light"
	case zeppStageDeep:
		return "deep"
	case zeppStageAwake:
		return "awake"
	case zeppStageREM:
		return "rem"
	default:
		return ""
	}
}

// zeppSpanTime converts a minutes-from-midnight offset into a timestamp. Spans
// that begin in the evening are reported as offsets past 24h, so values beyond a
// day roll forward rather than wrapping.
func zeppSpanTime(day time.Time, minute int) time.Time {
	return day.Add(time.Duration(minute) * time.Minute)
}

type zeppSleepInterval struct {
	Stage   string
	Start   time.Time
	End     time.Time
	Minutes int
}

// zeppSleepSpans converts the summary's stage spans into concrete intervals,
// dropping any span with an unknown code or a non-positive duration.
//
// The offsets are minutes from some midnight Zepp does not name, and it is not
// the midnight of the labelled date: observed spans sat 27 hours (a day plus the
// local UTC offset) past the session's real start. Rather than reverse-engineer
// that convention, the earliest offset is pinned to slp.st, which arrives as a
// true epoch, so the base is derived instead of assumed.
func zeppSleepSpans(day time.Time, sleep *zeppSleep) []zeppSleepInterval {
	if sleep == nil {
		return nil
	}

	base := day
	if sleep.Start > 0 {
		if earliest, ok := zeppEarliestSpanOffset(sleep.Stages); ok {
			base = zeppUnixTime(sleep.Start).Add(-time.Duration(earliest) * time.Minute)
		}
	}

	intervals := make([]zeppSleepInterval, 0, len(sleep.Stages))
	for _, span := range sleep.Stages {
		stage := zeppSleepStageName(span.Mode)
		if stage == "" || span.Stop <= span.Start {
			continue
		}
		intervals = append(intervals, zeppSleepInterval{
			Stage:   stage,
			Start:   zeppSpanTime(base, span.Start),
			End:     zeppSpanTime(base, span.Stop),
			Minutes: span.Stop - span.Start,
		})
	}
	return intervals
}

// zeppEarliestSpanOffset returns the smallest usable span offset, which is the
// one that lines up with the reported sleep start.
func zeppEarliestSpanOffset(spans []zeppSleepSpan) (int, bool) {
	earliest := 0
	found := false
	for _, span := range spans {
		if zeppSleepStageName(span.Mode) == "" || span.Stop <= span.Start {
			continue
		}
		if !found || span.Start < earliest {
			earliest = span.Start
			found = true
		}
	}
	return earliest, found
}

// zeppSleepTotals sums the stage spans. The summary's own dp/lt fields only cover
// deep and light, so REM and awake minutes have to come from the spans.
func zeppSleepTotals(intervals []zeppSleepInterval) map[string]int {
	totals := map[string]int{}
	for _, interval := range intervals {
		totals[interval.Stage] += interval.Minutes
	}
	return totals
}

// zeppUnixTime converts the API's epoch milliseconds into a time.Time, treating
// values that are plainly seconds as seconds.
func zeppUnixTime(value int64) time.Time {
	if value > 1e12 {
		return time.UnixMilli(value)
	}
	return time.Unix(value, 0)
}

// zeppOxygenValue pulls the SpO2 reading out of the embedded extra document.
func zeppOxygenValue(item zeppOxygenItem) (float64, bool) {
	if item.Extra == "" {
		return 0, false
	}
	var extra struct {
		SpO2 json.Number `json:"spo2"`
	}
	if err := json.Unmarshal([]byte(item.Extra), &extra); err != nil {
		return 0, false
	}
	value, err := extra.SpO2.Float64()
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}
