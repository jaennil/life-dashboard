package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Health Auto Export posts everything under a "data" object:
//
//	{"data": {
//	   "metrics": [{"name": "step_count", "units": "count",
//	                "data": [{"date": "2026-01-10 00:00:00 +0300", "qty": 196, "source": "iPhone"}]}],
//	   "workouts": [...], "stateOfMind": [...]}}
//
// Only metrics are ingested for now; the other sections need tables of their own
// and are reported back so their absence is visible rather than silent.
type healthAutoExportPayload struct {
	Data struct {
		Metrics     []healthAutoExportMetric `json:"metrics"`
		Workouts    []json.RawMessage        `json:"workouts"`
		StateOfMind []json.RawMessage        `json:"stateOfMind"`
	} `json:"data"`
}

type healthAutoExportMetric struct {
	Name  string            `json:"name"`
	Units string            `json:"units"`
	Data  []json.RawMessage `json:"data"`
}

type healthAutoExportReport struct {
	Metrics          int      `json:"metrics"`
	Points           int      `json:"points"`
	Workouts         int      `json:"workouts_ignored"`
	StateOfMind      int      `json:"state_of_mind_ignored"`
	UnreadablePoints int      `json:"unreadable_points"`
	Converted        []string `json:"converted_units"`
}

// looksLikeHealthAutoExport reports whether the body is a Health Auto Export
// payload, so the generic paths can be skipped for it.
func looksLikeHealthAutoExport(fields map[string]json.RawMessage) bool {
	raw, ok := fields["data"]
	if !ok {
		return false
	}
	var inner map[string]json.RawMessage
	if err := json.Unmarshal(raw, &inner); err != nil {
		return false
	}
	_, hasMetrics := inner["metrics"]
	return hasMetrics
}

// parseHealthAutoExport converts the export into metric entries.
func parseHealthAutoExport(body []byte) ([]healthEntry, healthAutoExportReport, error) {
	var payload healthAutoExportPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, healthAutoExportReport{}, err
	}

	report := healthAutoExportReport{
		Workouts:    len(payload.Data.Workouts),
		StateOfMind: len(payload.Data.StateOfMind),
		Converted:   []string{},
	}
	converted := map[string]bool{}
	entries := []healthEntry{}

	for _, metric := range payload.Data.Metrics {
		metricType := normalizeHealthMetricType(metric.Name)
		if metricType == "" {
			continue
		}
		report.Metrics++

		for _, rawPoint := range metric.Data {
			var point struct {
				Date   string  `json:"date"`
				Qty    float64 `json:"qty"`
				Avg    float64 `json:"Avg"`
				Source string  `json:"source"`
			}
			if err := json.Unmarshal(rawPoint, &point); err != nil {
				report.UnreadablePoints++
				continue
			}

			stamp, err := parseHealthAutoExportDate(point.Date)
			if err != nil {
				report.UnreadablePoints++
				continue
			}

			// Discrete metrics such as heart rate carry Avg instead of qty.
			value := point.Qty
			if value == 0 && point.Avg != 0 {
				value = point.Avg
			}

			value, unit, didConvert := convertHealthUnit(value, metric.Units)
			if didConvert && !converted[metric.Name] {
				converted[metric.Name] = true
				report.Converted = append(report.Converted, metric.Name+": "+metric.Units+" -> "+unit)
			}

			entries = append(entries, healthEntry{
				Type:      metricType,
				Value:     value,
				Unit:      unit,
				Timestamp: stamp.Format(time.RFC3339),
				Metadata:  map[string]any{"hae_source": point.Source, "hae_metric": metric.Name},
			})
			report.Points++
		}
	}

	return entries, report, nil
}

