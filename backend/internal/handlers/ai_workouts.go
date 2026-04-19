package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type AIWorkoutExerciseStat struct {
	ExerciseName string `json:"exercise_name"`
	SetCount     int    `json:"set_count"`
}

type AIWorkoutOverviewData struct {
	WorkoutCount int                     `json:"workout_count"`
	ActiveDays   int                     `json:"active_days"`
	SetCount     int                     `json:"set_count"`
	TopExercises []AIWorkoutExerciseStat `json:"top_exercises,omitempty"`
}

type AIRecentWorkoutsData struct {
	Count    int       `json:"count"`
	Workouts []Workout `json:"workouts,omitempty"`
}

type AIRoutineSet struct {
	SetIndex        int      `json:"set_index"`
	SetType         string   `json:"set_type"`
	WeightKg        *float64 `json:"weight_kg,omitempty"`
	Reps            *int     `json:"reps,omitempty"`
	DistanceMeters  *float64 `json:"distance_meters,omitempty"`
	DurationSeconds *int     `json:"duration_seconds,omitempty"`
}

type AIRoutineExercise struct {
	Index       int            `json:"index"`
	Name        string         `json:"name"`
	Notes       string         `json:"notes,omitempty"`
	TemplateID  string         `json:"template_id,omitempty"`
	SupersetID  string         `json:"superset_id,omitempty"`
	RestSeconds *int           `json:"rest_seconds,omitempty"`
	Sets        []AIRoutineSet `json:"sets,omitempty"`
}

type AIRoutine struct {
	ID              string              `json:"id"`
	ExternalID      string              `json:"external_id"`
	Title           string              `json:"title"`
	FolderID        *int64              `json:"folder_id,omitempty"`
	UsageCount      int                 `json:"usage_count"`
	LastUsedAt      *time.Time          `json:"last_used_at,omitempty"`
	SourceCreatedAt *time.Time          `json:"source_created_at,omitempty"`
	SourceUpdatedAt *time.Time          `json:"source_updated_at,omitempty"`
	Exercises       []AIRoutineExercise `json:"exercises,omitempty"`
}

type AIRoutineOverviewData struct {
	Source     string      `json:"source"`
	TotalCount int         `json:"total_count"`
	Routines   []AIRoutine `json:"routines,omitempty"`
}

func (h *AIHandler) buildWorkoutOverviewInRange(ctx context.Context, userID string, start, end time.Time) (AIWorkoutOverviewData, error) {
	data := AIWorkoutOverviewData{}

	if err := h.db.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(DISTINCT DATE(started_at))
		FROM workouts
		WHERE user_id = $1
			AND started_at >= $2
			AND started_at < $3
	`, userID, start, end).Scan(&data.WorkoutCount, &data.ActiveDays); err != nil {
		return AIWorkoutOverviewData{}, err
	}

	if err := h.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM workout_sets ws
		JOIN workouts w ON w.id = ws.workout_id
		WHERE w.user_id = $1
			AND w.started_at >= $2
			AND w.started_at < $3
	`, userID, start, end).Scan(&data.SetCount); err != nil {
		return AIWorkoutOverviewData{}, err
	}

	rows, err := h.db.Query(ctx, `
		SELECT exercise_name, COUNT(*)
		FROM workout_sets ws
		JOIN workouts w ON w.id = ws.workout_id
		WHERE w.user_id = $1
			AND w.started_at >= $2
			AND w.started_at < $3
		GROUP BY exercise_name
		ORDER BY COUNT(*) DESC, exercise_name ASC
		LIMIT 5
	`, userID, start, end)
	if err != nil {
		return AIWorkoutOverviewData{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var item AIWorkoutExerciseStat
		if err := rows.Scan(&item.ExerciseName, &item.SetCount); err != nil {
			return AIWorkoutOverviewData{}, err
		}
		data.TopExercises = append(data.TopExercises, item)
	}
	if err := rows.Err(); err != nil {
		return AIWorkoutOverviewData{}, err
	}

	return data, nil
}

