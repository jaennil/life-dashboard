package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"life-dashboard/internal/connectors"
)

type IntegrationsHandler struct {
	db          *pgxpool.Pool
	connectors  map[string]connectors.Connector
	configured  map[string]bool
	logger      zerolog.Logger
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

var knownIntegrations = []string{"strava", "hevy", "zenmoney", "myfitnesspal", "fatsecret"}

var integrationMeta_ = map[string]integrationMeta{
	"strava": {
		displayName: "Strava",
		description: "Активности: пробежки, велосипед, плавание",
		countQuery:  "SELECT COUNT(*) FROM activities",
	},
	"hevy": {
		displayName: "Hevy",
		description: "Тренировки с упражнениями и весами",
		countQuery:  "SELECT COUNT(*) FROM workouts",
	},
	"zenmoney": {
		displayName: "ZenMoney",
		description: "Финансы: счета и транзакции",
		countQuery:  "SELECT COUNT(*) FROM transactions",
	},
	"myfitnesspal": {
		displayName: "MyFitnessPal",
		description: "Питание: дневник калорий и КБЖУ",
		countQuery:  "SELECT COUNT(*) FROM nutrition_daily",
	},
	"fatsecret": {
		displayName: "FatSecret",
		description: "Питание: дневник калорий и КБЖУ (официальный OAuth2)",
		countQuery:  "SELECT COUNT(*) FROM nutrition_daily WHERE source='fatsecret'",
	},
}

type IntegrationStatus struct {
	Name        string     `json:"name"`
	DisplayName string     `json:"display_name"`
	Description string     `json:"description"`
	Configured  bool       `json:"configured"`
	Enabled     bool       `json:"enabled"`
	LastSyncAt  *time.Time `json:"last_sync_at"`
	RecordCount int        `json:"record_count"`
}

func (h *IntegrationsHandler) GetIntegrations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type syncRow struct {
		lastSyncAt *time.Time
		enabled    bool
	}
	syncStates := map[string]syncRow{}

	rows, err := h.db.Query(ctx, `SELECT source, last_synced_at, enabled FROM sync_state`)
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

	result := make([]IntegrationStatus, 0, len(knownIntegrations))
	for _, name := range knownIntegrations {
		meta := integrationMeta_[name]
		state := syncStates[name]

		var count int
		if meta.countQuery != "" {
			h.db.QueryRow(ctx, meta.countQuery).Scan(&count)
		}

		result = append(result, IntegrationStatus{
			Name:        name,
			DisplayName: meta.displayName,
			Description: meta.description,
			Configured:  h.configured[name],
			Enabled:     state.enabled,
			LastSyncAt:  state.lastSyncAt,
			RecordCount: count,
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
	_, err := h.db.Exec(ctx, `
		INSERT INTO sync_state (source, last_synced_at, updated_at, enabled)
		VALUES ($1, NULL, NOW(), $2)
		ON CONFLICT (source) DO UPDATE SET enabled = $2, updated_at = NOW()
	`, name, body.Enabled)
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
	_, err := h.db.Exec(ctx, `
		INSERT INTO oauth_tokens (source, access_token, refresh_token, expires_at, athlete_id, updated_at)
		VALUES ('myfitnesspal', $1, $2, NOW() + INTERVAL '10 days', $3, NOW())
		ON CONFLICT (source) DO UPDATE SET
			access_token = $1, refresh_token = $2,
			expires_at = NOW() + INTERVAL '10 days',
			athlete_id = $3, updated_at = NOW()
	`, body.AccessToken, body.RefreshToken, body.UserID)
	if err != nil {
		h.logger.Error().Err(err).Msg("save mfp token")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// also mark as configured in sync_state
	h.db.Exec(ctx, `
		INSERT INTO sync_state (source, enabled, updated_at) VALUES ('myfitnesspal', true, NOW())
		ON CONFLICT (source) DO UPDATE SET enabled = true, updated_at = NOW()
	`)

	h.configured["myfitnesspal"] = true
	h.logger.Info().Str("user_id", body.UserID).Msg("mfp token saved")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// IsEnabled checks DB state for use in scheduler/sync
func IsEnabled(ctx context.Context, db *pgxpool.Pool, source string) bool {
	var enabled bool
	err := db.QueryRow(ctx, `SELECT enabled FROM sync_state WHERE source = $1`, source).Scan(&enabled)
	if err != nil {
		return true // default to enabled if no row
	}
	return enabled
}
