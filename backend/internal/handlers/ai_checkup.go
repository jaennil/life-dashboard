package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	authmw "life-dashboard/internal/middleware"
)

type CheckupRequest struct {
	Period string `json:"period"`
}

type CheckupResponse struct {
	Content     string    `json:"content"`
	Period      string    `json:"period"`
	PeriodLabel string    `json:"period_label"`
	GeneratedAt time.Time `json:"generated_at"`
}

type checkupWindow struct {
	RequestedPeriod string
	EffectivePeriod string
	Title           string
	UserLabel       string
	Start           time.Time
	End             time.Time
	Note            string
}

const (
	checkupPeriodToday     = "today"
	checkupPeriodWeek      = "week"
	checkupPeriodMonth     = "month"
	checkupPeriodSinceLast = "since_last"
)

func (h *AIHandler) Checkup(w http.ResponseWriter, r *http.Request) {
	if h.unleash != nil && !h.unleash.IsEnabled("ai-chat") {
		http.Error(w, "AI чат временно отключён", http.StatusForbidden)
		return
	}

	var req CheckupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	userID := r.Context().Value(authmw.UserIDKey).(string)
	now := time.Now()

	lastReportAt, err := h.getLastCheckupAt(r.Context(), userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("load last ai checkup")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	window, err := resolveCheckupWindow(now, req.Period, lastReportAt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	dataContext, err := h.buildCheckupContext(r.Context(), userID, window)
	if err != nil {
		h.logger.Error().Err(err).Msg("build ai checkup context")
		dataContext = "Данные пользователя временно недоступны."
	}

	systemPrompt := buildAICheckupPrompt(now, window, dataContext)
	userPrompt := fmt.Sprintf("Сделай checkup %s.", window.UserLabel)

	content, err := h.complete(r.Context(), []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	if err != nil {
		writeAICompletionError(w, err)
		return
	}

	if err := h.storeCheckupReport(r.Context(), userID, window, content); err != nil {
		h.logger.Warn().Err(err).Msg("store ai checkup report")
	}
	h.storeChatExchange(r.Context(), userID, userPrompt, content)

	respBody, err := json.Marshal(CheckupResponse{
		Content:     content,
		Period:      window.RequestedPeriod,
		PeriodLabel: window.Title,
		GeneratedAt: now,
	})
	if err != nil {
		h.logger.Error().Err(err).Msg("marshal ai checkup response")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(respBody); err != nil {
		h.logger.Error().Err(err).Msg("write ai checkup response")
	}
}

func (h *AIHandler) getLastCheckupAt(ctx context.Context, userID string) (*time.Time, error) {
	var ts time.Time
	err := h.db.QueryRow(ctx, `
		SELECT created_at
		FROM ai_checkup_reports
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, userID).Scan(&ts)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &ts, nil
}

func resolveCheckupWindow(now time.Time, requested string, lastReportAt *time.Time) (checkupWindow, error) {
	requested = strings.TrimSpace(strings.ToLower(requested))
	if requested == "" {
		requested = checkupPeriodWeek
	}

	switch requested {
	case checkupPeriodToday:
		return checkupWindow{
			RequestedPeriod: requested,
			EffectivePeriod: requested,
			Title:           "Checkup за сегодня",
			UserLabel:       "за сегодня",
			Start:           startOfDay(now),
			End:             now,
		}, nil
	case checkupPeriodWeek:
		return checkupWindow{
			RequestedPeriod: requested,
			EffectivePeriod: requested,
			Title:           "Checkup за неделю",
			UserLabel:       "за последние 7 дней",
			Start:           startOfDay(now.AddDate(0, 0, -6)),
			End:             now,
		}, nil
	case checkupPeriodMonth:
		return checkupWindow{
			RequestedPeriod: requested,
			EffectivePeriod: requested,
			Title:           "Checkup за месяц",
			UserLabel:       "за последние 30 дней",
			Start:           startOfDay(now.AddDate(0, 0, -29)),
			End:             now,
		}, nil
	case checkupPeriodSinceLast:
		if lastReportAt == nil {
			return checkupWindow{
				RequestedPeriod: requested,
				EffectivePeriod: checkupPeriodWeek,
				Title:           "Checkup с момента последнего отчёта",
				UserLabel:       "с момента последнего отчёта",
				Start:           startOfDay(now.AddDate(0, 0, -6)),
				End:             now,
				Note:            "Предыдущих checkup-отчётов не найдено, поэтому для первого отчёта взят период за последние 7 дней.",
			}, nil
		}

		start := *lastReportAt
		if start.After(now) {
			start = now
		}

		return checkupWindow{
			RequestedPeriod: requested,
			EffectivePeriod: requested,
			Title:           "Checkup с момента последнего отчёта",
			UserLabel:       "с момента последнего отчёта",
			Start:           start,
			End:             now,
		}, nil
	default:
		return checkupWindow{}, fmt.Errorf("unknown period")
	}
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func buildAICheckupPrompt(now time.Time, window checkupWindow, dataContext string) string {
	var sb strings.Builder
	sb.WriteString("Ты персональный AI-ассистент приложения Life Dashboard.\n")
	sb.WriteString("Твоя задача — сделать checkup-отчёт по всем доступным сферам жизни пользователя за указанный период.\n")
	sb.WriteString("Отвечай только на русском языке. Не выдумывай факты и не додумывай цифры.\n")
	sb.WriteString("Сделай структурированный ответ:\n")
	sb.WriteString("1. Короткий итог в 2-4 предложениях.\n")
	sb.WriteString("2. Финансы.\n")
	sb.WriteString("3. Активность и тренировки.\n")
	sb.WriteString("4. Питание и здоровье.\n")
	sb.WriteString("5. Личное / заметки / календарь, если данные есть.\n")
	sb.WriteString("6. Что хорошо.\n")
	sb.WriteString("7. Что требует внимания.\n")
	sb.WriteString("8. Три конкретных шага на следующий период.\n")
	sb.WriteString("Если по какой-то сфере данных нет, напиши это коротко и без воды.\n")
	sb.WriteString("Если видна динамика веса, шагов, расходов, тренировок или питания — покажи её числами.\n")
	sb.WriteString("Не делай длинное эссе: нужен практичный checkup.\n\n")
	sb.WriteString(fmt.Sprintf("Сейчас %s.\n", now.Format("02.01.2006 15:04")))
	sb.WriteString(fmt.Sprintf("Период отчёта: %s (%s — %s).\n",
		window.Title,
		window.Start.Format("02.01.2006 15:04"),
		window.End.Format("02.01.2006 15:04"),
	))
	if window.Note != "" {
		sb.WriteString(window.Note + "\n")
	}
	sb.WriteString("\nДанные пользователя:\n")
	sb.WriteString(dataContext)
	return sb.String()
}

func (h *AIHandler) buildCheckupContext(ctx context.Context, userID string, window checkupWindow) (string, error) {
	var sb strings.Builder
	sb.WriteString("=== ПЕРИОД ===\n")
	sb.WriteString(fmt.Sprintf("Начало: %s\n", window.Start.Format("02.01.2006 15:04")))
	sb.WriteString(fmt.Sprintf("Конец: %s\n", window.End.Format("02.01.2006 15:04")))
	if window.Note != "" {
		sb.WriteString("Примечание: " + window.Note + "\n")
	}

	h.appendCheckupFinanceContext(ctx, &sb, userID, window)
	h.appendCheckupHealthContext(ctx, &sb, userID, window)
	h.appendCheckupActivityContext(ctx, &sb, userID, window)
	if err := h.appendCheckupWorkoutContext(ctx, &sb, userID, window); err != nil {
		return "", err
	}
	h.appendCheckupNutritionContext(ctx, &sb, userID, window)
	h.appendCheckupJournalContext(ctx, &sb, userID, window)
	h.appendCheckupCalendarContext(ctx, &sb, userID, window)

	return sb.String(), nil
}

func (h *AIHandler) appendCheckupFinanceContext(ctx context.Context, sb *strings.Builder, userID string, window checkupWindow) {
	sb.WriteString("\n=== ФИНАНСЫ ===\n")

	var currentBalance, spending, income float64
	var txCount int
	h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(balance), 0)
		FROM accounts
		WHERE currency = 'RUB' AND in_balance = TRUE AND user_id = $1
	`, userID).Scan(&currentBalance)
	h.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN t.amount < 0 THEN ABS(t.amount) ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN t.amount > 0 THEN t.amount ELSE 0 END), 0),
			COUNT(*)
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.user_id = $1
			AND t.currency = 'RUB'
			AND t.is_transfer = FALSE
			AND COALESCE(a.in_balance, TRUE) = TRUE
			AND t.occurred_at >= $2
			AND t.occurred_at < $3
	`, userID, window.Start, window.End).Scan(&spending, &income, &txCount)
	sb.WriteString(fmt.Sprintf("Текущий баланс: %.0f ₽\n", currentBalance))
	sb.WriteString(fmt.Sprintf("За период: %d транзакций, расходы %.0f ₽, доходы %.0f ₽, net %.0f ₽\n", txCount, spending, income, income-spending))

	rows, err := h.db.Query(ctx, `
		SELECT COALESCE(NULLIF(t.category, ''), 'Без категории'), COALESCE(SUM(ABS(t.amount)), 0)
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.user_id = $1
			AND t.amount < 0
			AND t.currency = 'RUB'
			AND t.is_transfer = FALSE
			AND COALESCE(a.in_balance, TRUE) = TRUE
			AND t.occurred_at >= $2
			AND t.occurred_at < $3
		GROUP BY 1
		ORDER BY 2 DESC
		LIMIT 5
	`, userID, window.Start, window.End)
	if err == nil {
		defer rows.Close()
		sb.WriteString("Топ категорий:\n")
		hasRows := false
		for rows.Next() {
			hasRows = true
			var category string
			var amount float64
			if rows.Scan(&category, &amount) == nil {
				sb.WriteString(fmt.Sprintf("  - %s: %.0f ₽\n", category, amount))
			}
		}
		if !hasRows {
			sb.WriteString("  - Нет расходных категорий за период\n")
		}
	}

	payeeRows, err := h.db.Query(ctx, `
		SELECT COALESCE(NULLIF(t.payee, ''), NULLIF(t.comment, ''), 'Без названия'), COALESCE(SUM(ABS(t.amount)), 0), COUNT(*)
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.user_id = $1
			AND t.amount < 0
			AND t.currency = 'RUB'
			AND t.is_transfer = FALSE
			AND COALESCE(a.in_balance, TRUE) = TRUE
			AND t.occurred_at >= $2
			AND t.occurred_at < $3
		GROUP BY 1
		ORDER BY 2 DESC
		LIMIT 5
	`, userID, window.Start, window.End)
	if err == nil {
		defer payeeRows.Close()
		sb.WriteString("Крупные получатели денег:\n")
		hasRows := false
		for payeeRows.Next() {
			hasRows = true
			var label string
			var amount float64
			var count int
			if payeeRows.Scan(&label, &amount, &count) == nil {
				sb.WriteString(fmt.Sprintf("  - %s: %.0f ₽ (%d)\n", label, amount, count))
			}
		}
		if !hasRows {
			sb.WriteString("  - Нет заметных расходных получателей за период\n")
		}
	}
}

func (h *AIHandler) appendCheckupHealthContext(ctx context.Context, sb *strings.Builder, userID string, window checkupWindow) {
	sb.WriteString("\n")
	h.appendHealthContextInRange(ctx, sb, userID, window.Start, window.End, "=== ЗДОРОВЬЕ ===")
}

func (h *AIHandler) appendCheckupActivityContext(ctx context.Context, sb *strings.Builder, userID string, window checkupWindow) {
	sb.WriteString("\n=== АКТИВНОСТЬ ===\n")

	var count int
	var distanceKm, durationHours, calories float64
	h.db.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(distance_meters) / 1000.0, 0),
			COALESCE(SUM(duration_seconds) / 3600.0, 0),
			COALESCE(SUM(calories), 0)
		FROM activities
		WHERE user_id = $1
			AND started_at >= $2
			AND started_at < $3
	`, userID, window.Start, window.End).Scan(&count, &distanceKm, &durationHours, &calories)
	sb.WriteString(fmt.Sprintf("За период: %d активностей, %.1f км, %.1f ч, %.0f ккал\n", count, distanceKm, durationHours, calories))

	rows, err := h.db.Query(ctx, `
		SELECT COALESCE(NULLIF(sport_type, ''), type), COUNT(*), COALESCE(SUM(distance_meters) / 1000.0, 0)
		FROM activities
		WHERE user_id = $1
			AND started_at >= $2
			AND started_at < $3
		GROUP BY 1
		ORDER BY 2 DESC, 3 DESC
		LIMIT 5
	`, userID, window.Start, window.End)
	if err == nil {
		defer rows.Close()
		sb.WriteString("Типы активностей:\n")
		hasRows := false
		for rows.Next() {
			hasRows = true
			var activityType string
			var activityCount int
			var km float64
			if rows.Scan(&activityType, &activityCount, &km) == nil {
				sb.WriteString(fmt.Sprintf("  - %s: %d шт, %.1f км\n", activityType, activityCount, km))
			}
		}
		if !hasRows {
			sb.WriteString("  - Нет активностей за период\n")
		}
	}

	recentRows, err := h.db.Query(ctx, `
		SELECT started_at, COALESCE(NULLIF(sport_type, ''), type), COALESCE(name, ''), COALESCE(distance_meters / 1000.0, 0), COALESCE(duration_seconds / 60.0, 0)
		FROM activities
		WHERE user_id = $1
			AND started_at >= $2
			AND started_at < $3
		ORDER BY started_at DESC
		LIMIT 5
	`, userID, window.Start, window.End)
	if err == nil {
		defer recentRows.Close()
		sb.WriteString("Последние активности периода:\n")
		hasRows := false
		for recentRows.Next() {
			hasRows = true
			var startedAt time.Time
			var activityType, name string
			var km, minutes float64
			if recentRows.Scan(&startedAt, &activityType, &name, &km, &minutes) == nil {
				sb.WriteString(fmt.Sprintf("  - %s %s: %s, %.1f км, %.0f мин\n", startedAt.Format("02.01"), activityType, name, km, minutes))
			}
		}
		if !hasRows {
			sb.WriteString("  - Нет активностей за период\n")
		}
	}
}

func (h *AIHandler) appendCheckupWorkoutContext(ctx context.Context, sb *strings.Builder, userID string, window checkupWindow) error {
	sb.WriteString("\n=== ТРЕНИРОВКИ ===\n")

	var workoutCount, activeDays, totalSets, uniqueExercises int
	h.db.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(DISTINCT DATE(started_at))
		FROM workouts
		WHERE user_id = $1
			AND started_at >= $2
			AND started_at < $3
	`, userID, window.Start, window.End).Scan(&workoutCount, &activeDays)
	h.db.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(DISTINCT LOWER(TRIM(exercise_name)))
		FROM workout_sets ws
		JOIN workouts w ON w.id = ws.workout_id
		WHERE w.user_id = $1
			AND w.started_at >= $2
			AND w.started_at < $3
	`, userID, window.Start, window.End).Scan(&totalSets, &uniqueExercises)
	sb.WriteString(fmt.Sprintf("За период: %d тренировок, %d тренировочных дней, %d подходов, %d уникальных упражнений\n",
		workoutCount, activeDays, totalSets, uniqueExercises))

	topRows, err := h.db.Query(ctx, `
		SELECT exercise_name, COUNT(*) AS sets_count
		FROM workout_sets ws
		JOIN workouts w ON w.id = ws.workout_id
		WHERE w.user_id = $1
			AND w.started_at >= $2
			AND w.started_at < $3
		GROUP BY exercise_name
		ORDER BY sets_count DESC, exercise_name ASC
		LIMIT 5
	`, userID, window.Start, window.End)
	if err == nil {
		defer topRows.Close()
		sb.WriteString("Частые упражнения:\n")
		hasRows := false
		for topRows.Next() {
			hasRows = true
			var name string
			var setsCount int
			if topRows.Scan(&name, &setsCount) == nil {
				sb.WriteString(fmt.Sprintf("  - %s: %d подходов\n", name, setsCount))
			}
		}
		if !hasRows {
			sb.WriteString("  - Нет данных по упражнениям за период\n")
		}
	}

	workoutContext, err := h.buildWorkoutContextInRange(ctx, userID, window.Start, window.End, 4)
	if err != nil {
		return err
	}
	if strings.TrimSpace(workoutContext) == "" {
		sb.WriteString("Последние тренировки периода: нет данных\n")
		return nil
	}

	sb.WriteString("Последние тренировки периода:\n")
	sb.WriteString(workoutContext)
	return nil
}

