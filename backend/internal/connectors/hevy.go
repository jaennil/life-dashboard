package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	hevyBaseURL  = "https://api.hevyapp.com/v1"
	hevyPageSize = 10
)

// ---- API response types ----

type hevyWorkoutsResponse struct {
	Page      int           `json:"page"`
	PageCount int           `json:"page_count"`
	Workouts  []hevyWorkout `json:"workouts"`
}

type hevyRoutinesResponse struct {
	Page      int           `json:"page"`
	PageCount int           `json:"page_count"`
	Routines  []hevyRoutine `json:"routines"`
}

type hevyEventsResponse struct {
	Page      int         `json:"page"`
	PageCount int         `json:"page_count"`
	Events    []hevyEvent `json:"events"`
}

type hevyEvent struct {
	Type      string       `json:"type"` // "updated" | "deleted"
	Workout   *hevyWorkout `json:"workout,omitempty"`
	WorkoutID string       `json:"workout_id,omitempty"`
	DeletedAt *time.Time   `json:"deleted_at,omitempty"`
}

type hevyWorkout struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	RoutineID   string         `json:"routine_id"`
	Description string         `json:"description"`
	StartTime   time.Time      `json:"start_time"`
	EndTime     *time.Time     `json:"end_time"`
	UpdatedAt   time.Time      `json:"updated_at"`
	CreatedAt   time.Time      `json:"created_at"`
	Exercises   []hevyExercise `json:"exercises"`
}

type hevyRoutine struct {
	ID        string             `json:"id"`
	Title     string             `json:"title"`
	FolderID  *int64             `json:"folder_id"`
	UpdatedAt time.Time          `json:"updated_at"`
	CreatedAt time.Time          `json:"created_at"`
	Exercises []hevyRoutineEntry `json:"exercises"`
}

type hevyFlexibleString string

func (s *hevyFlexibleString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*s = ""
		return nil
	}

	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = hevyFlexibleString(str)
		return nil
	}

	var num json.Number
	if err := json.Unmarshal(data, &num); err == nil {
		*s = hevyFlexibleString(num.String())
		return nil
	}

	return fmt.Errorf("unsupported flexible string payload: %s", string(data))
}

func (s hevyFlexibleString) String() string {
	return string(s)
}

type hevyExercise struct {
	Index              int                `json:"index"`
	Title              string             `json:"title"`
	Notes              string             `json:"notes"`
	ExerciseTemplateID string             `json:"exercise_template_id"`
	SupersetID         hevyFlexibleString `json:"superset_id"`
	Sets               []hevySet          `json:"sets"`
}

type hevySet struct {
	Index           int      `json:"index"`
	Type            string   `json:"type"`
	WeightKg        *float64 `json:"weight_kg"`
	Reps            *int     `json:"reps"`
	DistanceMeters  *float64 `json:"distance_meters"`
	DurationSeconds *int     `json:"duration_seconds"`
	RPE             *float64 `json:"rpe"`
	CustomMetric    any      `json:"custom_metric"`
}

type hevyRoutineEntry struct {
	Index              int                `json:"index"`
	Title              string             `json:"title"`
	Notes              *string            `json:"notes"`
	ExerciseTemplateID string             `json:"exercise_template_id"`
	SupersetID         hevyFlexibleString `json:"superset_id"`
	RestSeconds        *int               `json:"rest_seconds"`
	Sets               []hevyRoutineSet   `json:"sets"`
}

type hevyRoutineSet struct {
	Index           int      `json:"index"`
	Type            string   `json:"type"`
	WeightKg        *float64 `json:"weight_kg"`
	Reps            *int     `json:"reps"`
	DistanceMeters  *float64 `json:"distance_meters"`
	DurationSeconds *int     `json:"duration_seconds"`
	CustomMetric    any      `json:"custom_metric"`
}

// ---- Connector ----

type HevyConnector struct {
	db     *pgxpool.Pool
	client *http.Client
	logger zerolog.Logger
}

func NewHevy(db *pgxpool.Pool, logger zerolog.Logger) *HevyConnector {
	return &HevyConnector{
		db:     db,
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger.With().Str("connector", "hevy").Logger(),
	}
}

func (h *HevyConnector) loadAPIKey(ctx context.Context, userID string) (string, error) {
	var key string
	err := h.db.QueryRow(ctx, `SELECT access_token FROM oauth_tokens WHERE source = 'hevy' AND user_id = $1`, userID).Scan(&key)
	if err != nil {
		return "", fmt.Errorf("no API key — add your Hevy API key in Settings")
	}
	return key, nil
}

