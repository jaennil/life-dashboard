package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	xiaomiScaleSource   = "xiaomi_scale"
	xiaomiScaleModel    = "yunmai.scales.ms104"
	xiaomiScaleLookback = 7 * 24 * time.Hour
)

// The scale reports weight plus two impedance bands; every percentage and mass
// below it is computed by Xiaomi in the cloud, so these are their numbers, not
// ours. bodyRes/bodyRes2 are the raw inputs, kept so the composition can be
// recomputed locally later.
//
// Frequency naming follows xiaomi-ble: the 50 kHz band reads higher than the
// 250 kHz one, so the larger value (bodyRes) is impedance_low. Implementations
// disagree on these labels - the raw field name is recorded in metadata.
var xiaomiScaleMetrics = []struct {
	metricType string
	field      string
	unit       string
}{
	{"weight", "weight", "kg"},
	{"bmi", "bmi", ""},
	{"body_fat", "bfp", "%"},
	{"lean_body_mass", "ffm", "kg"},
	{"muscle_mass", "slm", "kg"},
	{"skeletal_muscle_mass", "smm", "kg"},
	{"body_water", "bwp", "%"},
	{"bone_mass", "bmc", "kg"},
	{"protein_mass", "pm", "kg"},
	{"visceral_fat", "vfl", ""},
	{"bmr", "bmr", "kcal"},
	{"metabolic_age", "ma", "years"},
	{"heart_rate", "heartRate", "bpm"},
	{"impedance_low", "bodyRes", "ohm"},
	{"impedance_high", "bodyRes2", "ohm"},
}

type XiaomiScaleConnector struct {
	db     *pgxpool.Pool
	region string
	model  string
	logger zerolog.Logger
}

func NewXiaomiScale(region, model string, db *pgxpool.Pool, logger zerolog.Logger) *XiaomiScaleConnector {
	if model == "" {
		model = xiaomiScaleModel
	}
	return &XiaomiScaleConnector{
		db:     db,
		region: region,
		model:  model,
		logger: logger.With().Str("connector", xiaomiScaleSource).Logger(),
	}
}

func (x *XiaomiScaleConnector) Name() string { return xiaomiScaleSource }

func (x *XiaomiScaleConnector) Sync(ctx context.Context, userID string) error {
	passToken, accountID, err := x.loadCredentials(ctx, userID)
	if err != nil {
		return err
	}

	cloud := NewXiaomiCloud(x.region)
	if err := cloud.Login(ctx, accountID, passToken); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	// Xiaomi usually replays the same passToken, but it may hand back a new
	// one; persisting it keeps the unattended sync alive across a rotation.
	if fresh := cloud.PassToken(); fresh != "" && fresh != passToken {
		if err := x.storePassToken(ctx, userID, fresh); err != nil {
			x.logger.Warn().Err(err).Msg("could not persist refreshed passToken")
		}
	}

	cutoff := x.syncCutoff(ctx, userID)
	records, err := cloud.FetchSince(ctx, x.model, cutoff)
	if err != nil {
		return fmt.Errorf("fetch measurements: %w", err)
	}

	saved, mine, foreign := 0, 0, 0
	for _, record := range records {
		n, ok, err := x.storeRecord(ctx, userID, accountID, record)
		if err != nil {
			return fmt.Errorf("store measurement %s: %w", record.MeasuredAt().Format(time.RFC3339), err)
		}
		if !ok {
			foreign++
			continue
		}
		mine++
		saved += n
	}

	if err := x.updateLastSync(ctx, userID); err != nil {
		return fmt.Errorf("update last sync: %w", err)
	}

	x.logger.Info().
		Int("measurements", mine).
		Int("metrics", saved).
		Int("other_profiles", foreign).
		Msg("xiaomi scale sync complete")
	return nil
}

func (x *XiaomiScaleConnector) loadCredentials(ctx context.Context, userID string) (passToken, accountID string, err error) {
	err = x.db.QueryRow(ctx, `
		SELECT access_token, refresh_token FROM oauth_tokens
		WHERE source = $1 AND user_id = $2
	`, xiaomiScaleSource, userID).Scan(&passToken, &accountID)
	if err != nil {
		return "", "", fmt.Errorf("no credentials — add your Xiaomi passToken in Settings")
	}
	if strings.TrimSpace(accountID) == "" {
		return "", "", fmt.Errorf("no Xiaomi account id — re-save the credentials in Settings")
	}
	return passToken, strings.TrimSpace(accountID), nil
}