func (h *AIHandler) appendCheckupNutritionContext(ctx context.Context, sb *strings.Builder, userID string, window checkupWindow) {
	sb.WriteString("\n=== ПИТАНИЕ ===\n")

	var trackedDays int
	var avgCalories, avgProtein, avgCarbs, avgFat, minCalories, maxCalories float64
	h.db.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COALESCE(AVG(calories_total), 0),
			COALESCE(AVG(protein_g), 0),
			COALESCE(AVG(carbs_g), 0),
			COALESCE(AVG(fat_g), 0),
			COALESCE(MIN(calories_total), 0),
			COALESCE(MAX(calories_total), 0)
		FROM nutrition_daily
		WHERE user_id = $1
			AND date >= $2::date
			AND date <= $3::date
	`, userID, window.Start, window.End).Scan(&trackedDays, &avgCalories, &avgProtein, &avgCarbs, &avgFat, &minCalories, &maxCalories)
	if trackedDays == 0 {
		sb.WriteString("Нет записей питания за период\n")
		return
	}

	sb.WriteString(fmt.Sprintf("Отслежено дней: %d, среднее %.0f ккал | Б %.0f г | Ж %.0f г | У %.0f г\n",
		trackedDays, avgCalories, avgProtein, avgFat, avgCarbs))
	sb.WriteString(fmt.Sprintf("Диапазон калорийности: %.0f — %.0f ккал\n", minCalories, maxCalories))

	rows, err := h.db.Query(ctx, `
		SELECT TO_CHAR(date, 'DD.MM'), calories_total, protein_g, carbs_g, fat_g
		FROM nutrition_daily
		WHERE user_id = $1
			AND date >= $2::date
			AND date <= $3::date
		ORDER BY date DESC
		LIMIT 7
	`, userID, window.Start, window.End)
	if err == nil {
		defer rows.Close()
		sb.WriteString("Последние дни питания:\n")
		for rows.Next() {
			var day string
			var calories, protein, carbs, fat float64
			if rows.Scan(&day, &calories, &protein, &carbs, &fat) == nil {
				sb.WriteString(fmt.Sprintf("  - %s: %.0f ккал | Б %.0f | Ж %.0f | У %.0f\n", day, calories, protein, fat, carbs))
			}
		}
	}
}

func (h *AIHandler) appendCheckupJournalContext(ctx context.Context, sb *strings.Builder, userID string, window checkupWindow) {
	sb.WriteString("\n=== ДНЕВНИК ===\n")

	var entriesCount int
	var avgMood float64
	h.db.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(AVG(mood), 0)
		FROM journal_entries
		WHERE user_id = $1
			AND COALESCE(date, created_at::date) >= $2::date
			AND COALESCE(date, created_at::date) <= $3::date
	`, userID, window.Start, window.End).Scan(&entriesCount, &avgMood)
	if entriesCount == 0 {
		sb.WriteString("Нет записей дневника за период\n")
		return
	}

	sb.WriteString(fmt.Sprintf("Записей: %d, среднее настроение %.1f/10\n", entriesCount, avgMood))

	rows, err := h.db.Query(ctx, `
		SELECT TO_CHAR(COALESCE(date, created_at::date), 'DD.MM'), COALESCE(title, ''), COALESCE(tags, '{}'), mood
		FROM journal_entries
		WHERE user_id = $1
			AND COALESCE(date, created_at::date) >= $2::date
			AND COALESCE(date, created_at::date) <= $3::date
		ORDER BY COALESCE(date, created_at::date) DESC
		LIMIT 5
	`, userID, window.Start, window.End)
	if err == nil {
		defer rows.Close()
		sb.WriteString("Последние записи:\n")
		for rows.Next() {
			var day, title string
			var tags []string
			var mood *int
			if rows.Scan(&day, &title, &tags, &mood) == nil {
				line := fmt.Sprintf("  - %s: %s", day, title)
				if mood != nil {
					line += fmt.Sprintf(" | настроение %d/10", *mood)
				}
				if len(tags) > 0 {
					line += " | " + strings.Join(tags, ", ")
				}
				sb.WriteString(line + "\n")
			}
		}
	}
}

