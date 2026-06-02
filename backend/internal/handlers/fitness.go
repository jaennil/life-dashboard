package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
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

type FitnessGoldenCard struct {
	Key    string `json:"key"`
	Title  string `json:"title"`
	Value  string `json:"value"`
	Detail string `json:"detail"`
	Tone   string `json:"tone"`
}

type FitnessActivityTypeStat struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type StravaGoldenWeek struct {
	Week            string  `json:"week"`
	ActivityDays    int     `json:"activity_days"`
	ActivitiesCount int     `json:"activities_count"`
	KM              float64 `json:"km"`
}

type HevyGoldenWeek struct {
	Week          string `json:"week"`
	WorkoutsCount int    `json:"workouts_count"`
	SetsCount     int    `json:"sets_count"`
	PushCount     int    `json:"push_count"`
	PullCount     int    `json:"pull_count"`
	LegsCount     int    `json:"legs_count"`
	OtherCount    int    `json:"other_count"`
}

type FitnessSplitBucket struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

type FitnessProgressLift struct {
	Exercise string `json:"exercise"`
	Latest   string `json:"latest"`
	Previous string `json:"previous"`
	Delta    string `json:"delta"`
}

type StravaGoldenMetrics struct {
	Cards    []FitnessGoldenCard       `json:"cards"`
	TopTypes []FitnessActivityTypeStat `json:"top_types"`
	Weekly   []StravaGoldenWeek        `json:"weekly"`
}

type HevyGoldenMetrics struct {
	Cards        []FitnessGoldenCard   `json:"cards"`
	Splits       []FitnessSplitBucket  `json:"splits"`
	Progressions []FitnessProgressLift `json:"progressions"`
	Weekly       []HevyGoldenWeek      `json:"weekly"`
}

type FitnessGoldenMetricsResponse struct {
	Strava StravaGoldenMetrics `json:"strava"`
	Hevy   HevyGoldenMetrics   `json:"hevy"`
}

