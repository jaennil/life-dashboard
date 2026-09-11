package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const locationSourceOverland = "overland"

const (
	// A full batch is 1000 fixes of a few hundred bytes each; the cap leaves room
	// for that and rejects anything that is not a location batch at all.
	locationMaxBody = 8 << 20
	// Overland's own maximum batch size, used as the sanity bound.
	locationMaxPoints = 1000
)

type LocationWebhookHandler struct {
	db     *pgxpool.Pool
	logger zerolog.Logger
}

func NewLocationWebhook(db *pgxpool.Pool, logger zerolog.Logger) *LocationWebhookHandler {
	return &LocationWebhookHandler{
		db:     db,
		logger: logger.With().Str("handler", "location_webhook").Logger(),
	}
}

// overlandBatch is what the phone posts: a GeoJSON FeatureCollection in all but
// name. "current" and "trip" also arrive but carry nothing the fixes do not.
type overlandBatch struct {
	Locations []overlandFeature `json:"locations"`
}

type overlandFeature struct {
	Geometry struct {
		// [longitude, latitude] - GeoJSON order, which is the reverse of how
		// every map UI shows it.
		Coordinates []float64 `json:"coordinates"`
	} `json:"geometry"`
	// Kept as raw JSON as well as decoded: the app sends optional tracking stats
	// this struct does not model, and re-encoding the struct would drop them.
	Properties json.RawMessage `json:"properties"`
}

type overlandProperties struct {
	Timestamp          string   `json:"timestamp"`
	Altitude           *float64 `json:"altitude"`
	Speed              *float64 `json:"speed"`
	Course             *float64 `json:"course"`
	HorizontalAccuracy *float64 `json:"horizontal_accuracy"`
	Motion             []string `json:"motion"`
	BatteryState       string   `json:"battery_state"`
	BatteryLevel       *float64 `json:"battery_level"`
	Wifi               string   `json:"wifi"`
	DeviceID           string   `json:"device_id"`
}

