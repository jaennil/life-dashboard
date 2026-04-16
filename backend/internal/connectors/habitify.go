package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	habitifyBaseURL             = "https://api.habitify.me"
	habitifyInitialBackfillDays = 60
	habitifyIncrementalLookback = 7
	habitifyRequestTimeout      = 30 * time.Second
)

type habitifyEnvelope struct {
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Version string          `json:"version"`
	Status  bool            `json:"status"`
}

type habitifyDB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type HabitifyConnector struct {
	db     *pgxpool.Pool
	client *http.Client
	logger zerolog.Logger
}

func NewHabitify(db *pgxpool.Pool, logger zerolog.Logger) *HabitifyConnector {
	return &HabitifyConnector{
		db:     db,
		client: &http.Client{Timeout: habitifyRequestTimeout},
		logger: logger.With().Str("connector", "habitify").Logger(),
	}
}

func (h *HabitifyConnector) Name() string { return "habitify" }

func (h *HabitifyConnector) Sync(ctx context.Context, userID string) error {
	apiKey, err := h.loadAPIKey(ctx, userID)
	if err != nil {
		return err
	}

	habits, err := h.fetchList(ctx, apiKey, "/habits", nil)
	if err != nil {
		return fmt.Errorf("fetch habits: %w", err)
	}
	for _, raw := range habits {
		if _, err := h.upsertHabit(ctx, h.db, userID, raw); err != nil {
			return fmt.Errorf("upsert habit: %w", err)
		}
	}

	lastSync, err := h.getLastSync(ctx, userID)
	if err != nil {
		return fmt.Errorf("get last sync: %w", err)
	}

	now := time.Now()
	start := h.journalSyncStart(lastSync, now)
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	totalDays := 0
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if err := h.syncJournalDay(ctx, userID, apiKey, day); err != nil {
			return fmt.Errorf("sync journal %s: %w", day.Format("2006-01-02"), err)
		}
		totalDays++
	}

	if err := h.updateLastSync(ctx, userID); err != nil {
		return fmt.Errorf("update last sync: %w", err)
	}

	h.logger.Info().Int("days", totalDays).Int("habits", len(habits)).Msg("habitify sync complete")
	return nil
}

func (h *HabitifyConnector) loadAPIKey(ctx context.Context, userID string) (string, error) {
	var key string
	err := h.db.QueryRow(ctx, `SELECT access_token FROM oauth_tokens WHERE source = 'habitify' AND user_id = $1`, userID).Scan(&key)
	if err != nil {
		return "", fmt.Errorf("no API key — add your Habitify API key in Settings")
	}
	return key, nil
}

func (h *HabitifyConnector) journalSyncStart(lastSync time.Time, now time.Time) time.Time {
	if lastSync.IsZero() {
		start := now.AddDate(0, 0, -(habitifyInitialBackfillDays - 1))
		return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	}

	start := lastSync.AddDate(0, 0, -habitifyIncrementalLookback)
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
}

func (h *HabitifyConnector) fetchList(ctx context.Context, apiKey, path string, params url.Values) ([]json.RawMessage, error) {
	env, err := h.doRequest(ctx, apiKey, path, params)
	if err != nil {
		return nil, err
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil, nil
	}

	var items []json.RawMessage
	if err := json.Unmarshal(env.Data, &items); err != nil {
		return nil, fmt.Errorf("decode list: %w", err)
	}
	return items, nil
}

func (h *HabitifyConnector) fetchJournalDay(ctx context.Context, apiKey string, day time.Time) ([]json.RawMessage, error) {
	params := url.Values{}
	params.Set("target_date", day.Format("2006-01-02"))
	params.Set("order_by", "status")
	return h.fetchList(ctx, apiKey, "/journal", params)
}

