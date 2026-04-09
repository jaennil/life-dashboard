package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	authmw "life-dashboard/internal/middleware"
)

type FitnessHandler struct {
	db     *pgxpool.Pool
	logger zerolog.Logger
}

func NewFitness(db *pgxpool.Pool, logger zerolog.Logger) *FitnessHandler {
	return &FitnessHandler{db: db, logger: logger.With().Str("handler", "fitness").Logger()}
}

type WeekStat struct {
	Week            string  `json:"week"`
	ActivitiesCount int     `json:"activities_count"`
	WorkoutsCount   int     `json:"workouts_count"`
	KM              float64 `json:"km"`
}

type Activity struct {
	ID              string    `json:"id"`
	Type            string    `json:"type"`
	Name            string    `json:"name"`
	StartedAt       time.Time `json:"started_at"`
	DurationSeconds *int      `json:"duration_seconds"`
	DistanceMeters  *float64  `json:"distance_meters"`
	Calories        *int      `json:"calories"`
	AvgHeartRate    *int      `json:"avg_heart_rate"`
}

type WorkoutSet struct {
	ExerciseName     string   `json:"exercise_name"`
	ExerciseCategory string   `json:"exercise_category"`
	ExerciseIndex    int      `json:"exercise_index"`
	SetIndex         int      `json:"set_index"`
	SetType          string   `json:"set_type"`
	WeightKg         *float64 `json:"weight_kg"`
	Reps             *int     `json:"reps"`
	DistanceMeters   *float64 `json:"distance_meters"`
	DurationSeconds  *int     `json:"duration_seconds"`
	RPE              *float64 `json:"rpe"`
}

type WorkoutExercise struct {
	Index      int          `json:"index"`
	Name       string       `json:"name"`
	Category   string       `json:"category"`
	Notes      string       `json:"notes"`
	TemplateID string       `json:"template_id"`
	Sets       []WorkoutSet `json:"sets"`
}

type Workout struct {
	ID              string            `json:"id"`
	Source          string            `json:"source"`
	Title           string            `json:"title"`
	Notes           string            `json:"notes"`
	StartedAt       time.Time         `json:"started_at"`
	EndedAt         *time.Time        `json:"ended_at"`
	SourceCreatedAt *time.Time        `json:"source_created_at"`
	SourceUpdatedAt *time.Time        `json:"source_updated_at"`
	Exercises       []WorkoutExercise `json:"exercises"`
}

type FitnessSummaryResponse struct {
	ActivitiesThisWeek int     `json:"activities_this_week"`
	WorkoutsThisWeek   int     `json:"workouts_this_week"`
	DistanceThisWeek   float64 `json:"distance_km_this_week"`
	ActivitiesTotal    int     `json:"activities_total"`
	WorkoutsTotal      int     `json:"workouts_total"`
	TotalDistanceKm    float64 `json:"total_distance_km"`
}

func (h *FitnessHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	now := time.Now()
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))

	var s FitnessSummaryResponse
	h.db.QueryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(distance_meters)/1000.0,0) FROM activities WHERE started_at >= $1 AND user_id = $2`, weekStart, userID).
		Scan(&s.ActivitiesThisWeek, &s.DistanceThisWeek)
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM workouts WHERE started_at >= $1 AND user_id = $2`, weekStart, userID).
		Scan(&s.WorkoutsThisWeek)
	h.db.QueryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(distance_meters)/1000.0,0) FROM activities WHERE user_id = $1`, userID).
		Scan(&s.ActivitiesTotal, &s.TotalDistanceKm)
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM workouts WHERE user_id = $1`, userID).
		Scan(&s.WorkoutsTotal)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func (h *FitnessHandler) GetWeeklyStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)

	rows, err := h.db.Query(ctx, `
		WITH activity_stats AS (
			SELECT
				DATE_TRUNC('week', started_at) AS week,
				COUNT(*) AS activities_count,
				COALESCE(SUM(distance_meters) / 1000.0, 0) AS km
			FROM activities
			WHERE started_at >= NOW() - INTERVAL '10 weeks' AND user_id = $1
			GROUP BY DATE_TRUNC('week', started_at)
		),
		workout_stats AS (
			SELECT
				DATE_TRUNC('week', started_at) AS week,
				COUNT(*) AS workouts_count
			FROM workouts
			WHERE started_at >= NOW() - INTERVAL '10 weeks' AND user_id = $1
			GROUP BY DATE_TRUNC('week', started_at)
		),
		weeks AS (
			SELECT week FROM activity_stats
			UNION
			SELECT week FROM workout_stats
		)
		SELECT
			TO_CHAR(weeks.week, 'YYYY-MM-DD') AS week,
			COALESCE(activity_stats.activities_count, 0) AS activities_count,
			COALESCE(workout_stats.workouts_count, 0) AS workouts_count,
			COALESCE(activity_stats.km, 0) AS km
		FROM weeks
		LEFT JOIN activity_stats ON activity_stats.week = weeks.week
		LEFT JOIN workout_stats ON workout_stats.week = weeks.week
		ORDER BY weeks.week ASC
	`, userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("query weekly stats")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	stats := make([]WeekStat, 0)
	for rows.Next() {
		var s WeekStat
		if err := rows.Scan(&s.Week, &s.ActivitiesCount, &s.WorkoutsCount, &s.KM); err != nil {
			continue
		}
		stats = append(stats, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *FitnessHandler) GetActivities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)

	rows, err := h.db.Query(ctx, `
		SELECT id, type, COALESCE(name,''), started_at,
			duration_seconds, distance_meters, calories, avg_heart_rate
		FROM activities
		WHERE user_id = $1
		ORDER BY started_at DESC
		LIMIT 30
	`, userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("query activities")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	activities := make([]Activity, 0)
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.Type, &a.Name, &a.StartedAt,
			&a.DurationSeconds, &a.DistanceMeters, &a.Calories, &a.AvgHeartRate); err != nil {
			continue
		}
		activities = append(activities, a)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(activities)
}