// ReceiveOverland ingests one batch of location fixes.
//
// POST /api/v1/webhook/overland
//
// The phone keeps a batch on disk and resends it until the body comes back as
// {"result":"ok"}, so this answers that exact shape on success and anything else
// on failure - a dropped batch is data that no longer exists anywhere.
func (h *LocationWebhookHandler) ReceiveOverland(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, locationMaxBody))
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}

	apiKey := healthAPIKeyFromRequest(r, "")
	if apiKey == "" {
		http.Error(w, "api_key required", http.StatusUnauthorized)
		return
	}
	var userID string
	if err := h.db.QueryRow(r.Context(),
		`SELECT user_id FROM api_keys WHERE key = $1`, apiKey).Scan(&userID); err != nil {
		http.Error(w, "invalid api_key", http.StatusUnauthorized)
		return
	}

	var batch overlandBatch
	if err := json.Unmarshal(body, &batch); err != nil {
		// Keep what could not be read: the phone will drop this batch as soon as
		// it sees a failure response, and then the payload is gone for good.
		h.archiveUnparsedBatch(r.Context(), userID, body, err)
		h.logger.Error().Err(err).Int("bytes", len(body)).Msg("decode location batch")
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if len(batch.Locations) > locationMaxPoints {
		batch.Locations = batch.Locations[:locationMaxPoints]
	}

	points := parseOverlandPoints(batch)
	saved, err := h.storeLocationPoints(r.Context(), userID, points)
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", userID).Msg("store location points")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if saved > 0 {
		h.touchLocationSync(r.Context(), userID)
	}
	h.logger.Info().Str("user_id", userID).
		Int("received", len(batch.Locations)).Int("stored", saved).Msg("location batch ingested")

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"result":"ok"}`))
}

// locationPoint is one fix, already in the shape the table stores.
type locationPoint struct {
	RecordedAt   time.Time
	Latitude     float64
	Longitude    float64
	Accuracy     *float64
	Altitude     *float64
	Speed        *float64
	Course       *float64
	Motion       []string
	BatteryLevel *float64
	BatteryState string
	Wifi         string
	DeviceID     string
	Raw          []byte
}

// parseOverlandPoints turns a batch into storable fixes, dropping only what
// cannot be placed on a map or in time.
func parseOverlandPoints(batch overlandBatch) []locationPoint {
	points := make([]locationPoint, 0, len(batch.Locations))
	for _, feature := range batch.Locations {
		if len(feature.Geometry.Coordinates) < 2 {
			continue
		}
		var properties overlandProperties
		if err := json.Unmarshal(feature.Properties, &properties); err != nil {
			continue
		}
		recordedAt, ok := parseOverlandTime(properties.Timestamp)
		if !ok {
			continue
		}

		points = append(points, locationPoint{
			RecordedAt:   recordedAt,
			Longitude:    feature.Geometry.Coordinates[0],
			Latitude:     feature.Geometry.Coordinates[1],
			Accuracy:     properties.HorizontalAccuracy,
			Altitude:     properties.Altitude,
			Speed:        properties.Speed,
			Course:       properties.Course,
			Motion:       normalizeMotion(properties.Motion),
			BatteryLevel: properties.BatteryLevel,
			BatteryState: truncateField(properties.BatteryState, 20),
			Wifi:         truncateField(properties.Wifi, 255),
			DeviceID:     truncateField(properties.DeviceID, 100),
			Raw:          feature.Properties,
		})
	}
	return points
}

// parseOverlandTime accepts the timestamp formats the app has shipped with. A
// fix with an unreadable time is useless: every question about location is a
// question about when.
func parseOverlandTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z0700", "2006-01-02 15:04:05Z07:00"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func normalizeMotion(motion []string) []string {
	cleaned := make([]string, 0, len(motion))
	for _, kind := range motion {
		kind = strings.ToLower(strings.TrimSpace(kind))
		if kind != "" {
			cleaned = append(cleaned, truncateField(kind, 20))
		}
	}
	return cleaned
}

func truncateField(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return strings.ToValidUTF8(value[:limit], "")
}

func (h *LocationWebhookHandler) storeLocationPoints(ctx context.Context, userID string, points []locationPoint) (int, error) {
	if len(points) == 0 {
		return 0, nil
	}

	batch := &pgx.Batch{}
	for _, point := range points {
		batch.Queue(`
			INSERT INTO location_points (
				user_id, source, recorded_at, latitude, longitude, accuracy_m, altitude_m,
				speed_ms, course_deg, motion, battery_level, battery_state, wifi, device_id, raw
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
			ON CONFLICT (user_id, source, recorded_at) DO NOTHING
		`, userID, locationSourceOverland, point.RecordedAt, point.Latitude, point.Longitude,
			point.Accuracy, point.Altitude, point.Speed, point.Course, point.Motion,
			point.BatteryLevel, point.BatteryState, point.Wifi, point.DeviceID, point.Raw)
	}

	results := h.db.SendBatch(ctx, batch)
	defer results.Close()

	saved := 0
	for range points {
		tag, err := results.Exec()
		if err != nil {
			return saved, err
		}
		saved += int(tag.RowsAffected())
	}
	return saved, results.Close()
}

// touchLocationSync puts the phone's push on the same freshness board as every
// pulled integration, so a tracker that stopped reporting is visible where all
// the others are.
func (h *LocationWebhookHandler) touchLocationSync(ctx context.Context, userID string) {
	if _, err := h.db.Exec(ctx, `
		INSERT INTO sync_state (source, last_synced_at, updated_at, enabled, user_id)
		VALUES ($1, NOW(), NOW(), TRUE, $2)
		ON CONFLICT (source, user_id) DO UPDATE SET
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at = NOW(),
			enabled = TRUE
	`, locationSourceOverland, userID); err != nil {
		h.logger.Warn().Err(err).Str("user_id", userID).Msg("touch location sync state")
	}
}

func (h *LocationWebhookHandler) archiveUnparsedBatch(ctx context.Context, userID string, body []byte, cause error) {
	stored, err := json.Marshal(map[string]string{"raw": string(body), "error": cause.Error()})
	if err != nil {
		return
	}
	if _, err := h.db.Exec(ctx, `
		INSERT INTO raw_events (source, event_type, payload, user_id)
		VALUES ($1, 'unparsed_batch', $2, $3)
	`, locationSourceOverland, stored, userID); err != nil {
		h.logger.Warn().Err(err).Msg("archive unparsed location batch")
	}
}