func (h *AIHandler) appendCheckupCalendarContext(ctx context.Context, sb *strings.Builder, userID string, window checkupWindow) {
	sb.WriteString("\n=== КАЛЕНДАРЬ ===\n")

	var eventsCount int
	h.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM calendar_events
		WHERE user_id = $1
			AND start_time >= $2
			AND start_time < $3
	`, userID, window.Start, window.End).Scan(&eventsCount)
	if eventsCount == 0 {
		sb.WriteString("Нет календарных событий за период\n")
		return
	}

	sb.WriteString(fmt.Sprintf("Событий за период: %d\n", eventsCount))

	rows, err := h.db.Query(ctx, `
		SELECT TO_CHAR(start_time, 'DD.MM HH24:MI'), title, COALESCE(location, '')
		FROM calendar_events
		WHERE user_id = $1
			AND start_time >= $2
			AND start_time < $3
		ORDER BY start_time DESC
		LIMIT 5
	`, userID, window.Start, window.End)
	if err == nil {
		defer rows.Close()
		sb.WriteString("Последние события:\n")
		for rows.Next() {
			var at, title, location string
			if rows.Scan(&at, &title, &location) == nil {
				if location != "" {
					sb.WriteString(fmt.Sprintf("  - %s: %s @ %s\n", at, title, location))
				} else {
					sb.WriteString(fmt.Sprintf("  - %s: %s\n", at, title))
				}
			}
		}
	}
}

func (h *AIHandler) buildWorkoutContextInRange(ctx context.Context, userID string, start, end time.Time, limit int) (string, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, source, started_at, COALESCE(title,''), COALESCE(notes,''), raw_payload
		FROM workouts
		WHERE user_id = $1
			AND started_at >= $2
			AND started_at < $3
		ORDER BY started_at DESC
		LIMIT $4
	`, userID, start, end, limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	fitnessHelper := &FitnessHandler{db: h.db, logger: h.logger}
	var sb strings.Builder

	for rows.Next() {
		var workout Workout
		var rawPayload []byte
		if err := rows.Scan(
			&workout.ID,
			&workout.Source,
			&workout.StartedAt,
			&workout.Title,
			&workout.Notes,
			&rawPayload,
		); err != nil {
			continue
		}

		if workout.Source == "hevy" && len(rawPayload) > 0 {
			if err := hydrateHevyWorkout(&workout, rawPayload); err != nil {
				h.logger.Warn().Str("workout_id", workout.ID).Err(err).Msg("failed to hydrate hevy workout for ai checkup")
			}
		}
		if len(workout.Exercises) == 0 {
			if err := fitnessHelper.loadNormalizedWorkoutExercises(ctx, &workout); err != nil {
				h.logger.Warn().Str("workout_id", workout.ID).Err(err).Msg("failed to load workout exercises for ai checkup")
			}
		}

		sb.WriteString(formatAIWorkoutContext(workout))
	}

	return sb.String(), nil
}

func (h *AIHandler) storeCheckupReport(ctx context.Context, userID string, window checkupWindow, content string) error {
	_, err := h.db.Exec(ctx, `
		INSERT INTO ai_checkup_reports (user_id, requested_period, period_started_at, period_ended_at, content)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, window.RequestedPeriod, window.Start, window.End, content)
	return err
}