func (h *FitnessHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	now := time.Now().In(aiDisplayLocation)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, aiDisplayLocation)
	weekStart := todayStart.AddDate(0, 0, -int(todayStart.Weekday()))
	dateRange := parseQueryDateRange(r, weekStart, todayStart)

	var s FitnessSummaryResponse
	h.db.QueryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(distance_meters)/1000.0,0) FROM activities WHERE started_at >= $1 AND started_at < $3 AND user_id = $2`, dateRange.Start, userID, dateRange.EndExclusive).
		Scan(&s.ActivitiesThisWeek, &s.DistanceThisWeek)
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM workouts WHERE started_at >= $1 AND started_at < $3 AND user_id = $2`, dateRange.Start, userID, dateRange.EndExclusive).
		Scan(&s.WorkoutsThisWeek)
	h.db.QueryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(distance_meters)/1000.0,0) FROM activities WHERE user_id = $1 AND started_at >= $2 AND started_at < $3`, userID, dateRange.Start, dateRange.EndExclusive).
		Scan(&s.ActivitiesTotal, &s.TotalDistanceKm)
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM workouts WHERE user_id = $1 AND started_at >= $2 AND started_at < $3`, userID, dateRange.Start, dateRange.EndExclusive).
		Scan(&s.WorkoutsTotal)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func (h *FitnessHandler) GetGoldenMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	now := time.Now().In(aiDisplayLocation)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, aiDisplayLocation)
	activityRange := parseQueryDateRange(r, todayStart.AddDate(0, 0, -56), todayStart)
	workoutRange := parseQueryDateRange(r, todayStart.AddDate(0, 0, -89), todayStart)

	activities, err := h.loadActivitiesInRange(ctx, userID, activityRange.Start, activityRange.EndExclusive)
	if err != nil {
		h.logger.Error().Err(err).Msg("query fitness golden activities")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	workouts, err := h.loadHevyWorkoutsInRange(ctx, userID, workoutRange.Start, workoutRange.EndExclusive)
	if err != nil {
		h.logger.Error().Err(err).Msg("query fitness golden workouts")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := FitnessGoldenMetricsResponse{
		Strava: buildStravaGoldenMetrics(now, activities),
		Hevy:   buildHevyGoldenMetrics(now, workouts),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *FitnessHandler) GetWeeklyStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	now := time.Now().In(aiDisplayLocation)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, aiDisplayLocation)
	dateRange := parseQueryDateRange(r, todayStart.AddDate(0, 0, -69), todayStart)

	rows, err := h.db.Query(ctx, `
		WITH activity_stats AS (
			SELECT
				DATE_TRUNC('week', started_at) AS week,
				COUNT(*) AS activities_count,
				COALESCE(SUM(distance_meters) / 1000.0, 0) AS km
			FROM activities
			WHERE started_at >= $2 AND started_at < $3 AND user_id = $1
			GROUP BY DATE_TRUNC('week', started_at)
		),
		workout_stats AS (
			SELECT
				DATE_TRUNC('week', started_at) AS week,
				COUNT(*) AS workouts_count
			FROM workouts
			WHERE started_at >= $2 AND started_at < $3 AND user_id = $1
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
	`, userID, dateRange.Start, dateRange.EndExclusive)
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

func (h *FitnessHandler) loadActivitiesInRange(ctx context.Context, userID string, since, until time.Time) ([]Activity, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, type, COALESCE(name,''), started_at,
			duration_seconds, distance_meters, calories, avg_heart_rate
		FROM activities
		WHERE user_id = $1
			AND started_at >= $2
			AND started_at < $3
		ORDER BY started_at DESC
	`, userID, since, until)
	if err != nil {
		return nil, err
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

	return activities, rows.Err()
}

func (h *FitnessHandler) GetActivities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	now := time.Now().In(aiDisplayLocation)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, aiDisplayLocation)
	dateRange := parseQueryDateRange(r, todayStart.AddDate(0, 0, -29), todayStart)

	rows, err := h.db.Query(ctx, `
		SELECT id, type, COALESCE(name,''), started_at,
			duration_seconds, distance_meters, calories, avg_heart_rate
		FROM activities
		WHERE user_id = $1
			AND started_at >= $2
			AND started_at < $3
		ORDER BY started_at DESC
		LIMIT 30
	`, userID, dateRange.Start, dateRange.EndExclusive)
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
	now := time.Now().In(aiDisplayLocation)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, aiDisplayLocation)
	dateRange := parseQueryDateRange(r, todayStart.AddDate(0, 0, -29), todayStart)

	workouts, err := h.loadHevyWorkoutsInRange(ctx, userID, dateRange.Start, dateRange.EndExclusive)
	if err != nil {
		h.logger.Error().Err(err).Msg("query workouts")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(workouts) > 30 {
		workouts = workouts[:30]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workouts)
}

func (h *FitnessHandler) loadHevyWorkoutsInRange(ctx context.Context, userID string, since, until time.Time) ([]Workout, error) {
	args := []any{userID, since, until}
	query := `
		SELECT id, source, COALESCE(title,''), COALESCE(notes,''), started_at, ended_at, raw_payload
		FROM workouts
		WHERE user_id = $1
			AND source = 'hevy'
			AND started_at >= $2
			AND started_at < $3
	`
	query += ` ORDER BY started_at DESC`

	type workoutRow struct {
		workout    Workout
		rawPayload []byte
	}

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
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

	return workouts, rows.Err()
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

func buildStravaGoldenMetrics(now time.Time, activities []Activity) StravaGoldenMetrics {
	start7 := startOfLocalDay(now).AddDate(0, 0, -6)
	start28 := startOfLocalDay(now).AddDate(0, 0, -27)
	prev28Start := start28.AddDate(0, 0, -28)

	var activities7 int
	var distance7Km float64
	var duration7Seconds int
	activeDays7 := make(map[string]struct{})
	typeCounts := make(map[string]int)
	var last28DistanceKm float64
	var prev28DistanceKm float64

	for _, activity := range activities {
		startedAt := activity.StartedAt.In(aiDisplayLocation)
		dayKey := startedAt.Format("2006-01-02")
		typeCounts[activity.Type]++

		if !startedAt.Before(start7) {
			activities7++
			activeDays7[dayKey] = struct{}{}
			if activity.DistanceMeters != nil {
				distance7Km += *activity.DistanceMeters / 1000
			}
			if activity.DurationSeconds != nil {
				duration7Seconds += *activity.DurationSeconds
			}
		}

		if !startedAt.Before(start28) {
			if activity.DistanceMeters != nil {
				last28DistanceKm += *activity.DistanceMeters / 1000
			}
			continue
		}
		if !startedAt.Before(prev28Start) {
			if activity.DistanceMeters != nil {
				prev28DistanceKm += *activity.DistanceMeters / 1000
			}
		}
	}

	topTypes := make([]FitnessActivityTypeStat, 0, len(typeCounts))
	for activityType, count := range typeCounts {
		topTypes = append(topTypes, FitnessActivityTypeStat{Type: activityType, Count: count})
	}
	sort.Slice(topTypes, func(i, j int) bool {
		if topTypes[i].Count == topTypes[j].Count {
			return topTypes[i].Type < topTypes[j].Type
		}
		return topTypes[i].Count > topTypes[j].Count
	})
	if len(topTypes) > 4 {
		topTypes = topTypes[:4]
	}

	var lastActivity *Activity
	if len(activities) > 0 {
		lastActivity = &activities[0]
	}

	trendValue := "—"
	trendDetail := "недостаточно истории для сравнения 28д"
	trendTone := "muted"
	if last28DistanceKm > 0 || prev28DistanceKm > 0 {
		changePct := percentDelta(last28DistanceKm, prev28DistanceKm)
		trendValue = formatSignedPercent(changePct)
		trendDetail = fmt.Sprintf("%s км vs %s км за прошлые 28д", formatCompactFloat(last28DistanceKm), formatCompactFloat(prev28DistanceKm))
		switch {
		case changePct >= 10:
			trendTone = "success"
		case changePct <= -10:
			trendTone = "danger"
		default:
			trendTone = "warning"
		}
	}

	varietyValue := fmt.Sprintf("%d типа", len(typeCounts))
	varietyDetail := "активностей пока нет"
	varietyTone := "muted"
	if len(topTypes) > 0 {
		topParts := make([]string, 0, len(topTypes))
		for _, item := range topTypes {
			topParts = append(topParts, fmt.Sprintf("%s %d", item.Type, item.Count))
		}
		varietyDetail = strings.Join(topParts, " · ")
		switch {
		case len(typeCounts) >= 3:
			varietyTone = "success"
		case len(typeCounts) == 2:
			varietyTone = "warning"
		default:
			varietyTone = "danger"
		}
	}

	recencyValue := "нет данных"
	recencyDetail := "активности ещё не синхронизированы"
	recencyTone := "muted"
	if lastActivity != nil {
		days := daysSince(now, lastActivity.StartedAt)
		recencyValue = formatDaysSince(days)
		recencyDetail = fmt.Sprintf("последняя активность %s", lastActivity.StartedAt.In(aiDisplayLocation).Format("02.01 15:04"))
		switch {
		case days <= 2:
			recencyTone = "success"
		case days <= 5:
			recencyTone = "warning"
		default:
			recencyTone = "danger"
		}
	}

	cards := []FitnessGoldenCard{
		{
			Key:    "consistency",
			Title:  "Режим",
			Value:  fmt.Sprintf("%d д/7", len(activeDays7)),
			Detail: fmt.Sprintf("%d активностей · %s км за 7д", activities7, formatCompactFloat(distance7Km)),
			Tone:   toneForThreshold(len(activeDays7), 4, 2),
		},
		{
			Key:    "volume",
			Title:  "Объём",
			Value:  fmt.Sprintf("%s км", formatCompactFloat(distance7Km)),
			Detail: fmt.Sprintf("%s ч движения за 7д", formatCompactFloat(float64(duration7Seconds)/3600)),
			Tone:   toneForFloat(distance7Km, 15, 5),
		},
		{
			Key:    "trend",
			Title:  "Тренд",
			Value:  trendValue,
			Detail: trendDetail,
			Tone:   trendTone,
		},
		{
			Key:    "variety",
			Title:  "Разнообразие",
			Value:  varietyValue,
			Detail: varietyDetail,
			Tone:   varietyTone,
		},
		{
			Key:    "recency",
			Title:  "Свежесть",
			Value:  recencyValue,
			Detail: recencyDetail,
			Tone:   recencyTone,
		},
	}

	return StravaGoldenMetrics{
		Cards:    cards,
		TopTypes: topTypes,
		Weekly:   buildStravaGoldenWeekly(now, activities, 8),
	}
}

func buildHevyGoldenMetrics(now time.Time, workouts []Workout) HevyGoldenMetrics {
	start7 := startOfLocalDay(now).AddDate(0, 0, -6)
	start30 := startOfLocalDay(now).AddDate(0, 0, -29)
	prev7Start := start7.AddDate(0, 0, -7)

	var workouts7, workouts30 int
	var sets7, prevSets7 int
	activeDays7 := make(map[string]struct{})
	splitCounts := map[string]int{"push": 0, "pull": 0, "legs": 0, "other": 0}
	latestLegDays := -1

	progressions := buildExerciseProgressions(workouts)

	for _, workout := range workouts {
		startedAt := workout.StartedAt.In(aiDisplayLocation)
		if !startedAt.Before(start7) {
			workouts7++
			activeDays7[startedAt.Format("2006-01-02")] = struct{}{}
			sets7 += countWorkoutSets(workout)
		} else if !startedAt.Before(prev7Start) {
			prevSets7 += countWorkoutSets(workout)
		}

		if !startedAt.Before(start30) {
			workouts30++
			split := classifyWorkoutSplit(workout)
			splitCounts[split]++
			if split == "legs" {
				days := daysSince(now, workout.StartedAt)
				if latestLegDays == -1 || days < latestLegDays {
					latestLegDays = days
				}
			}
		}
	}

	splits := []FitnessSplitBucket{
		{Key: "push", Label: "Push", Count: splitCounts["push"]},
		{Key: "pull", Label: "Pull", Count: splitCounts["pull"]},
		{Key: "legs", Label: "Legs", Count: splitCounts["legs"]},
		{Key: "other", Label: "Other", Count: splitCounts["other"]},
	}

	avgSetsPerWorkout := 0.0
	if workouts7 > 0 {
		avgSetsPerWorkout = float64(sets7) / float64(workouts7)
	}

	progressDetail := "недостаточно истории по рабочим сетам"
	progressValue := "—"
	progressTone := "muted"
	if len(progressions) > 0 {
		progressValue = fmt.Sprintf("%d в росте", len(progressions))
		progressDetail = fmt.Sprintf("%s %s", progressions[0].Exercise, progressions[0].Delta)
		progressTone = "success"
	}

	balanceValue := fmt.Sprintf("%d·%d·%d", splitCounts["push"], splitCounts["pull"], splitCounts["legs"])
	balanceDetail, balanceTone := describeSplitBalance(splitCounts)

	recencyValue := "нет данных"
	recencyDetail := "силовые тренировки ещё не синхронизированы"
	recencyTone := "muted"
	if len(workouts) > 0 {
		lastWorkoutDays := daysSince(now, workouts[0].StartedAt)
		recencyValue = formatDaysSince(lastWorkoutDays)
		if latestLegDays >= 0 {
			recencyDetail = fmt.Sprintf("последняя тренировка %s · ноги %s", workouts[0].StartedAt.In(aiDisplayLocation).Format("02.01"), formatDaysSince(latestLegDays))
		} else {
			recencyDetail = fmt.Sprintf("последняя тренировка %s · ноги не найдены", workouts[0].StartedAt.In(aiDisplayLocation).Format("02.01"))
		}
		switch {
		case lastWorkoutDays <= 2:
			recencyTone = "success"
		case lastWorkoutDays <= 5:
			recencyTone = "warning"
		default:
			recencyTone = "danger"
		}
	}

	cards := []FitnessGoldenCard{
		{
			Key:    "consistency",
			Title:  "Режим",
			Value:  fmt.Sprintf("%d трен./7д", workouts7),
			Detail: fmt.Sprintf("%d активных дней · %d тренировок за 30д", len(activeDays7), workouts30),
			Tone:   toneForThreshold(workouts7, 3, 1),
		},
		{
			Key:    "volume",
			Title:  "Объём",
			Value:  fmt.Sprintf("%d сетов", sets7),
			Detail: fmt.Sprintf("%s сет./трен. · %s к прошлой неделе", formatCompactFloat(avgSetsPerWorkout), formatSignedInt(sets7-prevSets7)),
			Tone:   toneForThreshold(sets7, 36, 18),
		},
		{
			Key:    "progression",
			Title:  "Прогресс",
			Value:  progressValue,
			Detail: progressDetail,
			Tone:   progressTone,
		},
		{
			Key:    "balance",
			Title:  "Баланс",
			Value:  balanceValue,
			Detail: balanceDetail,
			Tone:   balanceTone,
		},
		{
			Key:    "recency",
			Title:  "Свежесть",
			Value:  recencyValue,
			Detail: recencyDetail,
			Tone:   recencyTone,
		},
	}

	return HevyGoldenMetrics{
		Cards:        cards,
		Splits:       splits,
		Progressions: progressions,
		Weekly:       buildHevyGoldenWeekly(now, workouts, 8),
	}
}

func buildStravaGoldenWeekly(now time.Time, activities []Activity, weeks int) []StravaGoldenWeek {
	type weekAcc struct {
		activities int
		km         float64
		days       map[string]struct{}
	}

	start := startOfLocalWeek(now).AddDate(0, 0, -7*(weeks-1))
	byWeek := make(map[string]*weekAcc, weeks)
	for i := 0; i < weeks; i++ {
		weekStart := start.AddDate(0, 0, 7*i)
		key := weekStart.Format("2006-01-02")
		byWeek[key] = &weekAcc{days: make(map[string]struct{})}
	}

	for _, activity := range activities {
		local := activity.StartedAt.In(aiDisplayLocation)
		weekStart := startOfLocalWeek(local)
		key := weekStart.Format("2006-01-02")
		acc, ok := byWeek[key]
		if !ok {
			continue
		}
		acc.activities++
		acc.days[local.Format("2006-01-02")] = struct{}{}
		if activity.DistanceMeters != nil {
			acc.km += *activity.DistanceMeters / 1000
		}
	}

	result := make([]StravaGoldenWeek, 0, weeks)
	for i := 0; i < weeks; i++ {
		weekStart := start.AddDate(0, 0, 7*i)
		key := weekStart.Format("2006-01-02")
		acc := byWeek[key]
		result = append(result, StravaGoldenWeek{
			Week:            key,
			ActivityDays:    len(acc.days),
			ActivitiesCount: acc.activities,
			KM:              acc.km,
		})
	}
	return result
}

func buildHevyGoldenWeekly(now time.Time, workouts []Workout, weeks int) []HevyGoldenWeek {
	type weekAcc struct {
		workouts int
		sets     int
		push     int
		pull     int
		legs     int
		other    int
	}

	start := startOfLocalWeek(now).AddDate(0, 0, -7*(weeks-1))
	byWeek := make(map[string]*weekAcc, weeks)
	for i := 0; i < weeks; i++ {
		weekStart := start.AddDate(0, 0, 7*i)
		key := weekStart.Format("2006-01-02")
		byWeek[key] = &weekAcc{}
	}

	for _, workout := range workouts {
		local := workout.StartedAt.In(aiDisplayLocation)
		weekStart := startOfLocalWeek(local)
		key := weekStart.Format("2006-01-02")
		acc, ok := byWeek[key]
		if !ok {
			continue
		}
		acc.workouts++
		acc.sets += countWorkoutSets(workout)
		switch classifyWorkoutSplit(workout) {
		case "push":
			acc.push++
		case "pull":
			acc.pull++
		case "legs":
			acc.legs++
		default:
			acc.other++
		}
	}

	result := make([]HevyGoldenWeek, 0, weeks)
	for i := 0; i < weeks; i++ {
		weekStart := start.AddDate(0, 0, 7*i)
		key := weekStart.Format("2006-01-02")
		acc := byWeek[key]
		result = append(result, HevyGoldenWeek{
			Week:          key,
			WorkoutsCount: acc.workouts,
			SetsCount:     acc.sets,
			PushCount:     acc.push,
			PullCount:     acc.pull,
			LegsCount:     acc.legs,
			OtherCount:    acc.other,
		})
	}
	return result
}

func countWorkoutSets(workout Workout) int {
	total := 0
	for _, exercise := range workout.Exercises {
		total += len(exercise.Sets)
	}
	return total
}

func startOfLocalWeek(value time.Time) time.Time {
	local := startOfLocalDay(value)
	weekday := int(local.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return local.AddDate(0, 0, -(weekday - 1))
}

func startOfLocalDay(value time.Time) time.Time {
	local := value.In(aiDisplayLocation)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, aiDisplayLocation)
}

func daysSince(now, then time.Time) int {
	nowDay := startOfLocalDay(now)
	thenDay := startOfLocalDay(then)
	return int(nowDay.Sub(thenDay).Hours() / 24)
}

func formatDaysSince(days int) string {
	switch days {
	case 0:
		return "сегодня"
	case 1:
		return "1 д"
	default:
		return fmt.Sprintf("%d д", days)
	}
}

func formatCompactFloat(value float64) string {
	if math.Abs(value-math.Round(value)) < 0.05 {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("%.1f", value)
}

func formatSignedPercent(value float64) string {
	if math.Abs(value) < 0.5 {
		return "0%"
	}
	if value > 0 {
		return fmt.Sprintf("+%.0f%%", value)
	}
	return fmt.Sprintf("%.0f%%", value)
}

func formatSignedInt(value int) string {
	if value > 0 {
		return fmt.Sprintf("+%d", value)
	}
	return fmt.Sprintf("%d", value)
}

func toneForThreshold(value, success, warning int) string {
	switch {
	case value >= success:
		return "success"
	case value >= warning:
		return "warning"
	case value == 0:
		return "danger"
	default:
		return "danger"
	}
}

func toneForFloat(value, success, warning float64) string {
	switch {
	case value >= success:
		return "success"
	case value >= warning:
		return "warning"
	case value == 0:
		return "danger"
	default:
		return "danger"
	}
}

func percentDelta(current, previous float64) float64 {
	if previous <= 0 {
		if current <= 0 {
			return 0
		}
		return 100
	}
	return ((current - previous) / previous) * 100
}

type exerciseSnapshot struct {
	DisplayName string
	StartedAt   time.Time
	Score       float64
	WeightKg    float64
	Reps        int
}

func buildExerciseProgressions(workouts []Workout) []FitnessProgressLift {
	byExercise := make(map[string][]exerciseSnapshot)

	for _, workout := range workouts {
		for _, exercise := range workout.Exercises {
			best, ok := bestWorkingSet(exercise)
			if !ok {
				continue
			}
			key := normalizeExerciseName(exercise.Name)
			if key == "" {
				continue
			}
			byExercise[key] = append(byExercise[key], exerciseSnapshot{
				DisplayName: exercise.Name,
				StartedAt:   workout.StartedAt,
				Score:       best.WeightKg * (1 + float64(best.Reps)/30),
				WeightKg:    best.WeightKg,
				Reps:        best.Reps,
			})
		}
	}

	progressions := make([]struct {
		FitnessProgressLift
		deltaScore float64
	}, 0)

	for _, sessions := range byExercise {
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].StartedAt.After(sessions[j].StartedAt)
		})
		if len(sessions) < 2 {
			continue
		}
		latest := sessions[0]
		var previous *exerciseSnapshot
		for i := 1; i < len(sessions); i++ {
			if !sameLocalDay(sessions[i].StartedAt, latest.StartedAt) {
				previous = &sessions[i]
				break
			}
		}
		if previous == nil {
			continue
		}
		if latest.Score <= previous.Score*1.015 && latest.WeightKg <= previous.WeightKg && latest.Reps <= previous.Reps {
			continue
		}

		delta := describeLiftDelta(latest, *previous)
		progressions = append(progressions, struct {
			FitnessProgressLift
			deltaScore float64
		}{
			FitnessProgressLift: FitnessProgressLift{
				Exercise: latest.DisplayName,
				Latest:   formatLift(latest.WeightKg, latest.Reps),
				Previous: formatLift(previous.WeightKg, previous.Reps),
				Delta:    delta,
			},
			deltaScore: latest.Score - previous.Score,
		})
	}

	sort.Slice(progressions, func(i, j int) bool {
		if math.Abs(progressions[i].deltaScore-progressions[j].deltaScore) < 0.01 {
			return progressions[i].Exercise < progressions[j].Exercise
		}
		return progressions[i].deltaScore > progressions[j].deltaScore
	})

	result := make([]FitnessProgressLift, 0, min(4, len(progressions)))
	for _, item := range progressions {
		result = append(result, item.FitnessProgressLift)
		if len(result) == 4 {
			break
		}
	}
	return result
}

type workingSet struct {
	WeightKg float64
	Reps     int
	Score    float64
}

func bestWorkingSet(exercise WorkoutExercise) (workingSet, bool) {
	best := workingSet{}
	found := false
	for _, set := range exercise.Sets {
		if set.WeightKg == nil || set.Reps == nil || *set.Reps <= 0 {
			continue
		}
		if strings.EqualFold(set.SetType, "warm-up") {
			continue
		}
		score := *set.WeightKg * (1 + float64(*set.Reps)/30)
		if !found || score > best.Score {
			best = workingSet{
				WeightKg: *set.WeightKg,
				Reps:     *set.Reps,
				Score:    score,
			}
			found = true
		}
	}
	return best, found
}

func sameLocalDay(a, b time.Time) bool {
	aa := a.In(aiDisplayLocation)
	bb := b.In(aiDisplayLocation)
	return aa.Year() == bb.Year() && aa.YearDay() == bb.YearDay()
}

func formatLift(weightKg float64, reps int) string {
	return fmt.Sprintf("%s×%d", formatCompactFloat(weightKg), reps)
}

func describeLiftDelta(latest, previous exerciseSnapshot) string {
	if latest.WeightKg > previous.WeightKg {
		return fmt.Sprintf("+%s кг", formatCompactFloat(latest.WeightKg-previous.WeightKg))
	}
	if latest.Reps > previous.Reps {
		return fmt.Sprintf("+%d повт.", latest.Reps-previous.Reps)
	}
	return fmt.Sprintf("+%s%%", formatCompactFloat(percentDelta(latest.Score, previous.Score)))
}

func normalizeExerciseName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(name))), " ")
}

func classifyWorkoutSplit(workout Workout) string {
	texts := []string{strings.ToLower(workout.Title)}
	for _, exercise := range workout.Exercises {
		texts = append(texts, strings.ToLower(exercise.Name))
		texts = append(texts, strings.ToLower(exercise.Category))
	}
	text := strings.Join(texts, " ")

	switch {
	case fitnessContainsAny(text, "legs", "leg", "squat", "lunge", "rdl", "deadlift", "hamstring", "quad", "calf", "glute", "hip thrust"):
		return "legs"
	case fitnessContainsAny(text, "push", "chest", "shoulder", "tricep", "bench", "press", "fly", "dip", "lateral raise", "arnold"):
		if !fitnessContainsAny(text, "pull", "row", "curl", "lat pulldown") {
			return "push"
		}
	case fitnessContainsAny(text, "pull", "back", "bicep", "row", "curl", "pulldown", "face pull", "shrug"):
		if !fitnessContainsAny(text, "squat", "leg", "hamstring", "quad", "rdl") {
			return "pull"
		}
	}

	pushScore := countMatches(text, "push", "chest", "shoulder", "tricep", "bench", "press", "fly", "dip", "lateral raise", "arnold")
	pullScore := countMatches(text, "pull", "back", "bicep", "row", "curl", "pulldown", "face pull", "shrug")
	legsScore := countMatches(text, "legs", "leg", "squat", "lunge", "rdl", "deadlift", "hamstring", "quad", "calf", "glute", "hip thrust")

	switch max(pushScore, pullScore, legsScore) {
	case 0:
		return "other"
	case pushScore:
		return "push"
	case pullScore:
		return "pull"
	default:
		return "legs"
	}
}

func describeSplitBalance(counts map[string]int) (string, string) {
	push := counts["push"]
	pull := counts["pull"]
	legs := counts["legs"]
	minCount := min(push, pull, legs)
	maxCount := max(push, pull, legs)

	switch {
	case push == 0 && pull == 0 && legs == 0:
		return "сплит ещё не накопился", "muted"
	case legs == 0:
		return "ноги выпали из сплита", "danger"
	case maxCount-minCount <= 1:
		return "push / pull / legs держатся ровно", "success"
	case legs < push || legs < pull:
		return "ноги отстают от верха", "warning"
	default:
		return "есть перекос по сплиту", "warning"
	}
}

func fitnessContainsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func countMatches(text string, needles ...string) int {
	total := 0
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			total++
		}
	}
	return total
}