// parseHealthAutoExportDate reads "2026-01-10 00:00:00 +0300".
//
// A daily aggregate arrives stamped at local midnight, which existing queries
// bucketing with DATE(timestamp) would attribute to the previous day once the
// database evaluates it in UTC. Midnight-exact points are therefore moved to
// local midday; anything with a real time of day is left alone.
func parseHealthAutoExportDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05 Z0700",
		time.RFC3339,
	} {
		parsed, err := time.Parse(layout, value)
		if err != nil {
			continue
		}
		if parsed.Hour() == 0 && parsed.Minute() == 0 && parsed.Second() == 0 {
			return parsed.Add(12 * time.Hour), nil
		}
		return parsed, nil
	}
	return time.Time{}, &time.ParseError{Value: value, Layout: "2006-01-02 15:04:05 -0700"}
}

// convertHealthUnit normalizes the imperial units Health Auto Export emits when
// the phone is set to US measurements, so a weight in pounds can never land in
// the same column as a weight in kilograms.
func convertHealthUnit(value float64, unit string) (float64, string, bool) {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "lb", "lbs":
		return value * 0.45359237, "kg", true
	case "mi":
		return value * 1609.344, "m", true
	case "ft":
		return value * 0.3048, "m", true
	case "in":
		return value * 2.54, "cm", true
	case "mi/hr", "mph":
		return value * 1.609344, "km/hr", true
	case "oz":
		return value * 28.349523125, "g", true
	case "fl_oz_us", "floz":
		return value * 29.5735295625, "ml", true
	case "°f", "degf":
		return (value - 32) * 5 / 9, "°C", true
	}
	return value, unit, false
}

// ingestHealthAutoExport handles a Health Auto Export payload end to end. The raw
// body is already archived by the caller, so a parse failure here reports what
// arrived rather than discarding it.
func (h *HealthWebhookHandler) ingestHealthAutoExport(
	w http.ResponseWriter,
	r *http.Request,
	userID, source string,
	bodyBytes, cleaned []byte,
	rawEventID string,
) {
	entries, report, err := parseHealthAutoExport(cleaned)
	if err != nil {
		preview, truncated := previewBody(bodyBytes)
		h.logger.Warn().
			Err(err).
			Str("user_id", userID).
			Str("raw_event_id", rawEventID).
			Msg("health auto export payload could not be parsed")
		writeJSON(w, map[string]any{
			"status":       "health_auto_export_parse_failed",
			"error":        err.Error(),
			"raw_event_id": rawEventID,
			"raw_preview":  preview,
			"truncated":    truncated,
		})
		return
	}

	saved, skipped := h.saveHealthMetrics(r.Context(), userID, source, entries)

	if _, err := h.db.Exec(r.Context(), `
		INSERT INTO sync_state (source, last_synced_at, updated_at, enabled, user_id)
		VALUES ($1, NOW(), NOW(), TRUE, $2)
		ON CONFLICT (source, user_id) DO UPDATE SET
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at = NOW(),
			enabled = TRUE
	`, appleHealthSource, userID); err != nil {
		h.logger.Warn().Err(err).Msg("update apple health sync state failed")
	}

	h.logger.Info().
		Str("user_id", userID).
		Str("source", source).
		Int("metrics", report.Metrics).
		Int("points", report.Points).
		Int("saved", saved).
		Int("skipped", skipped).
		Int("unreadable_points", report.UnreadablePoints).
		Int("workouts_ignored", report.Workouts).
		Int("state_of_mind_ignored", report.StateOfMind).
		Strs("converted_units", report.Converted).
		Msg("health auto export ingested")

	writeJSON(w, map[string]any{
		"status":                "ok",
		"shape":                 "health_auto_export",
		"metrics":               report.Metrics,
		"points":                report.Points,
		"metrics_saved":         saved,
		"metrics_skipped":       skipped,
		"unreadable_points":     report.UnreadablePoints,
		"workouts_ignored":      report.Workouts,
		"state_of_mind_ignored": report.StateOfMind,
		"converted_units":       report.Converted,
		"raw_event_id":          rawEventID,
	})
}
