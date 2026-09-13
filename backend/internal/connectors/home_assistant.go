package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// homeAssistantLocation is the timezone a day is counted in. A daily counter
// belongs to the day the person lived, not to the day UTC happened to be on.
var homeAssistantLocation = func() *time.Location {
	loc, err := time.LoadLocation(zeppTimezone)
	if err != nil {
		return time.FixedZone("MSK", 3*60*60)
	}
	return loc
}()

const (
	homeAssistantSource         = "home_assistant"
	homeAssistantRequestTimeout = 30 * time.Second
	// After asking the phone to report, this is how long it is given before the
	// states are read. The request goes out as a push and comes back through the
	// app; a couple of seconds is enough when the phone is awake and no amount of
	// waiting helps when it is not.
	homeAssistantWakeGrace = 4 * time.Second
)

// HomeAssistantConnector reads what the phone reports to Home Assistant.
//
// It exists because Apple Health cannot be pulled: the phone pushes it, on its
// own schedule, and until now that meant a report was written against whatever
// the last Shortcut run happened to send. Home Assistant already runs the phone
// app, already holds these readings as entities, and can be asked for them at
// the moment a report needs them.
type HomeAssistantConnector struct {
	db        *pgxpool.Pool
	client    *http.Client
	baseURL   string
	token     string
	mobileApp string
	logger    zerolog.Logger
}

func NewHomeAssistant(db *pgxpool.Pool, baseURL, token, mobileApp string, logger zerolog.Logger) *HomeAssistantConnector {
	return &HomeAssistantConnector{
		db:        db,
		client:    &http.Client{Timeout: homeAssistantRequestTimeout},
		baseURL:   strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:     strings.TrimSpace(token),
		mobileApp: strings.TrimSpace(mobileApp),
		logger:    logger.With().Str("connector", homeAssistantSource).Logger(),
	}
}

func (h *HomeAssistantConnector) Name() string { return homeAssistantSource }

// Configured reports whether this instance has anything to talk to.
func (h *HomeAssistantConnector) Configured() bool {
	return h.baseURL != "" && h.token != ""
}

// homeAssistantReading describes one entity worth storing and how to read it.
type homeAssistantReading struct {
	entity string
	metric string
	unit   string
	// daily marks a counter the phone resets at midnight. Those are stamped at
	// midday of their own day and overwritten on every sync, so the row always
	// holds the day's running total rather than a scatter of partial readings.
	daily bool
}

// homeAssistantReadings is deliberately a short list rather than everything the
// app offers. Each entry is either a measurement no other source provides, or a
// second opinion worth having on one that matters.
var homeAssistantReadings = []homeAssistantReading{
	// Nothing else in this project reports these at all.
	{entity: "sensor.iphone_blood_oxygen", metric: "spo2", unit: "%"},
	{entity: "sensor.iphone_respiratory_rate", metric: "respiratory_rate", unit: "br/min"},
	{entity: "sensor.iphone_vo2_max", metric: "vo2max", unit: "ml/kg/min"},
	{entity: "sensor.iphone_heart_rate_variability", metric: "hrv", unit: "ms"},
	// Second opinions.
	{entity: "sensor.iphone_heart_rate", metric: "heart_rate", unit: "bpm"},
	{entity: "sensor.iphone_resting_heart_rate", metric: "resting_heart_rate", unit: "bpm"},
	{entity: "sensor.iphone_body_fat_percentage", metric: "body_fat", unit: "%"},
	{entity: "sensor.iphone_lean_body_mass", metric: "lean_body_mass", unit: "kg"},
	{entity: "sensor.iphone_weight", metric: "weight", unit: "kg"},
	// Counters that reset at midnight.
	{entity: "sensor.iphone_health_steps", metric: "steps", unit: "count", daily: true},
	{entity: "sensor.iphone_active_energy", metric: "active_energy", unit: "kcal", daily: true},
}

type homeAssistantState struct {
	EntityID    string `json:"entity_id"`
	State       string `json:"state"`
	LastUpdated string `json:"last_updated"`
}