func (h *FitnessHandler) GetWorkouts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)

	type workoutRow struct {
		workout    Workout
		rawPayload []byte
	}

	rows, err := h.db.Query(ctx, `
		SELECT id, source, COALESCE(title,''), COALESCE(notes,''), started_at, ended_at, raw_payload
		FROM workouts
		WHERE user_id = $1
		ORDER BY started_at DESC
		LIMIT 30
	`, userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("query workouts")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	workoutRows := make([]workoutRow, 0)
	for rows.Next() {
		var row workoutRow
		if err := rows.Scan(
			&row.workout.ID,
			&row.workout.Source,
			&row.workout.Title,
			&row.workout.Notes,
			&row.workout.StartedAt,
			&row.workout.EndedAt,
			&row.rawPayload,
		); err != nil {
			continue
		}
		workoutRows = append(workoutRows, row)
	}
	rows.Close()

	workouts := make([]Workout, 0, len(workoutRows))
	for i := range workoutRows {
		wk := workoutRows[i].workout

		if wk.Source == "hevy" && len(workoutRows[i].rawPayload) > 0 {
			if err := hydrateHevyWorkout(&wk, workoutRows[i].rawPayload); err == nil {
				workouts = append(workouts, wk)
				continue
			} else {
				h.logger.Warn().Str("workout_id", wk.ID).Err(err).Msg("failed to hydrate hevy workout from raw payload")
			}
		}

		if err := h.loadNormalizedWorkoutExercises(ctx, &wk); err != nil {
			h.logger.Warn().Str("workout_id", wk.ID).Err(err).Msg("failed to load normalized workout exercises")
		}
		workouts = append(workouts, wk)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workouts)
}

func (h *FitnessHandler) loadNormalizedWorkoutExercises(ctx context.Context, workout *Workout) error {
	setRows, err := h.db.Query(ctx, `
		SELECT exercise_name, COALESCE(exercise_category,''), set_index, COALESCE(set_type,'normal'), weight_kg, reps
		FROM workout_sets
		WHERE workout_id = $1
		ORDER BY exercise_name, set_index
	`, workout.ID)
	if err != nil {
		return err
	}
	defer setRows.Close()

	exMap := make(map[string]*WorkoutExercise)
	exOrder := []string{}
	for setRows.Next() {
		var s WorkoutSet
		var exName, exCat string
		if err := setRows.Scan(&exName, &exCat, &s.SetIndex, &s.SetType, &s.WeightKg, &s.Reps); err != nil {
			continue
		}
		s.ExerciseName = exName
		s.ExerciseCategory = exCat
		if _, ok := exMap[exName]; !ok {
			exMap[exName] = &WorkoutExercise{Name: exName, Category: exCat, Sets: []WorkoutSet{}}
			exOrder = append(exOrder, exName)
		}
		exMap[exName].Sets = append(exMap[exName].Sets, s)
	}

	workout.Exercises = make([]WorkoutExercise, 0, len(exOrder))
	for _, name := range exOrder {
		workout.Exercises = append(workout.Exercises, *exMap[name])
	}

	return nil
}

type hevyRawWorkout struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	CreatedAt   *time.Time        `json:"created_at"`
	UpdatedAt   *time.Time        `json:"updated_at"`
	Exercises   []hevyRawExercise `json:"exercises"`
}

type hevyRawExercise struct {
	Index              int          `json:"index"`
	Title              string       `json:"title"`
	Notes              string       `json:"notes"`
	ExerciseTemplateID string       `json:"exercise_template_id"`
	Sets               []hevyRawSet `json:"sets"`
}

type hevyRawSet struct {
	Index           int      `json:"index"`
	Type            string   `json:"type"`
	WeightKg        *float64 `json:"weight_kg"`
	Reps            *int     `json:"reps"`
	DistanceMeters  *float64 `json:"distance_meters"`
	DurationSeconds *int     `json:"duration_seconds"`
	RPE             *float64 `json:"rpe"`
}

func hydrateHevyWorkout(workout *Workout, rawPayload []byte) error {
	var payload hevyRawWorkout
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return err
	}

	if payload.Title != "" {
		workout.Title = payload.Title
	}
	if payload.Description != "" {
		workout.Notes = payload.Description
	}
	workout.SourceCreatedAt = payload.CreatedAt
	workout.SourceUpdatedAt = payload.UpdatedAt
	workout.Exercises = make([]WorkoutExercise, 0, len(payload.Exercises))

	for _, ex := range payload.Exercises {
		exercise := WorkoutExercise{
			Index:      ex.Index,
			Name:       ex.Title,
			Notes:      ex.Notes,
			TemplateID: ex.ExerciseTemplateID,
			Sets:       make([]WorkoutSet, 0, len(ex.Sets)),
		}

		for _, set := range ex.Sets {
			exercise.Sets = append(exercise.Sets, WorkoutSet{
				ExerciseName:    ex.Title,
				ExerciseIndex:   ex.Index,
				SetIndex:        set.Index,
				SetType:         coalesceWorkoutSetType(set.Type),
				WeightKg:        set.WeightKg,
				Reps:            set.Reps,
				DistanceMeters:  set.DistanceMeters,
				DurationSeconds: set.DurationSeconds,
				RPE:             set.RPE,
			})
		}

		workout.Exercises = append(workout.Exercises, exercise)
	}

	return nil
}

func coalesceWorkoutSetType(value string) string {
	if value == "" {
		return "normal"
	}
	return value
}
