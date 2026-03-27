package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type FitnessHandler struct {
	db     *pgxpool.Pool
	logger zerolog.Logger
}

func NewFitness(db *pgxpool.Pool, logger zerolog.Logger) *FitnessHandler {
	return &FitnessHandler{db: db, logger: logger.With().Str("handler", "fitness").Logger()}
}

type WeekStat struct {
	Week  string `json:"week"`
	Count int    `json:"count"`
	KM    float64 `json:"km"`
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

type Workout struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
	Notes     string    `json:"notes"`
}

type FitnessSummaryResponse struct {
	ActivitiesThisWeek int     `json:"activities_this_week"`
	WorkoutsThisWeek   int     `json:"workouts_this_week"`
	DistanceThisWeek   float64 `json:"distance_km_this_week"`
	ActivitiesTotal    int     `json:"activities_total"`
	TotalDistanceKm    float64 `json:"total_distance_km"`
}

func (h *FitnessHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))

	var s FitnessSummaryResponse
	h.db.QueryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(distance_meters)/1000.0,0) FROM activities WHERE started_at >= $1`, weekStart).
		Scan(&s.ActivitiesThisWeek, &s.DistanceThisWeek)
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM workouts WHERE started_at >= $1`, weekStart).
		Scan(&s.WorkoutsThisWeek)
	h.db.QueryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(distance_meters)/1000.0,0) FROM activities`).
		Scan(&s.ActivitiesTotal, &s.TotalDistanceKm)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func (h *FitnessHandler) GetWeeklyStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := h.db.Query(ctx, `
		SELECT
			TO_CHAR(DATE_TRUNC('week', started_at), 'YYYY-MM-DD') as week,
			COUNT(*) as count,
			COALESCE(SUM(distance_meters)/1000.0, 0) as km
		FROM activities
		WHERE started_at >= NOW() - INTERVAL '10 weeks'
		GROUP BY DATE_TRUNC('week', started_at)
		ORDER BY DATE_TRUNC('week', started_at) ASC
	`)
	if err != nil {
		h.logger.Error().Err(err).Msg("query weekly stats")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	stats := make([]WeekStat, 0)
	for rows.Next() {
		var s WeekStat
		if err := rows.Scan(&s.Week, &s.Count, &s.KM); err != nil {
			continue
		}
		stats = append(stats, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *FitnessHandler) GetActivities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := h.db.Query(ctx, `
		SELECT id, type, COALESCE(name,''), started_at,
			duration_seconds, distance_meters, calories, avg_heart_rate
		FROM activities
		ORDER BY started_at DESC
		LIMIT 30
	`)
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

	rows, err := h.db.Query(ctx, `
		SELECT id, COALESCE(title,''), started_at, ended_at, COALESCE(notes,'')
		FROM workouts
		ORDER BY started_at DESC
		LIMIT 30
	`)
	if err != nil {
		h.logger.Error().Err(err).Msg("query workouts")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	workouts := make([]Workout, 0)
	for rows.Next() {
		var wk Workout
		if err := rows.Scan(&wk.ID, &wk.Title, &wk.StartedAt, &wk.EndedAt, &wk.Notes); err != nil {
			continue
		}
		workouts = append(workouts, wk)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workouts)
}
