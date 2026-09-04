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
	"life-dashboard/internal/observability"
	"life-dashboard/internal/syncstate"
)

type IntegrationsHandler struct {
	db         *pgxpool.Pool
	connectors map[string]connectors.Connector
	configured map[string]bool
	oauth      map[string]bool
	logger     zerolog.Logger
}

func NewIntegrations(db *pgxpool.Pool, conns []connectors.Connector, configured map[string]bool, oauthConfigured map[string]bool, logger zerolog.Logger) *IntegrationsHandler {
	m := make(map[string]connectors.Connector, len(conns))
	for _, c := range conns {
		m[c.Name()] = c
	}
	return &IntegrationsHandler{
		db:         db,
		connectors: m,
		configured: configured,
		oauth:      oauthConfigured,
		logger:     logger.With().Str("handler", "integrations").Logger(),
	}
}

type integrationMeta struct {
	displayName string
	description string
	countQuery  string
}

var knownIntegrations = []string{"strava", "hevy", "apple_health", "habitify", "todoist", "vikunja", "zenmoney", "myfitnesspal", "fatsecret", "google_calendar", "notion", "xiaomi_scale", "ios_screentime", "zepp"}

var personalIntegrations = map[string]bool{
	"strava":          true,
	"hevy":            true,
	"apple_health":    true,
	"habitify":        true,
	"todoist":         true,
	"vikunja":         true,
	"zenmoney":        true,
	"myfitnesspal":    true,
	"fatsecret":       true,
	"google_calendar": true,
	"notion":          true,
	"xiaomi_scale":    true,
	"zepp":            true,
}

var manualTokenIntegrations = map[string]bool{
	"hevy":         true,
	"habitify":     true,
	"todoist":      true,
	"vikunja":      true,
	"notion":       true,
	"zenmoney":     true,
	"xiaomi_scale": true,
	"zepp":         true,
}

