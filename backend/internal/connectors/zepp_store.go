package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// zeppMetric is one value ready for the biometrics table.
type zeppMetric struct {
	metricType string
	value      float64
	unit       string
}

// storeBandDay writes the step metrics, per-minute heart rate and sleep for one day.
func (z *ZeppConnector) storeBandDay(ctx context.Context, userID string, day zeppBandDay) (int, bool, int, error) {
	dayStart, err := time.ParseInLocation("2006-01-02", day.DateTime, time.Local)
	if err != nil {
		return 0, false, 0, fmt.Errorf("parse date %q: %w", day.DateTime, err)
	}

	summary, err := decodeZeppSummary(day.Summary)
	if err != nil {
		return 0, false, 0, fmt.Errorf("summary: %w", err)
	}

	metrics := []zeppMetric{}
	if summary.Steps != nil {
		metrics = append(metrics,
			zeppMetric{"steps", float64(summary.Steps.Total), "count"},
			zeppMetric{"walking_running_distance", float64(summary.Steps.Distance), "m"},
			zeppMetric{"active_energy", float64(summary.Steps.Calories), "kcal"},
		)
		if summary.Steps.RunDistance > 0 {
			metrics = append(metrics, zeppMetric{"running_distance", float64(summary.Steps.RunDistance), "m"})
		}
	}
	if summary.Heart != nil && summary.Heart.MaxHR != nil && summary.Heart.MaxHR.HR > 0 {
		metrics = append(metrics, zeppMetric{"max_heart_rate", float64(summary.Heart.MaxHR.HR), "bpm"})
	}
	if summary.Sleep != nil && summary.Sleep.RestingHR > 0 {
		metrics = append(metrics, zeppMetric{"resting_heart_rate", float64(summary.Sleep.RestingHR), "bpm"})
	}

	saved := 0
	// Daily aggregates are stamped at local midday so DATE(timestamp) resolves to
	// this day under either timezone, matching how the health webhook stores them.
	stamp := dayStart.Add(12 * time.Hour)
	for _, metric := range metrics {
		if err := z.upsertMetric(ctx, userID, stamp, metric, map[string]any{"zepp_serial": summary.Serial}); err != nil {
			return saved, false, 0, err
		}
		saved++
	}

	heartRates, err := z.storeHeartRate(ctx, userID, dayStart, day.DataHR)
	if err != nil {
		return saved, false, heartRates, err
	}

	stored, err := z.storeSleep(ctx, userID, dayStart, summary.Sleep)
	if err != nil {
		return saved, false, heartRates, err
	}
	return saved, stored, heartRates, nil
}

// storeHeartRate expands the per-minute blob into individual readings. Each minute
// is its own biometrics row, which the natural key already keeps idempotent.
func (z *ZeppConnector) storeHeartRate(ctx context.Context, userID string, dayStart time.Time, blob string) (int, error) {
	samples, err := decodeZeppHeartRate(blob, dayStart)
	if err != nil {
		return 0, fmt.Errorf("heart rate blob: %w", err)
	}

	saved := 0
	for _, sample := range samples {
		metric := zeppMetric{"heart_rate", float64(sample.BPM), "bpm"}
		if err := z.upsertMetric(ctx, userID, sample.At, metric, nil); err != nil {
			return saved, err
		}
		saved++
	}
	return saved, nil
}

// storeSleep writes one night into sleep_sessions plus its stage spans. REM and
// awake minutes come from the spans: the summary itself only totals deep and light.
func (z *ZeppConnector) storeSleep(ctx context.Context, userID string, dayStart time.Time, sleep *zeppSleep) (bool, error) {
	if sleep == nil || (sleep.DeepMinutes == 0 && sleep.LightMinutes == 0 && len(sleep.Stages) == 0) {
		return false, nil
	}

	intervals := zeppSleepSpans(dayStart, sleep)
	totals := zeppSleepTotals(intervals)

	deep := firstPositive(totals["deep"], sleep.DeepMinutes)
	light := firstPositive(totals["light"], sleep.LightMinutes)
	rem := totals["rem"]
	awake := totals["awake"]
	total := deep + light + rem

	var start, end *time.Time
	if sleep.Start > 0 {
		value := zeppUnixTime(sleep.Start)
		start = &value
	}
	if sleep.End > 0 {
		value := zeppUnixTime(sleep.End)
		end = &value
	}

	raw, _ := json.Marshal(sleep)
	var sessionID string
	err := z.db.QueryRow(ctx, `
		INSERT INTO sleep_sessions (
			user_id, source, date, sleep_start, sleep_end, total_sleep_minutes,
			deep_sleep_minutes, light_sleep_minutes, rem_sleep_minutes, awake_minutes, raw_payload
		)
		VALUES ($1, $2, $3::date, $4, $5, NULLIF($6, 0), NULLIF($7, 0), NULLIF($8, 0), NULLIF($9, 0), NULLIF($10, 0), $11::jsonb)
		ON CONFLICT (user_id, source, date) DO UPDATE SET
			sleep_start = EXCLUDED.sleep_start,
			sleep_end = EXCLUDED.sleep_end,
			total_sleep_minutes = EXCLUDED.total_sleep_minutes,
			deep_sleep_minutes = EXCLUDED.deep_sleep_minutes,
			light_sleep_minutes = EXCLUDED.light_sleep_minutes,
			rem_sleep_minutes = EXCLUDED.rem_sleep_minutes,
			awake_minutes = EXCLUDED.awake_minutes,
			raw_payload = EXCLUDED.raw_payload
		RETURNING id
	`, userID, zeppSource, dayStart, start, end, total, deep, light, rem, awake, raw).Scan(&sessionID)
	if err != nil {
		return false, fmt.Errorf("upsert sleep session: %w", err)
	}

	if len(intervals) == 0 {
		return true, nil
	}
	// Spans are replaced wholesale: a re-read of the same night is authoritative.
	if _, err := z.db.Exec(ctx, `DELETE FROM sleep_stages WHERE session_id = $1`, sessionID); err != nil {
		return true, fmt.Errorf("clear sleep stages: %w", err)
	}
	for _, interval := range intervals {
		if _, err := z.db.Exec(ctx, `
			INSERT INTO sleep_stages (session_id, started_at, ended_at, stage)
			VALUES ($1, $2, $3, $4)
		`, sessionID, interval.Start, interval.End, interval.Stage); err != nil {
			return true, fmt.Errorf("insert sleep stage: %w", err)
		}
	}
	return true, nil
}

