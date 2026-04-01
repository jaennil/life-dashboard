package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	authmw "life-dashboard/internal/middleware"
)

type HealthWebhookHandler struct {
	db     *pgxpool.Pool
	logger zerolog.Logger
}

func NewHealthWebhook(db *pgxpool.Pool, logger zerolog.Logger) *HealthWebhookHandler {
	return &HealthWebhookHandler{db: db, logger: logger.With().Str("handler", "health_webhook").Logger()}
}

type healthEntry struct {
	Type  string  `json:"type"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
	Date  string  `json:"date"`
}

type healthWebhookRequest struct {
	APIKey string        `json:"api_key"`
	Data   []healthEntry `json:"data"`
}

// POST /api/v1/webhook/health — receive health data from iOS Shortcuts
func (h *HealthWebhookHandler) ReceiveData(w http.ResponseWriter, r *http.Request) {
	bodyBytes, _ := io.ReadAll(r.Body)
	h.logger.Info().Str("raw_body", string(bodyBytes)).Msg("webhook received")

	var req healthWebhookRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		h.logger.Error().Err(err).Str("body", string(bodyBytes)).Msg("json decode failed")
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if req.APIKey == "" {
		http.Error(w, "api_key required", http.StatusUnauthorized)
		return
	}

	// Lookup user by API key
	var userID string
	err := h.db.QueryRow(r.Context(),
		`SELECT user_id FROM api_keys WHERE key = $1`, req.APIKey).Scan(&userID)
	if err != nil {
		http.Error(w, "invalid api_key", http.StatusUnauthorized)
		return
	}

	saved := 0
	for _, entry := range req.Data {
		ts, err := parseDate(entry.Date)
		if err != nil {
			h.logger.Warn().Str("date", entry.Date).Err(err).Msg("skip bad date")
			continue
		}

		unit := entry.Unit
		if unit == "" {
			switch entry.Type {
			case "steps":
				unit = "count"
			case "weight":
				unit = "kg"
			case "heart_rate", "resting_heart_rate":
				unit = "bpm"
			case "body_fat":
				unit = "%"
			}
		}

		_, err = h.db.Exec(r.Context(), `
			INSERT INTO biometrics (timestamp, source, metric_type, value, unit, user_id)
			VALUES ($1, 'apple_health', $2, $3, $4, $5)
			ON CONFLICT (user_id, timestamp, source, metric_type) DO UPDATE SET value = $3, unit = $4
		`, ts, entry.Type, entry.Value, unit, userID)
		if err != nil {
			h.logger.Warn().Err(err).Str("type", entry.Type).Msg("insert failed")
			continue
		}
		saved++
	}

	h.logger.Info().Str("user_id", userID).Int("saved", saved).Int("total", len(req.Data)).Msg("health data received")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "saved": saved})
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"api_key": key})
}

// GetAPIKey returns the existing API key
func (h *HealthWebhookHandler) GetAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(authmw.UserIDKey).(string)

	var key string
	err := h.db.QueryRow(r.Context(),
		`SELECT key FROM api_keys WHERE user_id = $1`, userID).Scan(&key)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"api_key": ""})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"api_key": key})
}

func parseDate(s string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unknown date format: %s", s)
}

func generateRandomKey(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