func (h *HevyConnector) Name() string { return "hevy" }

func (h *HevyConnector) Sync(ctx context.Context, userID string) error {
	apiKey, err := h.loadAPIKey(ctx, userID)
	if err != nil {
		return err
	}

	lastSync, err := h.getLastSync(ctx, userID)
	if err != nil {
		return fmt.Errorf("get last sync: %w", err)
	}

	if lastSync.IsZero() {
		h.logger.Info().Msg("no previous sync, running full historical sync")
		if err := h.syncFull(ctx, userID, apiKey); err != nil {
			return err
		}
	} else {
		h.logger.Info().Time("since", lastSync).Msg("running incremental sync")
		if err := h.syncIncremental(ctx, userID, lastSync, apiKey); err != nil {
			return err
		}
	}

	if err := h.syncRoutines(ctx, userID, apiKey); err != nil {
		return fmt.Errorf("sync routines: %w", err)
	}

	return h.updateLastSync(ctx, userID)
}

// syncFull fetches all workouts via paginated /v1/workouts
func (h *HevyConnector) syncFull(ctx context.Context, userID string, apiKey string) error {
	page := 1
	total := 0

	for {
		resp, err := h.fetchWorkoutsPage(ctx, apiKey, page)
		if err != nil {
			return fmt.Errorf("fetch page %d: %w", page, err)
		}

		for i := range resp.Workouts {
			if err := h.upsertWorkout(ctx, userID, &resp.Workouts[i]); err != nil {
				return fmt.Errorf("upsert workout %s: %w", resp.Workouts[i].ID, err)
			}
		}

		total += len(resp.Workouts)
		h.logger.Debug().Int("page", page).Int("page_count", resp.PageCount).Int("synced", total).Msg("page synced")

		if page >= resp.PageCount {
			break
		}
		page++
	}

	h.logger.Info().Int("total", total).Msg("full sync complete")
	return nil
}

// syncIncremental uses /v1/workouts/events to fetch changes since last sync
func (h *HevyConnector) syncIncremental(ctx context.Context, userID string, since time.Time, apiKey string) error {
	page := 1
	updated, deleted := 0, 0

	for {
		resp, err := h.fetchEventsPage(ctx, apiKey, since, page)
		if err != nil {
			return fmt.Errorf("fetch events page %d: %w", page, err)
		}

		for i := range resp.Events {
			event := &resp.Events[i]
			switch event.Type {
			case "updated":
				if event.Workout == nil {
					h.logger.Warn().Msg("event type 'updated' has no workout payload, skipping")
					continue
				}
				if err := h.upsertWorkout(ctx, userID, event.Workout); err != nil {
					return fmt.Errorf("upsert workout %s: %w", event.Workout.ID, err)
				}
				updated++
			case "deleted":
				if err := h.deleteWorkout(ctx, userID, event.WorkoutID); err != nil {
					return fmt.Errorf("delete workout %s: %w", event.WorkoutID, err)
				}
				deleted++
			default:
				h.logger.Warn().Str("type", event.Type).Msg("unknown event type, skipping")
			}
		}

		h.logger.Debug().Int("page", page).Int("page_count", resp.PageCount).Msg("events page processed")

		if page >= resp.PageCount {
			break
		}
		page++
	}

	h.logger.Info().Int("updated", updated).Int("deleted", deleted).Msg("incremental sync complete")
	return nil
}

// ---- HTTP helpers ----

func (h *HevyConnector) fetchWorkoutsPage(ctx context.Context, apiKey string, page int) (*hevyWorkoutsResponse, error) {
	url := fmt.Sprintf("%s/workouts?page=%d&pageSize=%d", hevyBaseURL, page, hevyPageSize)
	return doRequest[hevyWorkoutsResponse](ctx, h.client, apiKey, url)
}

func (h *HevyConnector) fetchEventsPage(ctx context.Context, apiKey string, since time.Time, page int) (*hevyEventsResponse, error) {
	url := fmt.Sprintf("%s/workouts/events?since=%s&page=%d&pageSize=%d",
		hevyBaseURL, since.UTC().Format(time.RFC3339), page, hevyPageSize)
	return doRequest[hevyEventsResponse](ctx, h.client, apiKey, url)
}

func (h *HevyConnector) fetchRoutinesPage(ctx context.Context, apiKey string, page int) (*hevyRoutinesResponse, error) {
	url := fmt.Sprintf("%s/routines?page=%d&pageSize=%d", hevyBaseURL, page, hevyPageSize)
	return doRequest[hevyRoutinesResponse](ctx, h.client, apiKey, url)
}