// ingestSection fetches one event type and hands each item to a storer. A failure
// is logged and swallowed so one dead section cannot lose the others.
func (z *ZeppConnector) ingestSection(
	ctx context.Context,
	userID string,
	session zeppSession,
	eventType string,
	from, to time.Time,
	store func(context.Context, string, json.RawMessage) (int, error),
) int {
	items, err := z.fetchEvents(ctx, session, eventType, from, to)
	if err != nil {
		z.logger.Warn().Err(err).Str("event_type", eventType).Msg("zepp event section failed")
		return 0
	}

	saved := 0
	for _, item := range items {
		count, err := store(ctx, userID, item)
		if err != nil {
			z.logger.Warn().Err(err).Str("event_type", eventType).Msg("store zepp event failed")
			continue
		}
		saved += count
	}
	return saved
}

func (z *ZeppConnector) storeStress(ctx context.Context, userID string, raw json.RawMessage) (int, error) {
	var item zeppStressItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return 0, err
	}
	if item.Timestamp == 0 {
		return 0, nil
	}

	stamp := zeppUnixTime(item.Timestamp)
	metrics := []zeppMetric{
		{"stress_avg", float64(item.Avg), "score"},
		{"stress_min", float64(item.Min), "score"},
		{"stress_max", float64(item.Max), "score"},
		{"stress_relaxed_share", float64(item.Relax), "%"},
		{"stress_normal_share", float64(item.Normal), "%"},
		{"stress_medium_share", float64(item.Medium), "%"},
		{"stress_high_share", float64(item.High), "%"},
	}

	saved := 0
	for _, metric := range metrics {
		if metric.value == 0 && metric.metricType != "stress_avg" {
			continue
		}
		if err := z.upsertMetric(ctx, userID, stamp, metric, nil); err != nil {
			return saved, err
		}
		saved++
	}
	return saved, nil
}

func (z *ZeppConnector) storePAI(ctx context.Context, userID string, raw json.RawMessage) (int, error) {
	var item zeppPAIItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return 0, err
	}
	if item.Timestamp == 0 {
		return 0, nil
	}

	stamp := zeppUnixTime(item.Timestamp)
	metrics := []zeppMetric{
		{"pai_total", item.TotalPAI, "score"},
		{"pai_daily", item.DailyPAI, "score"},
		{"pai_low_zone", item.LowZone, "score"},
		{"pai_medium_zone", item.MedZone, "score"},
		{"pai_high_zone", item.HighZone, "score"},
		{"max_heart_rate", float64(item.MaxHR), "bpm"},
		{"resting_heart_rate", float64(item.RestHR), "bpm"},
	}

	saved := 0
	for _, metric := range metrics {
		if metric.value == 0 {
			continue
		}
		if err := z.upsertMetric(ctx, userID, stamp, metric, nil); err != nil {
			return saved, err
		}
		saved++
	}
	return saved, nil
}

func (z *ZeppConnector) storeOxygen(ctx context.Context, userID string, raw json.RawMessage) (int, error) {
	var item zeppOxygenItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return 0, err
	}
	value, ok := zeppOxygenValue(item)
	if !ok || item.Timestamp == 0 {
		return 0, nil
	}

	stamp := zeppUnixTime(item.Timestamp)
	metric := zeppMetric{"spo2", value, "%"}
	if err := z.upsertMetric(ctx, userID, stamp, metric, map[string]any{"zepp_sub_type": item.SubType}); err != nil {
		return 0, err
	}
	return 1, nil
}

func (z *ZeppConnector) upsertMetric(ctx context.Context, userID string, stamp time.Time, metric zeppMetric, meta map[string]any) error {
	metadata := []byte("{}")
	if len(meta) > 0 {
		if encoded, err := json.Marshal(meta); err == nil {
			metadata = encoded
		}
	}

	if _, err := z.db.Exec(ctx, `
		INSERT INTO biometrics (timestamp, source, metric_type, value, unit, metadata, user_id)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
		ON CONFLICT (user_id, timestamp, source, metric_type) DO UPDATE SET
			value = EXCLUDED.value,
			unit = EXCLUDED.unit,
			metadata = EXCLUDED.metadata
	`, stamp, zeppSource, metric.metricType, metric.value, metric.unit, metadata, userID); err != nil {
		return fmt.Errorf("upsert %s: %w", metric.metricType, err)
	}
	return nil
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