// secondCredentialIntegrations need more than a token to work, and keep it in
// oauth_tokens.refresh_token: a Notion database id, a Xiaomi or Zepp login, the
// URL of a self-hosted Vikunja. A row without it is not a connected integration.
var secondCredentialIntegrations = map[string]bool{
	"notion":       true,
	"xiaomi_scale": true,
	"zepp":         true,
	"vikunja":      true,
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
	"apple_health": {
		displayName: "Apple Health",
		description: "Фактические метрики здоровья: шаги, сон, пульс, вес",
		countQuery:  "SELECT (SELECT COUNT(*) FROM biometrics WHERE source='apple_health' AND user_id = $1) + (SELECT COUNT(*) FROM sleep_sessions WHERE source='apple_health' AND user_id = $1)",
	},
	"habitify": {
		displayName: "Habitify",
		description: "Привычки и ежедневные отметки выполнения",
		countQuery:  "SELECT COUNT(*) FROM habits WHERE source='habitify' AND archived = FALSE AND user_id = $1",
	},
	"todoist": {
		displayName: "Todoist",
		description: "Задачи, recurring tasks и productivity-аналитика из Todoist",
		countQuery:  "SELECT COUNT(*) FROM tasks WHERE source='todoist' AND is_active = TRUE AND user_id = $1",
	},
	"vikunja": {
		displayName: "Vikunja",
		description: "Задачи и проекты из собственного Vikunja: чтение и создание",
		countQuery:  "SELECT COUNT(*) FROM tasks WHERE source='vikunja' AND is_active = TRUE AND user_id = $1",
	},
	"zenmoney": {
		displayName: "ZenMoney",
		description: "Финансы: счета и транзакции",
		countQuery:  "SELECT COUNT(*) FROM transactions WHERE user_id = $1",
	},
	"myfitnesspal": {
		displayName: "MyFitnessPal",
		description: "Питание: дневник калорий и КБЖУ",
		countQuery:  "SELECT COUNT(*) FROM nutrition_daily WHERE source='myfitnesspal' AND user_id = $1",
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
	"xiaomi_scale": {
		displayName: "Xiaomi Scale S400",
		description: "Состав тела: вес, жир, мышцы, вода, кости, импеданс",
		countQuery:  "SELECT COUNT(*) FROM biometrics WHERE source='xiaomi_scale' AND user_id = $1",
	},
	"zepp": {
		displayName: "Zepp / Amazfit",
		description: "Часы: сон с фазами, стресс, PAI, SpO2 из облака Zepp",
		countQuery:  "SELECT (SELECT COUNT(*) FROM biometrics WHERE source='zepp' AND user_id = $1) + (SELECT COUNT(*) FROM sleep_sessions WHERE source='zepp' AND user_id = $1)",
	},
	// Deliberately absent from personalIntegrations: the api_keys row is shared
	// with apple_health, and the disable path there deletes it. Screen time must
	// only ever flip sync_state.enabled.
	"ios_screentime": {
		displayName: "Экранное время iPhone",
		description: "Время в приложениях и на сайтах из iOS Screen Time",
		countQuery:  "SELECT COUNT(*) FROM screen_time_app_usage WHERE user_id = $1",
	},
}

type IntegrationStatus struct {
	Name            string     `json:"name"`
	DisplayName     string     `json:"display_name"`
	Description     string     `json:"description"`
	Configured      bool       `json:"configured"`
	OAuthConfigured bool       `json:"oauth_configured"`
	Enabled         bool       `json:"enabled"`
	HasCredentials  bool       `json:"has_credentials"`
	LastSyncAt      *time.Time `json:"last_sync_at"`
	RecordCount     int        `json:"record_count"`
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
				hasCredentials[source] = !secondCredentialIntegrations[source] || refreshToken != ""
			}
		}
		tokenRows.Close()
	}
	if hasStoredCredentials(ctx, h.db, appleHealthSource, userID) {
		hasCredentials[appleHealthSource] = true
	}

	result := make([]IntegrationStatus, 0, len(knownIntegrations))
	for _, name := range knownIntegrations {
		meta := integrationMeta_[name]
		state, ok := syncStates[name]

		var count int
		if meta.countQuery != "" {
			h.db.QueryRow(ctx, meta.countQuery, userID).Scan(&count)
		}

		enabled := ok && state.enabled
		if enabled && personalIntegrations[name] && !hasIntegrationActivationState(ctx, h.db, name, userID) {
			enabled = false
		}
		if !ok && (hasCredentials[name] || count > 0) {
			enabled = true
		}

		result = append(result, IntegrationStatus{
			Name:            name,
			DisplayName:     meta.displayName,
			Description:     meta.description,
			Configured:      h.configured[name],
			OAuthConfigured: h.oauth[name],
			Enabled:         enabled,
			HasCredentials:  hasCredentials[name],
			LastSyncAt:      state.lastSyncAt,
			RecordCount:     count,
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
	if body.Enabled && personalIntegrations[name] && !hasIntegrationActivationState(ctx, h.db, name, userID) {
		http.Error(w, "integration is not connected", http.StatusConflict)
		return
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		h.logger.Error().Err(err).Str("name", name).Msg("begin toggle integration")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	credentialsDeleted := false
	if !body.Enabled && personalIntegrations[name] {
		if name == appleHealthSource {
			if _, err := tx.Exec(ctx, `
				DELETE FROM api_keys
				WHERE user_id = $1
			`, userID); err != nil {
				h.logger.Error().Err(err).Str("name", name).Msg("delete apple health api key")
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			credentialsDeleted = true
		} else {
			if _, err := tx.Exec(ctx, `
				DELETE FROM oauth_tokens
				WHERE source = $1 AND user_id = $2
			`, name, userID); err != nil {
				h.logger.Error().Err(err).Str("name", name).Msg("delete integration credentials")
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			credentialsDeleted = true
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO sync_state (source, last_synced_at, updated_at, enabled, user_id)
		VALUES ($1, NULL, NOW(), $2, $3)
		ON CONFLICT (source, user_id) DO UPDATE SET enabled = $2, updated_at = NOW()
	`, name, body.Enabled, userID)
	if err != nil {
		h.logger.Error().Err(err).Str("name", name).Msg("toggle integration")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		h.logger.Error().Err(err).Str("name", name).Msg("commit toggle integration")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.logger.Info().Str("name", name).Bool("enabled", body.Enabled).Bool("credentials_deleted", credentialsDeleted).Msg("integration toggled")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"name": name, "enabled": body.Enabled, "credentials_deleted": credentialsDeleted})
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
	json.NewEncoder(w).Encode(connectionResponse("myfitnesspal", h.runInitialSync("myfitnesspal", userID)))
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
		AccountID  string `json:"account_id,omitempty"`
		BaseURL    string `json:"base_url,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
		http.Error(w, "token is required", http.StatusBadRequest)
		return
	}
	if name == "notion" && body.DatabaseID == "" {
		http.Error(w, "database_id is required", http.StatusBadRequest)
		return
	}
	if name == "xiaomi_scale" && body.AccountID == "" {
		http.Error(w, "account_id is required", http.StatusBadRequest)
		return
	}
	if name == "zepp" && body.AccountID == "" {
		http.Error(w, "account_id is required", http.StatusBadRequest)
		return
	}
	if name == "vikunja" && body.BaseURL == "" {
		http.Error(w, "base_url is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)
	// refresh_token doubles as the integration's second credential.
	refreshToken := ""
	switch name {
	case "notion":
		refreshToken = body.DatabaseID
	case "xiaomi_scale":
		refreshToken = body.AccountID
	case "zepp":
		// token is the Zepp password, account_id the login.
		refreshToken = body.AccountID
	case "vikunja":
		// Self-hosted, so the instance to talk to is part of the credentials.
		refreshToken = body.BaseURL
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
	json.NewEncoder(w).Encode(connectionResponse(name, h.runInitialSync(name, userID)))
}

// IsEnabled checks DB state for use in scheduler/sync
func IsEnabled(ctx context.Context, db *pgxpool.Pool, source string, userID string) bool {
	var enabled bool
	err := db.QueryRow(ctx, `SELECT enabled FROM sync_state WHERE source = $1 AND user_id = $2`, source, userID).Scan(&enabled)
	if err != nil {
		return hasIntegrationActivationState(ctx, db, source, userID)
	}
	if enabled && personalIntegrations[source] && !hasIntegrationActivationState(ctx, db, source, userID) {
		return false
	}
	return enabled
}

func hasIntegrationActivationState(ctx context.Context, db *pgxpool.Pool, source string, userID string) bool {
	if !personalIntegrations[source] {
		return false
	}
	return hasStoredCredentials(ctx, db, source, userID)
}

func hasStoredCredentials(ctx context.Context, db *pgxpool.Pool, source string, userID string) bool {
	if source == appleHealthSource {
		var exists int
		if err := db.QueryRow(ctx, `SELECT 1 FROM api_keys WHERE user_id = $1 LIMIT 1`, userID).Scan(&exists); err != nil {
			return false
		}
		return true
	}

	query := `SELECT 1 FROM oauth_tokens WHERE source = $1 AND user_id = $2 LIMIT 1`
	args := []any{source, userID}
	if secondCredentialIntegrations[source] {
		query = `SELECT 1 FROM oauth_tokens WHERE source = $1 AND user_id = $2 AND refresh_token <> '' LIMIT 1`
	}

	var exists int
	if err := db.QueryRow(ctx, query, args...).Scan(&exists); err != nil {
		return false
	}
	return true
}

func (h *IntegrationsHandler) runInitialSync(source string, userID string) error {
	conn, ok := h.connectors[source]
	if !ok {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	h.logger.Info().Str("source", source).Str("user_id", userID).Msg("running initial sync inline")
	if err := observability.RunSync(ctx, source, observability.SyncTriggerInitial, func(ctx context.Context) error {
		return conn.Sync(ctx, userID)
	}); err != nil {
		if recordErr := syncstate.RecordSyncFailure(context.Background(), h.db, source, userID, time.Now()); recordErr != nil {
			h.logger.Warn().Err(recordErr).Str("source", source).Str("user_id", userID).Msg("record initial sync failure")
		}
		h.logger.Error().Err(err).Str("source", source).Str("user_id", userID).Msg("initial sync failed")
		return err
	}
	if err := syncstate.RecordSyncSuccess(context.Background(), h.db, source, userID, time.Now()); err != nil {
		h.logger.Warn().Err(err).Str("source", source).Str("user_id", userID).Msg("record initial sync success")
	}

	h.logger.Info().Str("source", source).Str("user_id", userID).Msg("initial sync complete")
	return nil
}

func connectionResponse(source string, syncErr error) map[string]any {
	resp := map[string]any{
		"status": "ok",
		"source": source,
	}
	if syncErr != nil {
		resp["status"] = "error"
		resp["sync_error"] = syncErr.Error()
		return resp
	}
	resp["sync_completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	return resp
}
