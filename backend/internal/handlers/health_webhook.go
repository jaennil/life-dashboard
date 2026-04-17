package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	authmw "life-dashboard/internal/middleware"
)

const appleHealthSource = "apple_health"

type HealthWebhookHandler struct {
	db     *pgxpool.Pool
	logger zerolog.Logger
}

func NewHealthWebhook(db *pgxpool.Pool, logger zerolog.Logger) *HealthWebhookHandler {
	return &HealthWebhookHandler{db: db, logger: logger.With().Str("handler", "health_webhook").Logger()}
}

type healthEntry struct {
	Type      string         `json:"type"`
	Metric    string         `json:"metric"`
	Value     float64        `json:"value"`
	Unit      string         `json:"unit"`
	Date      string         `json:"date"`
	Timestamp string         `json:"timestamp"`
	StartDate string         `json:"start_date"`
	EndDate   string         `json:"end_date"`
	Source    string         `json:"source"`
	Metadata  map[string]any `json:"metadata"`
}

type healthSleepStage struct {
	Stage     string `json:"stage"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type healthSleepSession struct {
	Date              string             `json:"date"`
	StartDate         string             `json:"start_date"`
	EndDate           string             `json:"end_date"`
	TotalSleepMinutes int                `json:"total_sleep_minutes"`
	TotalMinutes      int                `json:"total_minutes"`
	DeepSleepMinutes  int                `json:"deep_sleep_minutes"`
	LightSleepMinutes int                `json:"light_sleep_minutes"`
	REMSleepMinutes   int                `json:"rem_sleep_minutes"`
	AwakeMinutes      int                `json:"awake_minutes"`
	SleepScore        int                `json:"sleep_score"`
	AvgHRV            float64            `json:"avg_hrv"`
	AvgRestingHR      int                `json:"avg_resting_hr"`
	Stages            []healthSleepStage `json:"stages"`
	Metadata          map[string]any     `json:"metadata"`
}

type healthWebhookRequest struct {
	APIKey  string               `json:"api_key"`
	Source  string               `json:"source"`
	Data    []healthEntry        `json:"data"`
	Metrics []healthEntry        `json:"metrics"`
	Sleep   []healthSleepSession `json:"sleep"`
	Sleeps  []healthSleepSession `json:"sleeps"`
}

type healthAPIKeyResponse struct {
	APIKey      string     `json:"api_key"`
	WebhookURL  string     `json:"webhook_url"`
	LastSyncAt  *time.Time `json:"last_sync_at"`
	MetricCount int        `json:"metric_count"`
	SleepCount  int        `json:"sleep_count"`
}

// POST /api/v1/webhook/health — receive health data from iOS Shortcuts
func (h *HealthWebhookHandler) ReceiveData(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Fix common Shortcuts issues: newlines inside values, trailing commas
	cleaned := strings.ReplaceAll(string(bodyBytes), "\n", "")
	cleaned = strings.ReplaceAll(cleaned, "\r", "")

	var req healthWebhookRequest
	if err := json.Unmarshal([]byte(cleaned), &req); err != nil {
		h.logger.Error().Err(err).Str("body", cleaned).Msg("json decode failed")
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	apiKey := healthAPIKeyFromRequest(r, req.APIKey)
	if apiKey == "" {
		http.Error(w, "api_key required", http.StatusUnauthorized)
		return
	}

	// Lookup user by API key
	var userID string
	err = h.db.QueryRow(r.Context(),
		`SELECT user_id FROM api_keys WHERE key = $1`, apiKey).Scan(&userID)
	if err != nil {
		http.Error(w, "invalid api_key", http.StatusUnauthorized)
		return
	}

	entries := append([]healthEntry{}, req.Data...)
	entries = append(entries, req.Metrics...)
	sleepSessions := append([]healthSleepSession{}, req.Sleep...)
	sleepSessions = append(sleepSessions, req.Sleeps...)

	source := normalizeHealthSource(req.Source)
	rawPayload, _ := json.Marshal(req)
	_, _ = h.db.Exec(r.Context(), `
		INSERT INTO raw_events (source, event_type, external_id, payload, user_id)
		VALUES ($1, 'webhook', $2, $3, $4)
	`, source, time.Now().UTC().Format(time.RFC3339Nano), rawPayload, userID)

	savedMetrics := 0
	skippedMetrics := 0
	for _, entry := range entries {
		metricType := normalizeHealthMetricType(firstNonEmpty(entry.Type, entry.Metric))
		if metricType == "" {
			skippedMetrics++
			continue
		}

		ts, err := parseHealthEntryTime(entry)
		if err != nil {
			h.logger.Warn().Str("metric_type", metricType).Err(err).Msg("skip health metric with bad timestamp")
			skippedMetrics++
			continue
		}

		unit := normalizeHealthUnit(metricType, entry.Unit)
		metadata, _ := json.Marshal(entry.Metadata)
		if len(metadata) == 0 || string(metadata) == "null" {
			metadata = []byte("{}")
		}

		_, err = h.db.Exec(r.Context(), `
			INSERT INTO biometrics (timestamp, source, metric_type, value, unit, metadata, user_id)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
			ON CONFLICT (user_id, timestamp, source, metric_type) DO UPDATE SET
				value = EXCLUDED.value,
				unit = EXCLUDED.unit,
				metadata = EXCLUDED.metadata
		`, ts, source, metricType, entry.Value, unit, metadata, userID)
		if err != nil {
			h.logger.Warn().Err(err).Str("metric_type", metricType).Msg("insert health metric failed")
			skippedMetrics++
			continue
		}
		savedMetrics++
	}

	savedSleep := 0
	skippedSleep := 0
	for _, session := range sleepSessions {
		if err := h.upsertSleepSession(r.Context(), userID, source, session); err != nil {
			h.logger.Warn().Err(err).Msg("insert health sleep session failed")
			skippedSleep++
			continue
		}
		savedSleep++
	}

	_, err = h.db.Exec(r.Context(), `
		INSERT INTO sync_state (source, last_synced_at, updated_at, enabled, user_id)
		VALUES ($1, NOW(), NOW(), TRUE, $2)
		ON CONFLICT (source, user_id) DO UPDATE SET
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at = NOW(),
			enabled = TRUE
	`, appleHealthSource, userID)
	if err != nil {
		h.logger.Warn().Err(err).Msg("update apple health sync state failed")
	}

	h.logger.Info().
		Str("user_id", userID).
		Int("metrics_saved", savedMetrics).
		Int("metrics_skipped", skippedMetrics).
		Int("sleep_saved", savedSleep).
		Int("sleep_skipped", skippedSleep).
		Msg("health data received")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":          "ok",
		"metrics_saved":   savedMetrics,
		"metrics_skipped": skippedMetrics,
		"sleep_saved":     savedSleep,
		"sleep_skipped":   skippedSleep,
	})
}

// GenerateAPIKey creates a new API key for the user
func (h *HealthWebhookHandler) GenerateAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(authmw.UserIDKey).(string)

	// Generate random key
	key := generateRandomKey(32)

	_, err := h.db.Exec(r.Context(), `
		INSERT INTO api_keys (user_id, key, created_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (user_id) DO UPDATE SET key = $2, created_at = NOW()
	`, userID, key)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	_, _ = h.db.Exec(r.Context(), `
		INSERT INTO sync_state (source, enabled, updated_at, user_id)
		VALUES ($1, TRUE, NOW(), $2)
		ON CONFLICT (source, user_id) DO UPDATE SET enabled = TRUE, updated_at = NOW()
	`, appleHealthSource, userID)

	h.writeAPIKeyResponse(w, r, userID, key)
}

// GetAPIKey returns the existing API key
func (h *HealthWebhookHandler) GetAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(authmw.UserIDKey).(string)

	var key string
	err := h.db.QueryRow(r.Context(),
		`SELECT key FROM api_keys WHERE user_id = $1`, userID).Scan(&key)
	if err != nil && err != pgx.ErrNoRows {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err == pgx.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(healthAPIKeyResponse{WebhookURL: healthWebhookURL(r)})
		return
	}

	h.writeAPIKeyResponse(w, r, userID, key)
}

func (h *HealthWebhookHandler) writeAPIKeyResponse(w http.ResponseWriter, r *http.Request, userID, key string) {
	var resp healthAPIKeyResponse
	resp.APIKey = key
	resp.WebhookURL = healthWebhookURL(r)
	_ = h.db.QueryRow(r.Context(), `
		SELECT last_synced_at
		FROM sync_state
		WHERE source = $1 AND user_id = $2
	`, appleHealthSource, userID).Scan(&resp.LastSyncAt)
	_ = h.db.QueryRow(r.Context(), `
		SELECT COUNT(*)
		FROM biometrics
		WHERE user_id = $1 AND source = $2
	`, userID, appleHealthSource).Scan(&resp.MetricCount)
	_ = h.db.QueryRow(r.Context(), `
		SELECT COUNT(*)
		FROM sleep_sessions
		WHERE user_id = $1 AND source = $2
	`, userID, appleHealthSource).Scan(&resp.SleepCount)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *HealthWebhookHandler) upsertSleepSession(ctx context.Context, userID, source string, session healthSleepSession) error {
	sleepStart, err := parseDate(firstNonEmpty(session.StartDate, session.Date))
	if err != nil {
		return err
	}

	var sleepEnd *time.Time
	if strings.TrimSpace(session.EndDate) != "" {
		end, err := parseDate(session.EndDate)
		if err != nil {
			return err
		}
		sleepEnd = &end
	}

	sessionDate := sleepStart
	if strings.TrimSpace(session.Date) != "" {
		if parsedDate, err := parseDate(session.Date); err == nil {
			sessionDate = parsedDate
		}
	}

	totalMinutes := firstPositiveInt(session.TotalSleepMinutes, session.TotalMinutes)
	if totalMinutes == 0 && sleepEnd != nil {
		totalMinutes = int(sleepEnd.Sub(sleepStart).Minutes())
	}

	rawPayload, _ := json.Marshal(session)
	var sleepID string
	err = h.db.QueryRow(ctx, `
		INSERT INTO sleep_sessions (
			user_id, source, date, sleep_start, sleep_end, total_sleep_minutes,
			deep_sleep_minutes, light_sleep_minutes, rem_sleep_minutes, awake_minutes,
			sleep_score, avg_hrv, avg_resting_hr, raw_payload
		)
		VALUES ($1, $2, $3::date, $4, $5, NULLIF($6, 0), NULLIF($7, 0), NULLIF($8, 0), NULLIF($9, 0), NULLIF($10, 0), NULLIF($11, 0), NULLIF($12, 0), NULLIF($13, 0), $14::jsonb)
		ON CONFLICT (user_id, source, date) DO UPDATE SET
			sleep_start = EXCLUDED.sleep_start,
			sleep_end = EXCLUDED.sleep_end,
			total_sleep_minutes = EXCLUDED.total_sleep_minutes,
			deep_sleep_minutes = EXCLUDED.deep_sleep_minutes,
			light_sleep_minutes = EXCLUDED.light_sleep_minutes,
			rem_sleep_minutes = EXCLUDED.rem_sleep_minutes,
			awake_minutes = EXCLUDED.awake_minutes,
			sleep_score = EXCLUDED.sleep_score,
			avg_hrv = EXCLUDED.avg_hrv,
			avg_resting_hr = EXCLUDED.avg_resting_hr,
			raw_payload = EXCLUDED.raw_payload
		RETURNING id
	`, userID, source, sessionDate, sleepStart, sleepEnd, totalMinutes,
		session.DeepSleepMinutes, session.LightSleepMinutes, session.REMSleepMinutes, session.AwakeMinutes,
		session.SleepScore, session.AvgHRV, session.AvgRestingHR, rawPayload).Scan(&sleepID)
	if err != nil {
		return err
	}

	if len(session.Stages) == 0 {
		return nil
	}

	if _, err := h.db.Exec(ctx, `DELETE FROM sleep_stages WHERE session_id = $1`, sleepID); err != nil {
		return err
	}
	for _, stage := range session.Stages {
		startedAt, err := parseDate(stage.StartDate)
		if err != nil {
			continue
		}
		endedAt, err := parseDate(stage.EndDate)
		if err != nil {
			continue
		}
		stageName := normalizeSleepStage(stage.Stage)
		if stageName == "" {
			continue
		}
		_, _ = h.db.Exec(ctx, `
			INSERT INTO sleep_stages (session_id, started_at, ended_at, stage)
			VALUES ($1, $2, $3, $4)
		`, sleepID, startedAt, endedAt, stageName)
	}
	return nil
}

func parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02",
		"Jan 2, 2006 at 3:04 PM",
		"Jan 2, 2006 at 3:04:05 PM",
		"January 2, 2006 at 3:04 PM",
		"2 Jan 2006",
		"02.01.2006",
		"01/02/2006",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unknown date format: %s", s)
}

func parseHealthEntryTime(entry healthEntry) (time.Time, error) {
	return parseDate(firstNonEmpty(entry.Timestamp, entry.Date, entry.StartDate, entry.EndDate))
}

func generateRandomKey(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func healthAPIKeyFromRequest(r *http.Request, bodyKey string) string {
	if bodyKey = strings.TrimSpace(bodyKey); bodyKey != "" {
		return bodyKey
	}
	if key := strings.TrimSpace(r.Header.Get("X-API-Key")); key != "" {
		return key
	}
	if auth := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("Bearer "):])
	}
	return strings.TrimSpace(r.URL.Query().Get("api_key"))
}

func healthWebhookURL(r *http.Request) string {
	proto := firstNonEmpty(r.Header.Get("X-Forwarded-Proto"), "https")
	host := firstNonEmpty(r.Header.Get("X-Forwarded-Host"), r.Host)
	if host == "" {
		host = "lifedash.dubrovskih.ru"
	}
	return proto + "://" + host + "/api/v1/webhook/health"
}

func normalizeHealthSource(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	source = strings.ReplaceAll(source, "-", "_")
	source = strings.ReplaceAll(source, " ", "_")
	if source == "" || source == "health" || source == "zepp" {
		return appleHealthSource
	}
	if len(source) > 30 {
		return source[:30]
	}
	return source
}

func normalizeHealthMetricType(metricType string) string {
	metricType = strings.ToLower(strings.TrimSpace(metricType))
	metricType = strings.ReplaceAll(metricType, "-", "_")
	metricType = strings.ReplaceAll(metricType, " ", "_")
	switch metricType {
	case "step_count", "steps_count", "stepcount":
		return "steps"
	case "body_mass", "body_weight", "weight_kg":
		return "weight"
	case "heartrate", "heart_rate_bpm":
		return "heart_rate"
	case "resting_heartrate", "resting_heart_rate_bpm":
		return "resting_heart_rate"
	case "walking_running_distance", "distance_walking_running", "distance":
		return "walking_running_distance"
	case "active_calories", "active_energy_burned", "calories":
		return "active_energy"
	case "body_fat_percentage":
		return "body_fat"
	case "oxygen_saturation", "blood_oxygen", "spo2_percent":
		return "spo2"
	case "heart_rate_variability", "hrv_sdnn":
		return "hrv"
	case "vo2_max":
		return "vo2max"
	}
	return metricType
}

func normalizeHealthUnit(metricType, unit string) string {
	if unit = strings.TrimSpace(unit); unit != "" {
		return unit
	}
	switch metricType {
	case "steps", "flights_climbed":
		return "count"
	case "weight":
		return "kg"
	case "heart_rate", "resting_heart_rate", "respiratory_rate":
		return "bpm"
	case "body_fat", "spo2":
		return "%"
	case "walking_running_distance":
		return "m"
	case "active_energy":
		return "kcal"
	case "hrv":
		return "ms"
	case "vo2max":
		return "ml/kg/min"
	default:
		return ""
	}
}

func normalizeSleepStage(stage string) string {
	stage = strings.ToLower(strings.TrimSpace(stage))
	stage = strings.ReplaceAll(stage, " ", "_")
	switch stage {
	case "asleep_deep", "deep_sleep":
		return "deep"
	case "asleep_rem", "rem_sleep":
		return "rem"
	case "asleep_core", "asleep_light", "light_sleep", "core":
		return "light"
	case "awake", "in_bed":
		return stage
	default:
		return stage
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