func (x *XiaomiScaleConnector) storePassToken(ctx context.Context, userID, passToken string) error {
	_, err := x.db.Exec(ctx, `
		UPDATE oauth_tokens SET access_token = $1, updated_at = NOW()
		WHERE source = $2 AND user_id = $3
	`, passToken, xiaomiScaleSource, userID)
	return err
}

// syncCutoff returns the oldest measurement worth re-reading. The first sync
// pulls the full history; later ones re-read a short overlap so measurements
// that reached the cloud late are still picked up.
func (x *XiaomiScaleConnector) syncCutoff(ctx context.Context, userID string) time.Time {
	var lastSync time.Time
	err := x.db.QueryRow(ctx, `
		SELECT last_synced_at FROM sync_state WHERE source = $1 AND user_id = $2
	`, xiaomiScaleSource, userID).Scan(&lastSync)
	if err != nil || lastSync.IsZero() {
		return time.Time{}
	}
	return lastSync.Add(-xiaomiScaleLookback)
}

// storeRecord expands one weigh-in into biometrics rows. It reports how many
// were written and whether the record belongs to this account at all. The full
// payload goes to raw_events once, not per metric.
func (x *XiaomiScaleConnector) storeRecord(ctx context.Context, userID, accountID string, record XiaomiScaleRecord) (int, bool, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(record.Data), &payload); err != nil {
		return 0, false, fmt.Errorf("decode payload: %w", err)
	}

	// getUserDataByPage returns every profile on the scale, and they all share
	// the outer account uid. The inner user.accountId is what actually
	// distinguishes the account owner from family members.
	user := jsonObject(payload, "user")
	if owner := jsonString(user, "accountId"); owner != "" && owner != accountID {
		return 0, false, nil
	}

	measuredAt := record.MeasuredAt()
	externalID := strconv.FormatInt(record.CreateTime, 10)
	profile := jsonString(user, "name")

	// Casts are required: the same parameters are reused in the guard, and
	// Postgres will not deduce a single type for them otherwise.
	if _, err := x.db.Exec(ctx, `
		INSERT INTO raw_events (source, event_type, external_id, payload, user_id)
		SELECT $1::varchar, 'measurement', $2::varchar, $3::jsonb, $4::uuid
		WHERE NOT EXISTS (
			SELECT 1 FROM raw_events
			WHERE source = $1::varchar AND external_id = $2::varchar AND user_id = $4::uuid
		)
	`, xiaomiScaleSource, externalID, record.Data, userID); err != nil {
		return 0, false, fmt.Errorf("insert raw event: %w", err)
	}

	saved := 0
	for _, metric := range xiaomiScaleMetrics {
		value, ok := xiaomiScaleNumber(payload, metric.field)
		// A weigh-in with shoes on, or one the scale could not stabilise, has
		// no impedance, and Xiaomi fills every derived field with zero. None of
		// these metrics can legitimately be zero, so that means "not measured"
		// - storing it would put a 0 % body fat point on the chart.
		if !ok || value <= 0 {
			continue
		}
		metadata, _ := json.Marshal(map[string]string{
			"source_field": metric.field,
			"model":        record.Model,
			"did":          record.Did,
			"profile":      profile,
		})

		if _, err := x.db.Exec(ctx, `
			INSERT INTO biometrics (timestamp, source, metric_type, value, unit, metadata, user_id)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
			ON CONFLICT (user_id, timestamp, source, metric_type) DO UPDATE SET
				value = EXCLUDED.value,
				unit = EXCLUDED.unit,
				metadata = EXCLUDED.metadata
		`, measuredAt, xiaomiScaleSource, metric.metricType, value,
			nullIfEmpty(metric.unit), metadata, userID); err != nil {
			return saved, true, fmt.Errorf("upsert %s: %w", metric.metricType, err)
		}
		saved++
	}
	return saved, true, nil
}

func (x *XiaomiScaleConnector) updateLastSync(ctx context.Context, userID string) error {
	_, err := x.db.Exec(ctx, `
		INSERT INTO sync_state (source, last_synced_at, updated_at, enabled, user_id)
		VALUES ($1, NOW(), NOW(), TRUE, $2)
		ON CONFLICT (source, user_id) DO UPDATE SET
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at = EXCLUDED.updated_at,
			enabled = TRUE
	`, xiaomiScaleSource, userID)
	return err
}

// xiaomiScaleNumber reads a metric that Xiaomi may encode as either a JSON
// number or a quoted string, depending on the field.
func xiaomiScaleNumber(payload map[string]any, key string) (float64, bool) {
	value, ok := payload[key]
	if !ok || value == nil {
		return 0, false
	}
	switch v := value.(type) {
	case float64:
		return v, true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	}
	return 0, false
}
