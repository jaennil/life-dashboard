package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"life-dashboard/internal/observability"
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

	// The decoded summary is kept verbatim for raw_payload: storing the parsed
	// struct instead dropped every field the struct did not model, which is what
	// made the phase mapping impossible to check without re-fetching from the API.
	summary, rawSummary, err := decodeZeppSummary(day.Summary)
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

	stored, err := z.storeSleep(ctx, userID, dayStart, summary.Sleep, rawSummary)
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

// storeSleep writes one night into sleep_sessions plus its stage spans.
func (z *ZeppConnector) storeSleep(ctx context.Context, userID string, dayStart time.Time, sleep *zeppSleep, rawSummary []byte) (bool, error) {
	if sleep == nil || (sleep.DeepMinutes == 0 && sleep.LightMinutes == 0 && len(sleep.Stages) == 0) {
		return false, nil
	}

	intervals := zeppSleepSpans(dayStart, sleep)
	totals := zeppSleepTotals(intervals)

	// The summary wins over the spans: merging spans loses a few minutes per
	// night, which is why the stored total used to read ~25 minutes short of what
	// the Zepp app shows. Spans stay as the fallback for a summary that omits a
	// phase entirely.
	deep := firstPositive(sleep.DeepMinutes, totals["deep"])
	light := firstPositive(sleep.LightMinutes, totals["light"])
	rem := firstPositive(sleep.REMMinutes, totals["rem"])
	awake := firstPositive(sleep.AwakeMinutes, totals["awake"])
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

	var sessionID string
	err := z.db.QueryRow(ctx, `
		INSERT INTO sleep_sessions (
			user_id, source, date, sleep_start, sleep_end, total_sleep_minutes,
			deep_sleep_minutes, light_sleep_minutes, rem_sleep_minutes, awake_minutes,
			sleep_score, avg_resting_hr, raw_payload
		)
		VALUES ($1, $2, $3::date, $4, $5, NULLIF($6, 0), NULLIF($7, 0), NULLIF($8, 0), NULLIF($9, 0), NULLIF($10, 0), NULLIF($11, 0), NULLIF($12, 0), $13::jsonb)
		ON CONFLICT (user_id, source, date) DO UPDATE SET
			sleep_start = EXCLUDED.sleep_start,
			sleep_end = EXCLUDED.sleep_end,
			total_sleep_minutes = EXCLUDED.total_sleep_minutes,
			deep_sleep_minutes = EXCLUDED.deep_sleep_minutes,
			light_sleep_minutes = EXCLUDED.light_sleep_minutes,
			rem_sleep_minutes = EXCLUDED.rem_sleep_minutes,
			awake_minutes = EXCLUDED.awake_minutes,
			sleep_score = EXCLUDED.sleep_score,
			avg_resting_hr = EXCLUDED.avg_resting_hr,
			raw_payload = EXCLUDED.raw_payload
		RETURNING id
	`, userID, zeppSource, dayStart, start, end, total, deep, light, rem, awake,
		sleep.SleepScore, sleep.RestingHR, rawSummary).Scan(&sessionID)
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
//
// The one case that is not swallowed quietly is a section the provider answered
// and we then stored nothing from. A band that never recorded stress returns no
// items, which is normal and silent; items that all fail to store mean our
// reading of the payload is wrong. Those two used to look identical - an empty
// table either way - which is how a decoding bug kept stress and PAI out of the
// database for months without anyone noticing.
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

	saved, failed := 0, 0
	for _, item := range items {
		count, err := store(ctx, userID, item)
		if err != nil {
			failed++
			z.logger.Warn().Err(err).Str("event_type", eventType).Msg("store zepp event failed")
			continue
		}
		saved += count
	}

	if len(items) > 0 && saved == 0 {
		observability.RecordUnusableSection(zeppSource, eventType)
		z.logger.Error().
			Str("event_type", eventType).
			Int("items", len(items)).
			Int("decode_failures", failed).
			Msg("zepp section returned data but nothing could be stored")
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

	stamp := zeppUnixTime(item.Timestamp.int64())
	metrics := []zeppMetric{
		{"stress_avg", item.Avg.float(), "score"},
		{"stress_min", item.Min.float(), "score"},
		{"stress_max", item.Max.float(), "score"},
		{"stress_relaxed_share", item.Relax.float(), "%"},
		{"stress_normal_share", item.Normal.float(), "%"},
		{"stress_medium_share", item.Medium.float(), "%"},
		{"stress_high_share", item.High.float(), "%"},
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

	stamp := zeppUnixTime(item.Timestamp.int64())
	metrics := []zeppMetric{
		{"pai_total", item.TotalPAI.float(), "score"},
		{"pai_daily", item.DailyPAI.float(), "score"},
		{"pai_low_zone", item.LowZone.float(), "score"},
		{"pai_medium_zone", item.MedZone.float(), "score"},
		{"pai_high_zone", item.HighZone.float(), "score"},
		{"max_heart_rate", item.MaxHR.float(), "bpm"},
		{"resting_heart_rate", item.RestHR.float(), "bpm"},
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
	if item.Timestamp == 0 {
		return 0, nil
	}

	stamp := zeppUnixTime(item.Timestamp.int64())
	meta := map[string]any{"zepp_sub_type": item.SubType}

	saved := 0
	// A spot reading is a saturation percentage, taken when the band is asked for
	// one. It is the only thing here that may be called spo2.
	if value, ok := zeppOxygenValue(item); ok {
		if err := z.upsertMetric(ctx, userID, stamp, zeppMetric{"spo2", value, "%"}, meta); err != nil {
			return saved, err
		}
		saved++
	}

	// The overnight index is a different measurement in different units - dips
	// per hour, not percent - so it gets its own name rather than being filed as
	// a saturation nobody measured.
	if strings.EqualFold(item.SubType, "odi") && item.ODINum > 0 {
		for _, metric := range []zeppMetric{
			{"oxygen_desaturation_index", item.ODI.float(), "events/h"},
			{"oxygen_desaturation_events", item.ODINum.float(), "count"},
		} {
			if err := z.upsertMetric(ctx, userID, stamp, metric, meta); err != nil {
				return saved, err
			}
			saved++
		}
	}
	return saved, nil
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

// ingestSectionV2 is ingestSection against the newer events endpoint, which
// needs a subType alongside the event type.
func (z *ZeppConnector) ingestSectionV2(
	ctx context.Context,
	userID string,
	session zeppSession,
	eventType, subType string,
	from, to time.Time,
	store func(context.Context, string, json.RawMessage) (int, error),
) int {
	items, err := z.fetchEventsV2(ctx, session, eventType, subType, from, to)
	if err != nil {
		z.logger.Warn().Err(err).Str("event_type", eventType).Str("sub_type", subType).
			Msg("zepp event section failed")
		return 0
	}

	saved, failed := 0, 0
	for _, item := range items {
		count, err := store(ctx, userID, item)
		if err != nil {
			failed++
			z.logger.Warn().Err(err).Str("event_type", eventType).Msg("store zepp event failed")
			continue
		}
		saved += count
	}

	if len(items) > 0 && saved == 0 {
		observability.RecordUnusableSection(zeppSource, eventType)
		z.logger.Error().
			Str("event_type", eventType).
			Str("sub_type", subType).
			Int("items", len(items)).
			Int("decode_failures", failed).
			Msg("zepp section returned data but nothing could be stored")
	}
	return saved
}

// storeReadiness records the morning verdict the band forms about the night.
//
// sleepHRV is the number the app shows as the day's heart rate variability - it
// matched the app exactly on every one of the seven days it was checked against -
// and it is the only place HRV appears in this API at all.
func (z *ZeppConnector) storeReadiness(ctx context.Context, userID string, raw json.RawMessage) (int, error) {
	var event zeppReadinessEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return 0, err
	}

	value := event.Value
	stamp := zeppReadinessStamp(event, value)
	if stamp.IsZero() {
		return 0, nil
	}

	metrics := []zeppMetric{
		{"hrv", value.SleepHRV.float(), "ms"},
		{"hrv_baseline", value.HRVBaseline.float(), "ms"},
		{"hrv_score", value.HRVScore.float(), "score"},
		{"readiness_score", value.ReadinessScore.float(), "score"},
		{"physical_score", value.PhysicalScore.float(), "score"},
		{"mental_score", value.MentalScore.float(), "score"},
		{"sleep_resting_heart_rate", value.SleepRHR.float(), "bpm"},
	}

	saved := 0
	for _, metric := range metrics {
		// The band fills what it did not measure with sentinels rather than
		// omitting it: 255 for a score, 32767 for skin temperature. Storing those
		// would put a resting pulse of 255 into the history.
		if !zeppReadingIsReal(metric.value) {
			continue
		}
		if err := z.upsertMetric(ctx, userID, stamp, metric, nil); err != nil {
			return saved, err
		}
		saved++
	}
	return saved, nil
}

// zeppReadingIsReal rejects the placeholders the band uses for "not measured".
func zeppReadingIsReal(value float64) bool {
	return value > 0 && value != 255 && value != 32767
}

// zeppReadinessStamp puts the verdict on the night it describes. The inner
// timestamp is the start of that day; midday keeps DATE() on the same day under
// either timezone, the way every other daily metric here is stored.
func zeppReadinessStamp(event zeppReadinessEvent, value zeppReadinessValue) time.Time {
	millis := value.Timestamp.int64()
	if millis == 0 {
		millis = event.Timestamp.int64()
	}
	if millis == 0 {
		return time.Time{}
	}
	return zeppUnixTime(millis).Add(12 * time.Hour)
}
