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
	Page      int            `json:"page"`
	PageCount int            `json:"page_count"`
	Workouts  []hevyWorkout  `json:"workouts"`
}

type hevyEventsResponse struct {
	Page      int           `json:"page"`
	PageCount int           `json:"page_count"`
	Events    []hevyEvent   `json:"events"`
}

type hevyEvent struct {
	Type      string       `json:"type"` // "updated" | "deleted"
	Workout   *hevyWorkout `json:"workout,omitempty"`
	WorkoutID string       `json:"workout_id,omitempty"`
	DeletedAt *time.Time   `json:"deleted_at,omitempty"`
}

type hevyWorkout struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	StartTime   time.Time       `json:"start_time"`
	EndTime     *time.Time      `json:"end_time"`
	UpdatedAt   time.Time       `json:"updated_at"`
	CreatedAt   time.Time       `json:"created_at"`
	Exercises   []hevyExercise  `json:"exercises"`
}

type hevyExercise struct {
	Index              int       `json:"index"`
	Title              string    `json:"title"`
	Notes              string    `json:"notes"`
	ExerciseTemplateID string    `json:"exercise_template_id"`
	Sets               []hevySet `json:"sets"`
}

type hevySet struct {
	Index           int      `json:"index"`
	Type            string   `json:"type"`
	WeightKg        *float64 `json:"weight_kg"`
	Reps            *int     `json:"reps"`
	DistanceMeters  *float64 `json:"distance_meters"`
	DurationSeconds *int     `json:"duration_seconds"`
	RPE             *float64 `json:"rpe"`
}

// ---- Connector ----

type HevyConnector struct {
	apiKey string
	db     *pgxpool.Pool
	client *http.Client
	logger zerolog.Logger
}

func NewHevy(apiKey string, db *pgxpool.Pool, logger zerolog.Logger) *HevyConnector {
	return &HevyConnector{
		apiKey: apiKey,
		db:     db,
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger.With().Str("connector", "hevy").Logger(),
	}
}

func (h *HevyConnector) Name() string { return "hevy" }

func (h *HevyConnector) Sync(ctx context.Context, userID string) error {
	lastSync, err := h.getLastSync(ctx, userID)
	if err != nil {
		return fmt.Errorf("get last sync: %w", err)
	}

	if lastSync.IsZero() {
		h.logger.Info().Msg("no previous sync, running full historical sync")
		if err := h.syncFull(ctx, userID); err != nil {
			return err
		}
	} else {
		h.logger.Info().Time("since", lastSync).Msg("running incremental sync")
		if err := h.syncIncremental(ctx, userID, lastSync); err != nil {
			return err
		}
	}

	return h.updateLastSync(ctx, userID)
}

// syncFull fetches all workouts via paginated /v1/workouts
func (h *HevyConnector) syncFull(ctx context.Context, userID string) error {
	page := 1
	total := 0

	for {
		resp, err := h.fetchWorkoutsPage(ctx, page)
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
func (h *HevyConnector) syncIncremental(ctx context.Context, userID string, since time.Time) error {
	page := 1
	updated, deleted := 0, 0

	for {
		resp, err := h.fetchEventsPage(ctx, since, page)
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

func (h *HevyConnector) fetchWorkoutsPage(ctx context.Context, page int) (*hevyWorkoutsResponse, error) {
	url := fmt.Sprintf("%s/workouts?page=%d&pageSize=%d", hevyBaseURL, page, hevyPageSize)
	return doRequest[hevyWorkoutsResponse](ctx, h.client, h.apiKey, url)
}

func (h *HevyConnector) fetchEventsPage(ctx context.Context, since time.Time, page int) (*hevyEventsResponse, error) {
	url := fmt.Sprintf("%s/workouts/events?since=%s&page=%d&pageSize=%d",
		hevyBaseURL, since.UTC().Format(time.RFC3339), page, hevyPageSize)
	return doRequest[hevyEventsResponse](ctx, h.client, h.apiKey, url)
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
		INSERT INTO workouts (source, external_id, started_at, ended_at, title, notes, raw_payload, user_id)
		VALUES ('hevy', $1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, external_id) DO UPDATE SET
			started_at  = EXCLUDED.started_at,
			ended_at    = EXCLUDED.ended_at,
			title       = EXCLUDED.title,
			notes       = EXCLUDED.notes,
			raw_payload = EXCLUDED.raw_payload
	`, w.ID, w.StartTime, w.EndTime, w.Title, w.Description, raw, userID)
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