func (h *HomeAssistantConnector) Sync(ctx context.Context, userID string) error {
	if !h.Configured() {
		return fmt.Errorf("home assistant is not configured — set the base url and a long-lived token")
	}

	// Ask the phone to report before reading it. iOS refreshes the app's sensors
	// when it delivers a location update, and this is the only command that makes
	// it happen on demand; without it the values are as old as the last time iOS
	// felt like waking the app.
	h.requestPhoneUpdate(ctx)

	states, err := h.fetchStates(ctx)
	if err != nil {
		return fmt.Errorf("fetch home assistant states: %w", err)
	}

	saved := 0
	for _, reading := range homeAssistantReadings {
		state, ok := states[reading.entity]
		if !ok {
			continue
		}

		value, ok := parseHomeAssistantValue(state.State)
		if !ok {
			// "unavailable" and "unknown" are how Home Assistant says the phone has
			// no such reading, which is normal for anything Apple Health never
			// recorded.
			continue
		}

		stamp, err := homeAssistantStamp(state, reading.daily)
		if err != nil {
			h.logger.Warn().Err(err).Str("entity", reading.entity).Msg("unreadable timestamp")
			continue
		}

		if err := h.upsertMetric(ctx, userID, stamp, reading, value); err != nil {
			return fmt.Errorf("store %s: %w", reading.metric, err)
		}
		saved++
	}

	h.logger.Info().Str("user_id", userID).Int("entities", len(states)).Int("stored", saved).
		Msg("home assistant sync finished")
	return nil
}

// requestPhoneUpdate nudges the phone. It is best effort by design: a failure
// here means the readings are merely stale, which is still better than no sync.
func (h *HomeAssistantConnector) requestPhoneUpdate(ctx context.Context) {
	if h.mobileApp == "" {
		return
	}

	payload, err := json.Marshal(map[string]string{"message": "request_location_update"})
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		h.baseURL+"/api/services/notify/"+h.mobileApp, strings.NewReader(string(payload)))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		h.logger.Warn().Err(err).Msg("could not ask the phone to report")
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		h.logger.Warn().Int("status", resp.StatusCode).Msg("phone update request refused")
		return
	}

	select {
	case <-ctx.Done():
	case <-time.After(homeAssistantWakeGrace):
	}
}

func (h *HomeAssistantConnector) fetchStates(ctx context.Context) (map[string]homeAssistantState, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+"/api/states", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("home assistant answered %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var list []homeAssistantState
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("decode states: %w", err)
	}

	states := make(map[string]homeAssistantState, len(list))
	for _, state := range list {
		states[state.EntityID] = state
	}
	return states, nil
}

// parseHomeAssistantValue reads the numeric state, rejecting the words Home
// Assistant uses for absence.
func parseHomeAssistantValue(state string) (float64, bool) {
	state = strings.TrimSpace(state)
	switch strings.ToLower(state) {
	case "", "unknown", "unavailable", "none":
		return 0, false
	}
	value, err := strconv.ParseFloat(state, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

// homeAssistantStamp decides when a reading happened.
//
// A measurement belongs at the moment it was taken. A daily counter belongs to
// its day and nowhere in particular within it, so it goes to midday - the same
// convention every other daily metric here uses, and the one that keeps
// DATE(timestamp) on the intended day under either timezone.
func homeAssistantStamp(state homeAssistantState, daily bool) (time.Time, error) {
	updated, err := time.Parse(time.RFC3339, state.LastUpdated)
	if err != nil {
		return time.Time{}, err
	}
	if !daily {
		return updated.UTC(), nil
	}

	local := updated.In(homeAssistantLocation)
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	return dayStart.Add(12 * time.Hour).UTC(), nil
}

func (h *HomeAssistantConnector) upsertMetric(ctx context.Context, userID string, stamp time.Time, reading homeAssistantReading, value float64) error {
	_, err := h.db.Exec(ctx, `
		INSERT INTO biometrics (timestamp, source, metric_type, value, unit, metadata, user_id)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
		ON CONFLICT (user_id, timestamp, source, metric_type) DO UPDATE SET
			value = EXCLUDED.value,
			unit = EXCLUDED.unit,
			metadata = EXCLUDED.metadata
	`, stamp, homeAssistantSource, reading.metric, value, reading.unit,
		fmt.Sprintf(`{"entity_id":%q}`, reading.entity), userID)
	return err
}