func doRequest[T any](ctx context.Context, client *http.Client, apiKey, url string) (*T, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("api-key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hevy api returned status %d", resp.StatusCode)
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// ---- Database helpers ----

func (h *HevyConnector) upsertWorkout(ctx context.Context, userID string, w *hevyWorkout) error {
	raw, err := json.Marshal(w)
	if err != nil {
		return err
	}

	// Store raw event
	_, err = h.db.Exec(ctx, `
		INSERT INTO raw_events (source, event_type, external_id, payload, user_id)
		VALUES ('hevy', 'workout', $1, $2, $3)
	`, w.ID, raw, userID)
	if err != nil {
		return fmt.Errorf("insert raw event: %w", err)
	}

	// Upsert workout
	_, err = h.db.Exec(ctx, `
		INSERT INTO workouts (source, external_id, routine_external_id, started_at, ended_at, title, notes, raw_payload, user_id)
		VALUES ('hevy', $1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, external_id) DO UPDATE SET
			routine_external_id = EXCLUDED.routine_external_id,
			started_at  = EXCLUDED.started_at,
			ended_at    = EXCLUDED.ended_at,
			title       = EXCLUDED.title,
			notes       = EXCLUDED.notes,
			raw_payload = EXCLUDED.raw_payload
	`, w.ID, nullIfEmpty(w.RoutineID), w.StartTime, w.EndTime, w.Title, w.Description, raw, userID)
	if err != nil {
		return fmt.Errorf("upsert workout: %w", err)
	}

	// Get internal workout id
	var workoutID string
	err = h.db.QueryRow(ctx, `SELECT id FROM workouts WHERE external_id = $1 AND user_id = $2`, w.ID, userID).Scan(&workoutID)
	if err != nil {
		return fmt.Errorf("get workout id: %w", err)
	}

	// Delete existing sets (full replace on update)
	_, err = h.db.Exec(ctx, `DELETE FROM workout_sets WHERE workout_id = $1`, workoutID)
	if err != nil {
		return fmt.Errorf("delete old sets: %w", err)
	}

	// Insert sets
	for _, ex := range w.Exercises {
		for _, s := range ex.Sets {
			_, err = h.db.Exec(ctx, `
				INSERT INTO workout_sets
					(workout_id, exercise_name, set_index, set_type, weight_kg, reps, duration_seconds, rpe)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`, workoutID, ex.Title, s.Index, s.Type, s.WeightKg, s.Reps, s.DurationSeconds, s.RPE)
			if err != nil {
				return fmt.Errorf("insert set: %w", err)
			}
		}
	}

	h.logger.Debug().Str("workout_id", w.ID).Str("title", w.Title).Msg("workout upserted")
	return nil
}

func (h *HevyConnector) syncRoutines(ctx context.Context, userID string, apiKey string) error {
	page := 1
	total := 0
	seen := make([]string, 0, 32)

	for {
		resp, err := h.fetchRoutinesPage(ctx, apiKey, page)
		if err != nil {
			return fmt.Errorf("fetch routines page %d: %w", page, err)
		}

		for i := range resp.Routines {
			if err := h.upsertRoutine(ctx, userID, &resp.Routines[i]); err != nil {
				return fmt.Errorf("upsert routine %s: %w", resp.Routines[i].ID, err)
			}
			seen = append(seen, resp.Routines[i].ID)
		}

		total += len(resp.Routines)
		h.logger.Debug().Int("page", page).Int("page_count", resp.PageCount).Int("synced", total).Msg("routine page synced")

		if page >= resp.PageCount {
			break
		}
		page++
	}

	if err := h.pruneMissingRoutines(ctx, userID, seen); err != nil {
		return fmt.Errorf("prune routines: %w", err)
	}

	h.logger.Info().Int("total", total).Msg("routines sync complete")
	return nil
}

func (h *HevyConnector) upsertRoutine(ctx context.Context, userID string, routine *hevyRoutine) error {
	raw, err := json.Marshal(routine)
	if err != nil {
		return err
	}

	_, err = h.db.Exec(ctx, `
		INSERT INTO raw_events (source, event_type, external_id, payload, user_id)
		VALUES ('hevy', 'routine', $1, $2, $3)
	`, routine.ID, raw, userID)
	if err != nil {
		return fmt.Errorf("insert raw routine event: %w", err)
	}

	_, err = h.db.Exec(ctx, `
		INSERT INTO workout_routines (user_id, source, external_id, title, folder_id, raw_payload, source_created_at, source_updated_at)
		VALUES ($1, 'hevy', $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, external_id) DO UPDATE SET
			title = EXCLUDED.title,
			folder_id = EXCLUDED.folder_id,
			raw_payload = EXCLUDED.raw_payload,
			source_created_at = EXCLUDED.source_created_at,
			source_updated_at = EXCLUDED.source_updated_at
	`, userID, routine.ID, routine.Title, routine.FolderID, raw, routine.CreatedAt, routine.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert routine: %w", err)
	}

	var routineID string
	err = h.db.QueryRow(ctx, `
		SELECT id FROM workout_routines WHERE user_id = $1 AND external_id = $2
	`, userID, routine.ID).Scan(&routineID)
	if err != nil {
		return fmt.Errorf("get routine id: %w", err)
	}

	if _, err := h.db.Exec(ctx, `DELETE FROM routine_exercises WHERE routine_id = $1`, routineID); err != nil {
		return fmt.Errorf("delete old routine exercises: %w", err)
	}

	for _, exercise := range routine.Exercises {
		var routineExerciseID string
		err := h.db.QueryRow(ctx, `
			INSERT INTO routine_exercises (routine_id, exercise_index, exercise_name, notes, template_id, superset_id, rest_seconds)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id
		`, routineID, exercise.Index, exercise.Title, stringValue(exercise.Notes), nullIfEmpty(exercise.ExerciseTemplateID), nullIfEmpty(exercise.SupersetID.String()), exercise.RestSeconds).Scan(&routineExerciseID)
		if err != nil {
			return fmt.Errorf("insert routine exercise: %w", err)
		}

		for _, set := range exercise.Sets {
			customMetric, err := json.Marshal(set.CustomMetric)
			if err != nil {
				return fmt.Errorf("marshal routine custom metric: %w", err)
			}
			if _, err := h.db.Exec(ctx, `
				INSERT INTO routine_sets (routine_exercise_id, set_index, set_type, weight_kg, reps, distance_meters, duration_seconds, custom_metric)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`, routineExerciseID, set.Index, coalesceHevySetType(set.Type), set.WeightKg, set.Reps, set.DistanceMeters, set.DurationSeconds, nullJSON(customMetric)); err != nil {
				return fmt.Errorf("insert routine set: %w", err)
			}
		}
	}

	h.logger.Debug().Str("routine_id", routine.ID).Str("title", routine.Title).Msg("routine upserted")
	return nil
}

func (h *HevyConnector) pruneMissingRoutines(ctx context.Context, userID string, seen []string) error {
	if len(seen) == 0 {
		_, err := h.db.Exec(ctx, `DELETE FROM workout_routines WHERE user_id = $1 AND source = 'hevy'`, userID)
		return err
	}

	_, err := h.db.Exec(ctx, `
		DELETE FROM workout_routines
		WHERE user_id = $1
			AND source = 'hevy'
			AND NOT (external_id = ANY($2))
	`, userID, seen)
	return err
}

func (h *HevyConnector) deleteWorkout(ctx context.Context, userID string, externalID string) error {
	_, err := h.db.Exec(ctx, `DELETE FROM workouts WHERE external_id = $1 AND user_id = $2`, externalID, userID)
	if err != nil {
		return err
	}
	h.logger.Debug().Str("external_id", externalID).Msg("workout deleted")
	return nil
}

func (h *HevyConnector) getLastSync(ctx context.Context, userID string) (time.Time, error) {
	var t time.Time
	err := h.db.QueryRow(ctx, `
		SELECT last_synced_at FROM sync_state WHERE source = 'hevy' AND user_id = $1
	`, userID).Scan(&t)
	if err != nil {
		// No row = first sync
		return time.Time{}, nil
	}
	return t, nil
}

func (h *HevyConnector) updateLastSync(ctx context.Context, userID string) error {
	_, err := h.db.Exec(ctx, `
		INSERT INTO sync_state (source, last_synced_at, updated_at, user_id)
		VALUES ('hevy', NOW(), NOW(), $1)
		ON CONFLICT (source, user_id) DO UPDATE SET
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at     = EXCLUDED.updated_at
	`, userID)
	return err
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func coalesceHevySetType(value string) string {
	if value == "" {
		return "normal"
	}
	return value
}

func nullJSON(value []byte) any {
	if string(value) == "null" {
		return nil
	}
	return value
}