func (h *HabitifyConnector) doRequest(ctx context.Context, apiKey, path string, params url.Values) (*habitifyEnvelope, error) {
	u, err := url.Parse(habitifyBaseURL + path)
	if err != nil {
		return nil, err
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", apiKey)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("habitify api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var env habitifyEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if !env.Status {
		return nil, fmt.Errorf("habitify api error: %s", strings.TrimSpace(env.Message))
	}
	return &env, nil
}

func (h *HabitifyConnector) syncJournalDay(ctx context.Context, userID, apiKey string, day time.Time) error {
	entries, err := h.fetchJournalDay(ctx, apiKey, day)
	if err != nil {
		return err
	}

	tx, err := h.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM habit_daily_statuses s
		USING habits h
		WHERE s.habit_id = h.id
			AND h.user_id = $1
			AND h.source = 'habitify'
			AND s.target_date = $2
	`, userID, day.Format("2006-01-02")); err != nil {
		return fmt.Errorf("clear day statuses: %w", err)
	}

	for _, raw := range entries {
		habitID, err := h.upsertHabit(ctx, tx, userID, raw)
		if err != nil {
			return fmt.Errorf("upsert journal habit: %w", err)
		}
		if err := h.upsertDailyStatus(ctx, tx, habitID, userID, day, raw); err != nil {
			return fmt.Errorf("upsert daily status: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (h *HabitifyConnector) upsertHabit(ctx context.Context, db habitifyDB, userID string, raw json.RawMessage) (string, error) {
	payload, err := decodeJSONObject(raw)
	if err != nil {
		return "", err
	}

	externalID := jsonString(payload, "id")
	name := jsonString(payload, "name")
	if externalID == "" || name == "" {
		return "", fmt.Errorf("habit payload missing id/name")
	}

	area := jsonObject(payload, "area")
	areaExternalID := jsonString(area, "id")
	areaName := jsonString(area, "name")
	archived := jsonBool(payload, "is_archived", "isArchived", "archived")
	recurrence := jsonString(payload, "recurrence")
	logMethod := jsonString(payload, "log_method", "logMethod")
	timeOfDay := jsonStringSlice(payload, "time_of_day", "timeOfDay")
	remindAt := jsonStringSlice(payload, "remind")
	sourceCreatedAt := jsonTime(payload, "created_date", "createdAt")
	goalJSON := jsonSubdocument(payload, "goal")
	goalHistoryJSON := jsonSubdocument(payload, "goal_history_items", "goalHistoryItems")

	if _, err := db.Exec(ctx, `
		INSERT INTO raw_events (source, event_type, external_id, payload, user_id)
		VALUES ('habitify', 'habit', $1, $2, $3)
	`, externalID, raw, userID); err != nil {
		return "", fmt.Errorf("insert raw habit event: %w", err)
	}

	var habitID string
	err = db.QueryRow(ctx, `
		INSERT INTO habits (
			user_id, source, external_id, name, area_external_id, area_name, archived,
			recurrence, log_method, time_of_day, remind_at, goal, goal_history_items,
			raw_payload, source_created_at
		)
		VALUES ($1, 'habitify', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (user_id, source, external_id) DO UPDATE SET
			name = EXCLUDED.name,
			area_external_id = EXCLUDED.area_external_id,
			area_name = EXCLUDED.area_name,
			archived = EXCLUDED.archived,
			recurrence = EXCLUDED.recurrence,
			log_method = EXCLUDED.log_method,
			time_of_day = EXCLUDED.time_of_day,
			remind_at = EXCLUDED.remind_at,
			goal = EXCLUDED.goal,
			goal_history_items = EXCLUDED.goal_history_items,
			raw_payload = EXCLUDED.raw_payload,
			source_created_at = COALESCE(EXCLUDED.source_created_at, habits.source_created_at)
		RETURNING id
	`, userID, externalID, name, nullIfEmpty(areaExternalID), nullIfEmpty(areaName), archived, nullIfEmpty(recurrence), nullIfEmpty(logMethod), timeOfDay, remindAt, nullJSON(goalJSON), nullJSON(goalHistoryJSON), raw, sourceCreatedAt).Scan(&habitID)
	if err != nil {
		return "", fmt.Errorf("upsert habit row: %w", err)
	}

	return habitID, nil
}

func (h *HabitifyConnector) upsertDailyStatus(ctx context.Context, db habitifyDB, habitID, userID string, day time.Time, raw json.RawMessage) error {
	payload, err := decodeJSONObject(raw)
	if err != nil {
		return err
	}

	status := strings.ToLower(strings.TrimSpace(jsonString(payload, "status")))
	if status == "" {
		status = "none"
	}

	progress := jsonObject(payload, "progress")
	goal := jsonObject(payload, "goal")
	currentValue := jsonNumber(progress, "current_value", "currentValue")
	targetValue := jsonNumber(progress, "target_value", "targetValue")
	if targetValue == nil {
		targetValue = jsonNumber(goal, "value")
	}
	unitType := firstNonEmpty(jsonString(progress, "unit_type", "unitType"), jsonString(goal, "unit_type", "unitType"))
	periodicity := firstNonEmpty(jsonString(progress, "periodicity"), jsonString(goal, "periodicity"))
	externalID := jsonString(payload, "id")

	if _, err := db.Exec(ctx, `
		INSERT INTO raw_events (source, event_type, external_id, payload, user_id)
		VALUES ('habitify', 'journal', $1, $2, $3)
	`, externalID+":"+day.Format("2006-01-02"), raw, userID); err != nil {
		return fmt.Errorf("insert raw journal event: %w", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO habit_daily_statuses (habit_id, target_date, status, current_value, target_value, unit_type, periodicity, raw_payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (habit_id, target_date) DO UPDATE SET
			status = EXCLUDED.status,
			current_value = EXCLUDED.current_value,
			target_value = EXCLUDED.target_value,
			unit_type = EXCLUDED.unit_type,
			periodicity = EXCLUDED.periodicity,
			raw_payload = EXCLUDED.raw_payload
	`, habitID, day.Format("2006-01-02"), status, currentValue, targetValue, nullIfEmpty(unitType), nullIfEmpty(periodicity), raw)
	if err != nil {
		return fmt.Errorf("upsert habit daily status: %w", err)
	}

	return nil
}

func (h *HabitifyConnector) getLastSync(ctx context.Context, userID string) (time.Time, error) {
	var t time.Time
	err := h.db.QueryRow(ctx, `SELECT last_synced_at FROM sync_state WHERE source = 'habitify' AND user_id = $1`, userID).Scan(&t)
	if err != nil {
		return time.Time{}, nil
	}
	return t, nil
}

func (h *HabitifyConnector) updateLastSync(ctx context.Context, userID string) error {
	_, err := h.db.Exec(ctx, `
		INSERT INTO sync_state (source, last_synced_at, updated_at, enabled, user_id)
		VALUES ('habitify', NOW(), NOW(), TRUE, $1)
		ON CONFLICT (source, user_id) DO UPDATE SET
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at = EXCLUDED.updated_at,
			enabled = TRUE
	`, userID)
	return err
}

func decodeJSONObject(raw json.RawMessage) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	return payload, nil
}

func jsonObject(payload map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		obj, ok := value.(map[string]any)
		if ok {
			return obj
		}
	}
	return nil
}

func jsonString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case string:
			return strings.TrimSpace(v)
		case json.Number:
			return v.String()
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		}
	}
	return ""
}

func jsonBool(payload map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		if b, ok := value.(bool); ok {
			return b
		}
	}
	return false
}

func jsonNumber(payload map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case float64:
			return &v
		case json.Number:
			if f, err := v.Float64(); err == nil {
				return &f
			}
		}
	}
	return nil
}

func jsonStringSlice(payload map[string]any, keys ...string) []string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		items, ok := value.([]any)
		if !ok {
			continue
		}
		result := make([]string, 0, len(items))
		for _, item := range items {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			result = append(result, s)
		}
		return result
	}
	return []string{}
}

func jsonTime(payload map[string]any, keys ...string) *time.Time {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		raw, ok := value.(string)
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02"} {
			if t, err := time.Parse(layout, raw); err == nil {
				return &t
			}
		}
	}
	return nil
}

func jsonSubdocument(payload map[string]any, keys ...string) []byte {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		data, err := json.Marshal(value)
		if err == nil {
			return data
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
