package handlers

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Building a nested array of dictionaries inside a Shortcuts "Get Contents of
// URL" body is a lot of fiddly tapping, so the webhook also accepts one flat
// field per metric:
//
//	{"api_key": "...", "date": "2026-08-08", "steps": 12345, "hrv": 42}
//
// Any top-level numeric field that is not a known control key is treated as a
// metric, which means new metrics need no backend change at all.
var healthFlatControlKeys = map[string]bool{
	"api_key": true, "apikey": true, "source": true, "event_type": true,
	"date": true, "day": true, "timestamp": true, "timezone": true,
	"device": true, "during": true, "note": true, "notes": true,
	"data": true, "metrics": true, "sleep": true, "sleeps": true,
}

// Flat sleep fields build a session rather than a biometrics row, so they are
// control keys as far as metric collection is concerned.
var healthFlatSleepKeys = map[string]string{
	"sleep_minutes":       "total",
	"total_sleep_minutes": "total",
	"asleep_minutes":      "total",
	"deep_sleep_minutes":  "deep",
	"rem_sleep_minutes":   "rem",
	"light_sleep_minutes": "light",
	"core_sleep_minutes":  "light",
	"awake_minutes":       "awake",
	"in_bed_minutes":      "in_bed",
	"sleep_score":         "score",
	"sleep_start":         "start",
	"sleep_end":           "end",
	"avg_hrv":             "avg_hrv",
	"avg_resting_hr":      "avg_resting_hr",
}

// flatHealthDay resolves the day a flat payload describes. Metrics are stored at
// local midnight of that day so repeated runs upsert the same row instead of
// piling up one row per request.
func flatHealthDay(fields map[string]json.RawMessage) time.Time {
	now := time.Now().In(aiDisplayLocation)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, aiDisplayLocation)

	for _, key := range []string{"date", "day", "timestamp"} {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		value, ok := healthFlatString(raw)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		parsed, err := parseDate(value)
		if err != nil {
			continue
		}
		return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, aiDisplayLocation)
	}
	return day
}

// parseFlatHealthMetrics turns unrecognized top-level numeric fields into metric
// entries. Fields it cannot read as numbers are reported so a silently dropped
// value stays visible.
func parseFlatHealthMetrics(fields map[string]json.RawMessage, day time.Time) (entries []healthEntry, skipped []string) {
	// Daily aggregates are stamped at local midday, not midnight. Existing queries
	// bucket with DATE(timestamp), which evaluates in the database session's
	// timezone (UTC in production), so a local-midnight stamp would land the value
	// on the previous day. Midday reads as the same date under either timezone.
	stamp := day.Add(12 * time.Hour).Format(time.RFC3339)

	for key, raw := range fields {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		if healthFlatControlKeys[normalizedKey] {
			continue
		}
		if _, isSleep := healthFlatSleepKeys[normalizedKey]; isSleep {
			continue
		}

		value, ok := healthFlatNumber(raw)
		if !ok {
			skipped = append(skipped, key)
			continue
		}

		metricType := normalizeHealthMetricType(key)
		entries = append(entries, healthEntry{
			Type:      metricType,
			Value:     value,
			Unit:      normalizeHealthUnit(metricType, ""),
			Timestamp: stamp,
		})
	}
	return entries, skipped
}

// parseFlatHealthSleep builds a sleep session out of flat sleep_* fields. It
// returns false when the payload carries no sleep numbers at all.
func parseFlatHealthSleep(fields map[string]json.RawMessage, day time.Time) (healthSleepSession, bool) {
	session := healthSleepSession{Date: day.Format("2006-01-02")}
	found := false

	for key, raw := range fields {
		role, ok := healthFlatSleepKeys[strings.ToLower(strings.TrimSpace(key))]
		if !ok {
			continue
		}

		if role == "start" || role == "end" {
			text, ok := healthFlatString(raw)
			if !ok || strings.TrimSpace(text) == "" {
				continue
			}
			if role == "start" {
				session.StartDate = text
			} else {
				session.EndDate = text
			}
			found = true
			continue
		}

		value, ok := healthFlatNumber(raw)
		if !ok {
			continue
		}
		switch role {
		case "total":
			session.TotalSleepMinutes = int(value)
		case "deep":
			session.DeepSleepMinutes = int(value)
		case "rem":
			session.REMSleepMinutes = int(value)
		case "light":
			session.LightSleepMinutes = int(value)
		case "awake":
			session.AwakeMinutes = int(value)
		case "in_bed":
			session.TotalMinutes = int(value)
		case "score":
			session.SleepScore = int(value)
		case "avg_hrv":
			session.AvgHRV = value
		case "avg_resting_hr":
			session.AvgRestingHR = int(value)
		}
		found = true
	}

	if !found {
		return healthSleepSession{}, false
	}
	return session, true
}

// healthFlatNumber reads a JSON number, or a number sent as a string: Shortcuts
// quotes values often enough that refusing them would lose data.
func healthFlatNumber(raw json.RawMessage) (float64, bool) {
	// Unmarshalling JSON null into a float64 succeeds and leaves a zero behind,
	// which would record a fabricated 0 for a metric the phone had no value for.
	if string(bytes.TrimSpace(raw)) == "null" {
		return 0, false
	}

	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		return number, true
	}

	text, ok := healthFlatString(raw)
	if !ok {
		return 0, false
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, ",", "."))
	text = strings.ReplaceAll(text, " ", "")
	text = strings.ReplaceAll(text, " ", "")
	if text == "" {
		return 0, false
	}
	number, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

func healthFlatString(raw json.RawMessage) (string, bool) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, true
	}
	return "", false
}
