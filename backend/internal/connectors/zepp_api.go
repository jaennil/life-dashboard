package connectors

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// The Zepp cloud API is not documented by Amazfit; the shapes below come from the
// community reverse engineering that zepp_to_influxdb and hacking-mifit-api rely
// on. Treat every field as best effort and never fail a whole sync over one.
const (
	zeppAuthURLTemplate      = "https://api-user.huami.com/registrations/%s/tokens"
	zeppLoginURL             = "https://account.huami.com/v2/client/login"
	zeppBandDataURL          = "https://api-mifit.huami.com/v1/data/band_data.json"
	zeppEventsURLTemplate    = "https://api-mifit.zepp.com/users/%s/events"
	zeppPAIEventsURLTemplate = "https://api-mifit-de2.zepp.com/users/%s/events"
)

// Sleep stage codes as emitted inside the base64 summary blob.
const (
	zeppStageLight = 4
	zeppStageDeep  = 5
	zeppStageAwake = 7
	zeppStageREM   = 8
)

type zeppBandDataResponse struct {
	Data []zeppBandDay `json:"data"`
}

type zeppBandDay struct {
	DateTime string `json:"date_time"`
	// Summary is base64-encoded JSON, not JSON.
	Summary string `json:"summary"`
	// DataHR is a base64 blob of per-minute samples whose encoding is still
	// unconfirmed, so it is only measured and archived, never decoded.
	DataHR string `json:"data_hr"`
}

type zeppSummary struct {
	Steps  *zeppSteps `json:"stp"`
	Sleep  *zeppSleep `json:"slp"`
	Goal   int        `json:"goal"`
	Serial string     `json:"sn"`
}

type zeppSteps struct {
	Total    int `json:"ttl"`
	Calories int `json:"cal"`
	Distance int `json:"dis"`
}

type zeppSleep struct {
	// DeepMinutes and LightMinutes are the summary totals. Note lt is LIGHT
	// sleep: the zepp_to_influxdb reference records it as REM, which is wrong.
	DeepMinutes  int             `json:"dp"`
	LightMinutes int             `json:"lt"`
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

// decodeZeppSummary unwraps the base64-encoded JSON summary of one day.
func decodeZeppSummary(encoded string) (zeppSummary, error) {
	var summary zeppSummary
	if encoded == "" {
		return summary, fmt.Errorf("empty summary")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return summary, fmt.Errorf("base64: %w", err)
	}
	if err := json.Unmarshal(decoded, &summary); err != nil {
		return summary, fmt.Errorf("json: %w", err)
	}
	return summary, nil
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

// zeppSleepSpans converts the summary's stage spans into concrete intervals,
// dropping any span with an unknown code or a non-positive duration.
func zeppSleepSpans(day time.Time, sleep *zeppSleep) []zeppSleepInterval {
	if sleep == nil {
		return nil
	}
	intervals := make([]zeppSleepInterval, 0, len(sleep.Stages))
	for _, span := range sleep.Stages {
		stage := zeppSleepStageName(span.Mode)
		if stage == "" || span.Stop <= span.Start {
			continue
		}
		intervals = append(intervals, zeppSleepInterval{
			Stage:   stage,
			Start:   zeppSpanTime(day, span.Start),
			End:     zeppSpanTime(day, span.Stop),
			Minutes: span.Stop - span.Start,
		})
	}
	return intervals
}

type zeppSleepInterval struct {
	Stage   string
	Start   time.Time
	End     time.Time
	Minutes int
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
