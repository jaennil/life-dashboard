package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"life-dashboard/internal/connectors"
	authmw "life-dashboard/internal/middleware"
)

type IntegrationsHandler struct {
	db         *pgxpool.Pool
	connectors map[string]connectors.Connector
	configured map[string]bool
	logger     zerolog.Logger
}

func NewIntegrations(db *pgxpool.Pool, conns []connectors.Connector, configured map[string]bool, logger zerolog.Logger) *IntegrationsHandler {
	m := make(map[string]connectors.Connector, len(conns))
	for _, c := range conns {
		m[c.Name()] = c
	}
	return &IntegrationsHandler{
		db:         db,
		connectors: m,
		configured: configured,
		logger:     logger.With().Str("handler", "integrations").Logger(),
	}
}

type integrationMeta struct {
	displayName string
	description string
	countQuery  string
}

var knownIntegrations = []string{"strava", "hevy", "zenmoney", "myfitnesspal", "fatsecret", "google_calendar", "notion"}

var manualTokenIntegrations = map[string]bool{
	"hevy":     true,
	"notion":   true,
	"zenmoney": true,
}

var integrationMeta_ = map[string]integrationMeta{
	"strava": {
		displayName: "Strava",
		description: "Активности: пробежки, велосипед, плавание",
		countQuery:  "SELECT COUNT(*) FROM activities WHERE user_id = $1",
	},
	"hevy": {
		displayName: "Hevy",
		description: "Тренировки с упражнениями и весами",
		countQuery:  "SELECT COUNT(*) FROM workouts WHERE user_id = $1",
	},
	"zenmoney": {
		displayName: "ZenMoney",
		description: "Финансы: счета и транзакции",
		countQuery:  "SELECT COUNT(*) FROM transactions WHERE user_id = $1",
	},
	"myfitnesspal": {
		displayName: "MyFitnessPal",
		description: "Питание: дневник калорий и КБЖУ",
		countQuery:  "SELECT COUNT(*) FROM nutrition_daily WHERE user_id = $1",
	},
	"fatsecret": {
		displayName: "FatSecret",
		description: "Питание: дневник калорий и КБЖУ (официальный OAuth2)",
		countQuery:  "SELECT COUNT(*) FROM nutrition_daily WHERE source='fatsecret' AND user_id = $1",
	},
	"google_calendar": {
		displayName: "Google Calendar",
		description: "События и встречи из Google Календаря",
		countQuery:  "SELECT COUNT(*) FROM calendar_events WHERE user_id = $1",
	},
	"notion": {
		displayName: "Notion Journal",
		description: "Личный дневник из Notion",
		countQuery:  "SELECT COUNT(*) FROM journal_entries WHERE source='notion' AND user_id = $1",
	},
}

type IntegrationStatus struct {
	Name           string     `json:"name"`
	DisplayName    string     `json:"display_name"`
	Description    string     `json:"description"`
	Configured     bool       `json:"configured"`
	Enabled        bool       `json:"enabled"`
	HasCredentials bool       `json:"has_credentials"`
	LastSyncAt     *time.Time `json:"last_sync_at"`
	RecordCount    int        `json:"record_count"`
}

