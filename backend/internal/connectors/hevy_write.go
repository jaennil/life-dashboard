package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HevyWorkoutDraft is a workout ready to be created. It is the exported shape the
// voice handler fills in, deliberately free of anything provider-specific beyond
// the template ids it already resolved.
type HevyWorkoutDraft struct {
	Title     string
	StartTime time.Time
	EndTime   time.Time
	Exercises []HevyExerciseDraft
}

type HevyExerciseDraft struct {
	TemplateID string
	Sets       []HevySetDraft
}

type HevySetDraft struct {
	Type            string
	WeightKg        *float64
	Reps            *int
	DurationSeconds *int
}

// The request body is snake_case, matching the read side. Several third-party
// write-ups document camelCase for this endpoint; the OpenAPI spec and the rest
// of the API disagree with them, and a wrong casing here would be accepted as a
// workout with no exercises rather than rejected.
type hevyCreateWorkoutRequest struct {
	Workout hevyWorkoutPayload `json:"workout"`
}

type hevyWorkoutPayload struct {
	Title       string                `json:"title"`
	Description *string               `json:"description"`
	StartTime   string                `json:"start_time"`
	EndTime     string                `json:"end_time"`
	IsPrivate   bool                  `json:"is_private"`
	Exercises   []hevyExercisePayload `json:"exercises"`
}

type hevyExercisePayload struct {
	ExerciseTemplateID string           `json:"exercise_template_id"`
	SupersetID         *string          `json:"superset_id"`
	Notes              *string          `json:"notes"`
	Sets               []hevySetPayload `json:"sets"`
}

type hevySetPayload struct {
	Type            string   `json:"type"`
	WeightKg        *float64 `json:"weight_kg"`
	Reps            *int     `json:"reps"`
	DistanceMeters  *float64 `json:"distance_meters"`
	DurationSeconds *int     `json:"duration_seconds"`
	RPE             *float64 `json:"rpe"`
}

type hevyCreateWorkoutResponse struct {
	Workout struct {
		ID string `json:"id"`
	} `json:"workout"`
	// Some responses return the object at the top level instead.
	ID string `json:"id"`
}

// CreateWorkout writes a workout to Hevy and returns its provider id.
func (h *HevyConnector) CreateWorkout(ctx context.Context, userID string, draft HevyWorkoutDraft) (string, error) {
	if len(draft.Exercises) == 0 {
		return "", fmt.Errorf("refusing to create an empty workout")
	}

	apiKey, err := h.loadAPIKey(ctx, userID)
	if err != nil {
		return "", err
	}

	payload := hevyCreateWorkoutRequest{Workout: hevyWorkoutPayload{
		Title: draft.Title,
		// Hevy rejects a missing start or end, and the session knows both: the
		// first phrase and the last.
		StartTime: draft.StartTime.UTC().Format(time.RFC3339),
		EndTime:   draft.EndTime.UTC().Format(time.RFC3339),
		IsPrivate: false,
		Exercises: make([]hevyExercisePayload, 0, len(draft.Exercises)),
	}}

	for _, exercise := range draft.Exercises {
		sets := make([]hevySetPayload, 0, len(exercise.Sets))
		for _, set := range exercise.Sets {
			setType := set.Type
			if setType == "" {
				setType = "normal"
			}
			sets = append(sets, hevySetPayload{
				Type:            setType,
				WeightKg:        set.WeightKg,
				Reps:            set.Reps,
				DurationSeconds: set.DurationSeconds,
			})
		}
		payload.Workout.Exercises = append(payload.Workout.Exercises, hevyExercisePayload{
			ExerciseTemplateID: exercise.TemplateID,
			Sets:               sets,
		})
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hevyBaseURL+"/workouts", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	answer, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("hevy create workout: http %d: %s", resp.StatusCode, truncateForLog(answer))
	}

	var decoded hevyCreateWorkoutResponse
	if err := json.Unmarshal(answer, &decoded); err != nil {
		return "", fmt.Errorf("decode create response: %w", err)
	}

	id := decoded.Workout.ID
	if id == "" {
		id = decoded.ID
	}
	if id == "" {
		// The workout may well have been created; without an id we cannot record
		// it, and reporting success would invite a duplicate on the next attempt.
		return "", fmt.Errorf("hevy create workout returned no id: %s", truncateForLog(answer))
	}

	h.logger.Info().Str("workout_id", id).Int("exercises", len(draft.Exercises)).
		Msg("workout created in hevy")
	return id, nil
}