func (h *AIHandler) buildRecentWorkoutsData(ctx context.Context, userID string, limit int) (AIRecentWorkoutsData, error) {
	if limit <= 0 {
		limit = 4
	}

	rows, err := h.db.Query(ctx, `
		SELECT id, source, started_at, ended_at, COALESCE(title,''), COALESCE(notes,''), raw_payload
		FROM workouts
		WHERE user_id = $1
		ORDER BY started_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return AIRecentWorkoutsData{}, err
	}
	defer rows.Close()

	fitnessHelper := &FitnessHandler{db: h.db, logger: h.logger}
	data := AIRecentWorkoutsData{
		Workouts: make([]Workout, 0, limit),
	}

	for rows.Next() {
		var workout Workout
		var rawPayload []byte
		if err := rows.Scan(
			&workout.ID,
			&workout.Source,
			&workout.StartedAt,
			&workout.EndedAt,
			&workout.Title,
			&workout.Notes,
			&rawPayload,
		); err != nil {
			return AIRecentWorkoutsData{}, err
		}

		if workout.Source == "hevy" && len(rawPayload) > 0 {
			if err := hydrateHevyWorkout(&workout, rawPayload); err != nil {
				h.logger.Warn().Str("workout_id", workout.ID).Err(err).Msg("failed to hydrate hevy workout for ai data")
			}
		}
		if len(workout.Exercises) == 0 {
			if err := fitnessHelper.loadNormalizedWorkoutExercises(ctx, &workout); err != nil {
				h.logger.Warn().Str("workout_id", workout.ID).Err(err).Msg("failed to load workout exercises for ai data")
			}
		}

		data.Workouts = append(data.Workouts, workout)
	}
	if err := rows.Err(); err != nil {
		return AIRecentWorkoutsData{}, err
	}
	data.Count = len(data.Workouts)

	return data, nil
}

func (h *AIHandler) buildRoutineOverviewData(ctx context.Context, userID string, limit int) (AIRoutineOverviewData, error) {
	if limit <= 0 {
		limit = 4
	}

	data := AIRoutineOverviewData{Source: "hevy_routines"}
	if err := h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM workout_routines WHERE user_id = $1
	`, userID).Scan(&data.TotalCount); err != nil {
		return AIRoutineOverviewData{}, err
	}

	rows, err := h.db.Query(ctx, `
		SELECT
			r.id,
			r.external_id,
			r.title,
			r.folder_id,
			r.source_created_at,
			r.source_updated_at,
			COALESCE(stats.usage_count, 0),
			stats.last_used_at
		FROM workout_routines r
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS usage_count, MAX(started_at) AS last_used_at
			FROM workouts w
			WHERE w.user_id = r.user_id
				AND w.routine_external_id = r.external_id
		) stats ON true
		WHERE r.user_id = $1
		ORDER BY COALESCE(stats.last_used_at, r.source_updated_at, r.created_at) DESC NULLS LAST, r.title ASC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return AIRoutineOverviewData{}, err
	}
	defer rows.Close()

	data.Routines = make([]AIRoutine, 0, limit)
	for rows.Next() {
		var routine AIRoutine
		if err := rows.Scan(
			&routine.ID,
			&routine.ExternalID,
			&routine.Title,
			&routine.FolderID,
			&routine.SourceCreatedAt,
			&routine.SourceUpdatedAt,
			&routine.UsageCount,
			&routine.LastUsedAt,
		); err != nil {
			return AIRoutineOverviewData{}, err
		}

		exercises, err := h.buildRoutineExercisesData(ctx, routine.ID)
		if err != nil {
			return AIRoutineOverviewData{}, err
		}
		routine.Exercises = exercises
		data.Routines = append(data.Routines, routine)
	}
	if err := rows.Err(); err != nil {
		return AIRoutineOverviewData{}, err
	}

	return data, nil
}

func (h *AIHandler) buildRoutineExercisesData(ctx context.Context, routineID string) ([]AIRoutineExercise, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, exercise_index, exercise_name, COALESCE(notes, ''), COALESCE(template_id, ''), COALESCE(superset_id, ''), rest_seconds
		FROM routine_exercises
		WHERE routine_id = $1
		ORDER BY exercise_index ASC, exercise_name ASC
	`, routineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	exercises := make([]AIRoutineExercise, 0)
	for rows.Next() {
		var exerciseID string
		var exercise AIRoutineExercise
		if err := rows.Scan(
			&exerciseID,
			&exercise.Index,
			&exercise.Name,
			&exercise.Notes,
			&exercise.TemplateID,
			&exercise.SupersetID,
			&exercise.RestSeconds,
		); err != nil {
			return nil, err
		}

		sets, err := h.buildRoutineSetsData(ctx, exerciseID)
		if err != nil {
			return nil, err
		}
		exercise.Sets = sets
		exercises = append(exercises, exercise)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return exercises, nil
}

func (h *AIHandler) buildRoutineSetsData(ctx context.Context, exerciseID string) ([]AIRoutineSet, error) {
	rows, err := h.db.Query(ctx, `
		SELECT set_index, COALESCE(set_type, 'normal'), weight_kg, reps, distance_meters, duration_seconds
		FROM routine_sets
		WHERE routine_exercise_id = $1
		ORDER BY set_index ASC
	`, exerciseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sets := make([]AIRoutineSet, 0)
	for rows.Next() {
		var set AIRoutineSet
		if err := rows.Scan(
			&set.SetIndex,
			&set.SetType,
			&set.WeightKg,
			&set.Reps,
			&set.DistanceMeters,
			&set.DurationSeconds,
		); err != nil {
			return nil, err
		}
		sets = append(sets, set)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return sets, nil
}

func renderWorkoutOverviewText(title string, data AIWorkoutOverviewData) string {
	var sb strings.Builder
	sb.WriteString(title + "\n")
	sb.WriteString(fmt.Sprintf("За период: %d тренировок, %d активных дней, %d подходов\n", data.WorkoutCount, data.ActiveDays, data.SetCount))
	sb.WriteString("Топ упражнений по подходам:\n")
	if len(data.TopExercises) == 0 {
		sb.WriteString("  - Нет данных по упражнениям за период\n")
	} else {
		for _, item := range data.TopExercises {
			sb.WriteString(fmt.Sprintf("  - %s: %d подходов\n", item.ExerciseName, item.SetCount))
		}
	}
	return strings.TrimSpace(sb.String())
}

func renderRecentWorkoutsText(title string, data AIRecentWorkoutsData) string {
	var sb strings.Builder
	sb.WriteString(title + "\n")
	if len(data.Workouts) == 0 {
		sb.WriteString("Нет тренировок.\n")
		return strings.TrimSpace(sb.String())
	}
	for _, workout := range data.Workouts {
		sb.WriteString(formatAIWorkoutContext(workout))
	}
	return strings.TrimSpace(sb.String())
}

func renderRoutineOverviewText(title string, data AIRoutineOverviewData) string {
	var sb strings.Builder
	sb.WriteString(title + "\n")
	sb.WriteString("Это шаблоны тренировок из Hevy. Они показывают план упражнений и плановые веса/повторы, а не факт выполнения.\n")
	if data.TotalCount > 0 {
		sb.WriteString(fmt.Sprintf("Всего routines в Hevy: %d\n", data.TotalCount))
	}
	if len(data.Routines) == 0 {
		sb.WriteString("Нет routines.\n")
		return strings.TrimSpace(sb.String())
	}

	for _, routine := range data.Routines {
		sb.WriteString(fmt.Sprintf("Routine: %s\n", routine.Title))
		sb.WriteString(fmt.Sprintf("  external_id: %s\n", routine.ExternalID))
		if routine.FolderID != nil {
			sb.WriteString(fmt.Sprintf("  folder_id: %d\n", *routine.FolderID))
		}
		sb.WriteString(fmt.Sprintf("  выполнений по этой routine: %d\n", routine.UsageCount))
		if routine.LastUsedAt != nil {
			sb.WriteString(fmt.Sprintf("  последний раз использовалась: %s\n", formatAITimestampLocal(*routine.LastUsedAt, "02.01.2006 15:04")))
		}
		if routine.SourceUpdatedAt != nil {
			sb.WriteString(fmt.Sprintf("  обновлено в Hevy: %s\n", formatAITimestampLocal(*routine.SourceUpdatedAt, "02.01.2006 15:04")))
		}
		if routine.SourceCreatedAt != nil {
			sb.WriteString(fmt.Sprintf("  создано в Hevy: %s\n", formatAITimestampLocal(*routine.SourceCreatedAt, "02.01.2006 15:04")))
		}
		for _, exercise := range routine.Exercises {
			header := fmt.Sprintf("  Упражнение %d: %s", exercise.Index+1, exercise.Name)
			if exercise.TemplateID != "" {
				header += fmt.Sprintf(" [tpl %s]", exercise.TemplateID)
			}
			if exercise.SupersetID != "" {
				header += fmt.Sprintf(" [superset %s]", exercise.SupersetID)
			}
			sb.WriteString(header + "\n")
			if exercise.Notes != "" {
				sb.WriteString(fmt.Sprintf("    Заметки: %s\n", truncateAIText(exercise.Notes, 180)))
			}
			if exercise.RestSeconds != nil {
				sb.WriteString(fmt.Sprintf("    Отдых между подходами: %d сек\n", *exercise.RestSeconds))
			}
			for _, set := range exercise.Sets {
				parts := aiWorkoutSetParts(set.WeightKg, set.Reps, set.DistanceMeters, set.DurationSeconds, nil)
				sb.WriteString(fmt.Sprintf("    Плановый подход %d: %s", set.SetIndex, strings.Join(parts, ", ")))
				if set.SetType != "" && set.SetType != "normal" {
					sb.WriteString(fmt.Sprintf(" [%s]", set.SetType))
				}
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	}

	return strings.TrimSpace(sb.String())
}

func aiWorkoutSetParts(weightKg *float64, reps *int, distanceMeters *float64, durationSeconds *int, rpe *float64) []string {
	parts := make([]string, 0, 4)
	if weightKg != nil || reps != nil {
		weight := "-"
		repsValue := "-"
		if weightKg != nil {
			weight = formatAIFloat(*weightKg)
		}
		if reps != nil {
			repsValue = fmt.Sprintf("%d", *reps)
		}
		parts = append(parts, fmt.Sprintf("%s кг x %s", weight, repsValue))
	}
	if distanceMeters != nil {
		parts = append(parts, fmt.Sprintf("%s м", formatAIFloat(*distanceMeters)))
	}
	if durationSeconds != nil {
		parts = append(parts, fmt.Sprintf("%d сек", *durationSeconds))
	}
	if rpe != nil {
		parts = append(parts, fmt.Sprintf("RPE %s", formatAIFloat(*rpe)))
	}
	if len(parts) == 0 {
		parts = append(parts, "без числовых метрик")
	}
	return parts
}