func (h *IntegrationsHandler) GetIntegrations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)

	type syncRow struct {
		lastSyncAt *time.Time
		enabled    bool
	}
	syncStates := map[string]syncRow{}
	hasCredentials := map[string]bool{}

	rows, err := h.db.Query(ctx, `SELECT source, last_synced_at, enabled FROM sync_state WHERE user_id = $1`, userID)
	if err == nil {
		for rows.Next() {
			var source string
			var row syncRow
			if err := rows.Scan(&source, &row.lastSyncAt, &row.enabled); err == nil {
				syncStates[source] = row
			}
		}
		rows.Close()
	}

	tokenRows, err := h.db.Query(ctx, `SELECT source, refresh_token FROM oauth_tokens WHERE user_id = $1`, userID)
	if err == nil {
		for tokenRows.Next() {
			var source string
			var refreshToken string
			if err := tokenRows.Scan(&source, &refreshToken); err == nil {
				hasCredentials[source] = source != "notion" || refreshToken != ""
			}
		}
		tokenRows.Close()
	}

	result := make([]IntegrationStatus, 0, len(knownIntegrations))
	for _, name := range knownIntegrations {
		meta := integrationMeta_[name]
		state, ok := syncStates[name]
		enabled := true
		if ok {
			enabled = state.enabled
		}

		var count int
		if meta.countQuery != "" {
			h.db.QueryRow(ctx, meta.countQuery, userID).Scan(&count)
		}

		result = append(result, IntegrationStatus{
			Name:           name,
			DisplayName:    meta.displayName,
			Description:    meta.description,
			Configured:     h.configured[name],
			Enabled:        enabled,
			HasCredentials: hasCredentials[name],
			LastSyncAt:     state.lastSyncAt,
			RecordCount:    count,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *IntegrationsHandler) ToggleIntegration(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := integrationMeta_[name]; !ok {
		http.Error(w, "unknown integration", http.StatusNotFound)
		return
	}

	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	_, err := h.db.Exec(ctx, `
		INSERT INTO sync_state (source, last_synced_at, updated_at, enabled, user_id)
		VALUES ($1, NULL, NOW(), $2, $3)
		ON CONFLICT (source, user_id) DO UPDATE SET enabled = $2, updated_at = NOW()
	`, name, body.Enabled, userID)
	if err != nil {
		h.logger.Error().Err(err).Str("name", name).Msg("toggle integration")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.logger.Info().Str("name", name).Bool("enabled", body.Enabled).Msg("integration toggled")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"name": name, "enabled": body.Enabled})
}

func (h *IntegrationsHandler) SaveMFPToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		UserID       string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AccessToken == "" || body.RefreshToken == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	_, err := h.db.Exec(ctx, `
		INSERT INTO oauth_tokens (source, access_token, refresh_token, expires_at, athlete_id, updated_at, user_id)
		VALUES ('myfitnesspal', $1, $2, NOW() + INTERVAL '10 days', $3, NOW(), $4)
		ON CONFLICT (source, user_id) DO UPDATE SET
			access_token = $1, refresh_token = $2,
			expires_at = NOW() + INTERVAL '10 days',
			athlete_id = $3, updated_at = NOW()
	`, body.AccessToken, body.RefreshToken, body.UserID, userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("save mfp token")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// also mark as configured in sync_state
	h.db.Exec(ctx, `
		INSERT INTO sync_state (source, enabled, updated_at, user_id) VALUES ('myfitnesspal', true, NOW(), $1)
		ON CONFLICT (source, user_id) DO UPDATE SET enabled = true, updated_at = NOW()
	`, userID)

	h.configured["myfitnesspal"] = true
	h.logger.Info().Str("user_id", body.UserID).Msg("mfp token saved")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// SaveToken saves a manually-entered access token for integrations like ZenMoney
func (h *IntegrationsHandler) SaveToken(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !manualTokenIntegrations[name] {
		http.Error(w, "unsupported integration", http.StatusNotFound)
		return
	}

	var body struct {
		Token      string `json:"token"`
		DatabaseID string `json:"database_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
		http.Error(w, "token is required", http.StatusBadRequest)
		return
	}
	if name == "notion" && body.DatabaseID == "" {
		http.Error(w, "database_id is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)
	refreshToken := ""
	if name == "notion" {
		refreshToken = body.DatabaseID
	}

	_, err := h.db.Exec(ctx, `
		INSERT INTO oauth_tokens (source, access_token, refresh_token, expires_at, updated_at, user_id)
		VALUES ($1, $2, $3, NOW() + INTERVAL '100 years', NOW(), $4)
		ON CONFLICT (source, user_id) DO UPDATE SET
			access_token = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			expires_at = EXCLUDED.expires_at,
			updated_at = NOW()
	`, name, body.Token, refreshToken, userID)
	if err != nil {
		h.logger.Error().Err(err).Str("source", name).Msg("save token failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.db.Exec(ctx, `
		INSERT INTO sync_state (source, enabled, updated_at, user_id) VALUES ($1, true, NOW(), $2)
		ON CONFLICT (source, user_id) DO UPDATE SET enabled = true, updated_at = NOW()
	`, name, userID)

	h.logger.Info().Str("source", name).Str("user_id", userID).Msg("token saved")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// IsEnabled checks DB state for use in scheduler/sync
func IsEnabled(ctx context.Context, db *pgxpool.Pool, source string, userID string) bool {
	var enabled bool
	err := db.QueryRow(ctx, `SELECT enabled FROM sync_state WHERE source = $1 AND user_id = $2`, source, userID).Scan(&enabled)
	if err != nil {
		return true // default to enabled if no row
	}
	return enabled
}
