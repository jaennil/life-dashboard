package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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

type LatestCheckupResponse struct {
	HasReport   bool       `json:"has_report"`
	Period      string     `json:"period,omitempty"`
	PeriodLabel string     `json:"period_label,omitempty"`
	GeneratedAt *time.Time `json:"generated_at,omitempty"`
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
	checkupPeriodYesterday = "yesterday"
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

func (h *AIHandler) GetLatestCheckup(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(authmw.UserIDKey).(string)

	var requestedPeriod string
	var createdAt time.Time
	err := h.db.QueryRow(r.Context(), `
		SELECT requested_period, created_at
		FROM ai_checkup_reports
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, userID).Scan(&requestedPeriod, &createdAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, LatestCheckupResponse{HasReport: false})
			return
		}
		h.logger.Error().Err(err).Msg("load latest ai checkup")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, LatestCheckupResponse{
		HasReport:   true,
		Period:      requestedPeriod,
		PeriodLabel: checkupPeriodLabel(requestedPeriod),
		GeneratedAt: &createdAt,
	})
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
	case checkupPeriodYesterday:
		yesterdayStart := startOfDay(now.AddDate(0, 0, -1))
		return checkupWindow{
			RequestedPeriod: requested,
			EffectivePeriod: requested,
			Title:           "Checkup за вчера",
			UserLabel:       "за вчера",
			Start:           yesterdayStart,
			End:             yesterdayStart.Add(24*time.Hour - time.Nanosecond),
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

func checkupPeriodLabel(period string) string {
	switch strings.TrimSpace(strings.ToLower(period)) {
	case checkupPeriodToday:
		return "за сегодня"
	case checkupPeriodYesterday:
		return "за вчера"
	case checkupPeriodWeek:
		return "за 7 дней"
	case checkupPeriodMonth:
		return "за 30 дней"
	case checkupPeriodSinceLast:
		return "с прошлого отчёта"
	default:
		return ""
	}
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func buildAICheckupPrompt(now time.Time, window checkupWindow, dataContext string) string {
	var sb strings.Builder
	sb.WriteString("Ты персональный AI-ассистент приложения Life Dashboard.\n")
	sb.WriteString("Твоя задача — сделать checkup-отчёт по всем доступным сферам жизни пользователя за указанный период, включая продуктивность и привычки если по ним есть данные.\n")
	sb.WriteString("Отвечай только на русском языке. Не выдумывай факты и не додумывай цифры.\n")
	sb.WriteString("События Google Calendar — это только план/расписание. Они не подтверждают, что пользователь реально был в зале, лёг спать, поехал или что-то сделал.\n")
	sb.WriteString("Факт тренировки подтверждают только данные из workouts/Hevy. Факт сна, шагов, веса и пульса подтверждают только данные из biometrics/sleep_sessions.\n")
	sb.WriteString("Продуктивность и задачи подтверждаются только данными Todoist, а не календарём.\n")
	sb.WriteString("Питание отражает только залогированные записи из трекера. Не пиши \"отслежено полностью\", если в данных нет явного подтверждения полноты дня.\n")
	sb.WriteString("Ниже данные пользователя приходят как JSON-результаты внутренних tools. Сначала опирайся на поля tool/section/window/data. Если data есть, это основной источник фактов и чисел. context_text используй как summary.\n")
	sb.WriteString("Сделай структурированный ответ:\n")
	sb.WriteString("1. Короткий итог в 2-4 предложениях.\n")
	sb.WriteString("2. Финансы.\n")
	sb.WriteString("3. Продуктивность и задачи.\n")
	sb.WriteString("4. Активность и тренировки.\n")
	sb.WriteString("5. Питание и здоровье.\n")
	sb.WriteString("6. Привычки.\n")
	sb.WriteString("7. Личное / заметки / календарь, если данные есть.\n")
	sb.WriteString("8. Что хорошо.\n")
	sb.WriteString("9. Что требует внимания.\n")
	sb.WriteString("10. Три конкретных шага на следующий период.\n")
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

	run, err := h.runAITools(ctx, userID, h.checkupToolExecutions(ctx, userID, window))
	if err != nil {
		return "", err
	}
	if rendered := strings.TrimSpace(run.render()); rendered != "" {
		sb.WriteString("\n\n")
		sb.WriteString(rendered)
	}

	h.logger.Info().
		Str("user_id", userID).
		Str("period", window.RequestedPeriod).
		Strs("sections", run.Sections).
		Msg("ai checkup context built")

	return sb.String(), nil
}

func (h *AIHandler) checkupToolExecutions(ctx context.Context, userID string, window checkupWindow) []aiToolExecution {
	start := window.Start
	end := window.End
	var financeData *AIFinanceOverviewData
	loadFinanceData := func() (AIFinanceOverviewData, error) {
		if financeData != nil {
			return *financeData, nil
		}
		data, err := h.buildFinanceOverviewInRange(ctx, userID, window.Start, window.End)
		if err != nil {
			return AIFinanceOverviewData{}, err
		}
		financeData = &data
		return data, nil
	}
	var productivityData *AIProductivityOverviewData
	loadProductivityData := func() (AIProductivityOverviewData, error) {
		if productivityData != nil {
			return *productivityData, nil
		}
		data, err := h.buildProductivityOverviewInRange(ctx, userID, window.Start, window.End, 10)
		if err != nil {
			return AIProductivityOverviewData{}, err
		}
		productivityData = &data
		return data, nil
	}
	var workoutData *AIWorkoutOverviewData
	loadWorkoutData := func() (AIWorkoutOverviewData, error) {
		if workoutData != nil {
			return *workoutData, nil
		}
		data, err := h.buildWorkoutOverviewInRange(ctx, userID, window.Start, window.End)
		if err != nil {
			return AIWorkoutOverviewData{}, err
		}
		workoutData = &data
		return data, nil
	}

	return []aiToolExecution{
		{
			Name:            aiToolFinanceOverview,
			Section:         "финансы",
			RequestedPeriod: window.RequestedPeriod,
			Start:           &start,
			End:             &end,
			Data: func() (any, error) {
				data, err := loadFinanceData()
				if err != nil {
					return nil, err
				}
				return data, nil
			},
			Run: func(sb *strings.Builder) error {
				data, err := loadFinanceData()
				if err != nil {
					return err
				}
				sb.WriteString("\n")
				sb.WriteString(renderFinanceOverviewText("=== ФИНАНСЫ ===", data))
				return nil
			},
		},
		{
			Name:            aiToolProductivityOverview,
			Section:         "продуктивность",
			RequestedPeriod: window.RequestedPeriod,
			Start:           &start,
			End:             &end,
			Data: func() (any, error) {
				data, err := loadProductivityData()
				if err != nil {
					return nil, err
				}
				return data, nil
			},
			Run: func(sb *strings.Builder) error {
				data, err := loadProductivityData()
				if err != nil {
					return err
				}
				sb.WriteString("\n")
				sb.WriteString(renderProductivityOverviewText("=== ПРОДУКТИВНОСТЬ ===", data))
				return nil
			},
		},
		{
			Name:            aiToolHealthOverview,
			Section:         "здоровье",
			RequestedPeriod: window.RequestedPeriod,
			Start:           &start,
			End:             &end,
			Run: func(sb *strings.Builder) error {
				h.appendCheckupHealthContext(ctx, sb, userID, window)
				return nil
			},
		},
		{
			Name:            aiToolActivityOverview,
			Section:         "активности",
			RequestedPeriod: window.RequestedPeriod,
			Start:           &start,
			End:             &end,
			Run: func(sb *strings.Builder) error {
				h.appendCheckupActivityContext(ctx, sb, userID, window)
				return nil
			},
		},
		{
			Name:            aiToolWorkoutOverview,
			Section:         "тренировки",
			RequestedPeriod: window.RequestedPeriod,
			Start:           &start,
			End:             &end,
			Data: func() (any, error) {
				data, err := loadWorkoutData()
				if err != nil {
					return nil, err
				}
				return data, nil
			},
			Run: func(sb *strings.Builder) error {
				return h.appendCheckupWorkoutContext(ctx, sb, userID, window)
			},
		},
		{
			Name:            aiToolNutritionOverview,
			Section:         "питание",
			RequestedPeriod: window.RequestedPeriod,
			Start:           &start,
			End:             &end,
			Run: func(sb *strings.Builder) error {
				h.appendCheckupNutritionContext(ctx, sb, userID, window)
				return nil
			},
		},
		{
			Name:            aiToolHabitOverview,
			Section:         "привычки",
			RequestedPeriod: window.RequestedPeriod,
			Start:           &start,
			End:             &end,
			Run: func(sb *strings.Builder) error {
				h.appendCheckupHabitContext(ctx, sb, userID, window)
				return nil
			},
		},
		{
			Name:            aiToolJournalOverview,
			Section:         "дневник",
			RequestedPeriod: window.RequestedPeriod,
			Start:           &start,
			End:             &end,
			Run: func(sb *strings.Builder) error {
				h.appendCheckupJournalContext(ctx, sb, userID, window)
				return nil
			},
		},
		{
			Name:            aiToolCalendarOverview,
			Section:         "календарь",
			RequestedPeriod: window.RequestedPeriod,
			Start:           &start,
			End:             &end,
			Run: func(sb *strings.Builder) error {
				h.appendCheckupCalendarContext(ctx, sb, userID, window)
				return nil
			},
		},
	}
}

func (h *AIHandler) appendCheckupHealthContext(ctx context.Context, sb *strings.Builder, userID string, window checkupWindow) {
	sb.WriteString("\n")
	h.appendHealthContextInRange(ctx, sb, userID, window.Start, window.End, "=== ЗДОРОВЬЕ ===")
}

func (h *AIHandler) appendCheckupActivityContext(ctx context.Context, sb *strings.Builder, userID string, window checkupWindow) {
	sb.WriteString("\n=== АКТИВНОСТЬ ===\n")
	sb.WriteString("Фактическая активность берётся только из activities (например, Strava). Календарь сюда не относится. Шаги не отсюда, а из раздела здоровья.\n")

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
	sb.WriteString("Факт тренировки подтверждается только данными workouts/Hevy. Календарное событие \"зал\" само по себе не доказывает, что тренировка была.\n")

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
	sb.WriteString("Это только залогированные приёмы пищи из трекера. Отсутствие ужина/перекуса в логах не означает, что их не было.\n")
	if targets, err := loadNutritionTargets(ctx, h.db, userID); err == nil {
		sb.WriteString(renderNutritionTargetsForAI(targets))
	}

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

	mealRows, err := h.db.Query(ctx, `
		SELECT
			nd.date,
			COUNT(ni.id),
			COALESCE(
				array_agg(DISTINCT NULLIF(ni.meal_type, '')) FILTER (WHERE NULLIF(ni.meal_type, '') IS NOT NULL),
				'{}'::text[]
			)
		FROM nutrition_daily nd
		LEFT JOIN nutrition_items ni ON ni.daily_id = nd.id
		WHERE nd.user_id = $1
			AND nd.date >= $2::date
			AND nd.date <= $3::date
		GROUP BY nd.date
		ORDER BY nd.date DESC
		LIMIT 7
	`, userID, window.Start, window.End)
	if err == nil {
		defer mealRows.Close()
		sb.WriteString("Покрытие логов по приёмам пищи:\n")
		hasRows := false
		for mealRows.Next() {
			hasRows = true
			var day time.Time
			var itemsCount int
			var mealTypes []string
			if mealRows.Scan(&day, &itemsCount, &mealTypes) == nil {
				meals := formatAIMealTypes(mealTypes)
				if len(meals) == 0 {
					sb.WriteString(fmt.Sprintf("  - %s: %d записей, типы приёмов пищи не указаны\n", day.Format("02.01"), itemsCount))
					continue
				}
				sb.WriteString(fmt.Sprintf("  - %s: %d записей, внесено: %s\n", day.Format("02.01"), itemsCount, strings.Join(meals, ", ")))
			}
		}
		if !hasRows {
			sb.WriteString("  - Нет детализации по приёмам пищи за период\n")
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
	sb.WriteString("Это только план из Google Calendar, а не факт выполнения. Не интерпретируй эти события как реально совершившиеся действия.\n")

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
		SELECT start_time, end_time, all_day, title, COALESCE(location, '')
		FROM calendar_events
		WHERE user_id = $1
			AND start_time >= $2
			AND start_time < $3
		ORDER BY start_time DESC
		LIMIT 5
	`, userID, window.Start, window.End)
	if err == nil {
		defer rows.Close()
		sb.WriteString("Последние плановые события:\n")
		for rows.Next() {
			var startTime, endTime time.Time
			var allDay bool
			var title, location string
			if rows.Scan(&startTime, &endTime, &allDay, &title, &location) == nil {
				sb.WriteString(formatAICalendarEvent(startTime, endTime, allDay, title, location) + "\n")
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

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func formatAIMealTypes(mealTypes []string) []string {
	if len(mealTypes) == 0 {
		return nil
	}

	labels := make([]string, 0, len(mealTypes))
	seen := make(map[string]bool, len(mealTypes))
	for _, mealType := range mealTypes {
		label := aiMealTypeLabel(mealType)
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return labels
}

func aiMealTypeLabel(mealType string) string {
	normalized := strings.ToLower(strings.TrimSpace(mealType))
	switch normalized {
	case "breakfast":
		return "завтрак"
	case "lunch":
		return "обед"
	case "dinner", "supper":
		return "ужин"
	case "snack", "snacks":
		return "перекус"
	case "morning snack":
		return "утренний перекус"
	case "afternoon snack":
		return "дневной перекус"
	case "evening snack":
		return "вечерний перекус"
	case "other":
		return "другое"
	default:
		return normalized
	}
}
