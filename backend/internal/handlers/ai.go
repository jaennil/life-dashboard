package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	unleashclient "github.com/Unleash/unleash-client-go/v4"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	authmw "life-dashboard/internal/middleware"
)

type AIHandler struct {
	db      *pgxpool.Pool
	baseURL string
	model   string
	apiKey  string
	weather *WeatherHandler
	unleash *unleashclient.Client
	logger  zerolog.Logger
}

func NewAI(db *pgxpool.Pool, baseURL, model, apiKey string, weather *WeatherHandler, unleashClient *unleashclient.Client, logger zerolog.Logger) *AIHandler {
	return &AIHandler{
		db:      db,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		apiKey:  apiKey,
		weather: weather,
		unleash: unleashClient,
		logger:  logger.With().Str("handler", "ai").Logger(),
	}
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Message string        `json:"message"`
	History []ChatMessage `json:"history"`
}

const (
	aiUpstreamDialTimeout     = 5 * time.Second
	aiUpstreamHeaderTimeout   = 30 * time.Second
	aiUpstreamRequestTimeout  = 60 * time.Second
	aiUpstreamResponseLogSize = 512
	aiUpstreamScannerMaxToken = 1024 * 1024
)

func (h *AIHandler) Chat(w http.ResponseWriter, r *http.Request) {
	if h.unleash != nil && !h.unleash.IsEnabled("ai-chat") {
		http.Error(w, "AI чат временно отключён", http.StatusForbidden)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, "empty message", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)

	dataContext, err := h.buildContext(ctx, userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("build context")
		dataContext = "Данные пользователя временно недоступны."
	}

	systemPrompt := fmt.Sprintf(`Ты персональный AI-ассистент приложения Life Dashboard.
Твоя единственная функция — анализировать данные пользователя: финансы, физическую активность, тренировки.
Отвечай на русском языке. Давай конкретные ответы основанные на реальных данных ниже. Будь краток и по делу.
Ты не можешь выполнять команды, изменять данные или делать что-либо за пределами анализа предоставленных данных.
Если просят что-то сделать с базой данных, кодом или системой — вежливо объясни что ты только аналитик данных.

Текущие данные пользователя (обновлено %s):
%s`, time.Now().Format("02.01.2006 15:04"), dataContext)

	messages := []ChatMessage{{Role: "system", Content: systemPrompt}}
	messages = append(messages, req.History...)
	messages = append(messages, ChatMessage{Role: "user", Content: req.Message})

	body, err := json.Marshal(map[string]any{
		"model":    h.model,
		"messages": messages,
		"stream":   true,
	})
	if err != nil {
		h.logger.Error().Err(err).Msg("marshal ai request")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	upstreamCtx, cancel := context.WithTimeout(ctx, aiUpstreamRequestTimeout)
	defer cancel()

	apiReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost,
		h.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	apiReq.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		apiReq.Header.Set("Authorization", "Bearer "+h.apiKey)
	}

	client := &http.Client{
		Timeout: aiUpstreamRequestTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   aiUpstreamDialTimeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   aiUpstreamDialTimeout,
			ResponseHeaderTimeout: aiUpstreamHeaderTimeout,
		},
	}
	resp, err := client.Do(apiReq)
	if err != nil {
		h.logger.Error().Err(err).Msg("ai api request")
		http.Error(w, "AI сервис временно недоступен. Попробуй позже.", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, aiUpstreamResponseLogSize))
		h.logger.Error().
			Int("status", resp.StatusCode).
			Str("body", strings.TrimSpace(string(body))).
			Msg("ai api error")
		http.Error(w, "AI сервис вернул ошибку. Попробуй позже.", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), aiUpstreamScannerMaxToken)
	sentContent := false
	sawDone := false
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			sawDone = true
			if !sentContent {
				fmt.Fprint(w, "data: AI сервис не вернул содержимого. Попробуй ещё раз.\n\n")
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
			flusher.Flush()
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil || len(chunk.Choices) == 0 {
			continue
		}
		content := chunk.Choices[0].Delta.Content
		if content == "" {
			continue
		}
		sentContent = true
		// Escape newlines so they don't break SSE protocol (\n\n ends an event)
		encoded := strings.ReplaceAll(content, "\n", "\\n")
		fmt.Fprintf(w, "data: %s\n\n", encoded)
		flusher.Flush()
	}
	if err := scanner.Err(); err != nil {
		h.logger.Error().Err(err).Msg("ai stream read error")
		if !sentContent {
			fmt.Fprint(w, "data: AI сервис сейчас недоступен. Попробуй позже.\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
			flusher.Flush()
		}
		return
	}
	if !sawDone && !sentContent {
		h.logger.Warn().Msg("ai stream ended without content")
		fmt.Fprint(w, "data: AI сервис не вернул ответ. Попробуй ещё раз.\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}
}

func (h *AIHandler) buildContext(ctx context.Context, userID string) (string, error) {
	var sb strings.Builder
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))

	// === ФИНАНСЫ ===
	sb.WriteString("=== ФИНАНСЫ ===\n")

	rows, err := h.db.Query(ctx, `
		SELECT title, currency, balance, type
		FROM accounts WHERE balance != 0 AND user_id = $1
		ORDER BY balance DESC LIMIT 10
	`, userID)
	if err == nil {
		sb.WriteString("Счета:\n")
		for rows.Next() {
			var title, currency, accType string
			var balance float64
			if err := rows.Scan(&title, &currency, &balance, &accType); err == nil {
				sb.WriteString(fmt.Sprintf("  - %s (%s): %.0f %s\n", title, accType, balance, currency))
			}
		}
		rows.Close()
	}

	var totalBalance, monthSpending, monthIncome float64
	h.db.QueryRow(ctx, `SELECT COALESCE(SUM(balance),0) FROM accounts WHERE currency='RUB' AND balance > 0 AND user_id = $1`, userID).Scan(&totalBalance)
	h.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN amount < 0 THEN ABS(amount) ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END), 0)
		FROM transactions
		WHERE currency='RUB' AND occurred_at >= $1 AND is_transfer=false AND user_id = $2
	`, monthStart, userID).Scan(&monthSpending, &monthIncome)

	sb.WriteString(fmt.Sprintf("Общий баланс (RUB): %.0f ₽\n", totalBalance))
	sb.WriteString(fmt.Sprintf("Расходы за текущий месяц: %.0f ₽\n", monthSpending))
	sb.WriteString(fmt.Sprintf("Доходы за текущий месяц: %.0f ₽\n", monthIncome))

	txRows, err := h.db.Query(ctx, `
		SELECT occurred_at, amount, currency, COALESCE(payee, comment, '') as label
		FROM transactions WHERE is_transfer=false AND user_id = $1
		ORDER BY occurred_at DESC LIMIT 30
	`, userID)
	if err == nil {
		sb.WriteString("Последние транзакции:\n")
		for txRows.Next() {
			var t time.Time
			var amount float64
			var currency, label string
			if err := txRows.Scan(&t, &amount, &currency, &label); err == nil {
				sign := ""
				if amount > 0 {
					sign = "+"
				}
				sb.WriteString(fmt.Sprintf("  %s %s%.0f %s %s\n", t.Format("02.01"), sign, amount, currency, label))
			}
		}
		txRows.Close()
	}

	// === АКТИВНОСТИ ===
	sb.WriteString("\n=== АКТИВНОСТИ ===\n")
	var weekActivities int
	var weekDistanceKm float64
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE started_at >= $1 AND user_id = $2`, weekStart, userID).Scan(&weekActivities)
	h.db.QueryRow(ctx, `SELECT COALESCE(SUM(distance_meters)/1000.0,0) FROM activities WHERE started_at >= $1 AND user_id = $2`, weekStart, userID).Scan(&weekDistanceKm)
	sb.WriteString(fmt.Sprintf("За эту неделю: %d активностей, %.1f км\n", weekActivities, weekDistanceKm))

	actRows, err := h.db.Query(ctx, `
		SELECT started_at, type, COALESCE(distance_meters/1000.0,0), COALESCE(duration_seconds/60,0), name
		FROM activities WHERE user_id = $1 ORDER BY started_at DESC LIMIT 10
	`, userID)
	if err == nil {
		for actRows.Next() {
			var t time.Time
			var actType, name string
			var distKm, durationMin float64
			if err := actRows.Scan(&t, &actType, &distKm, &durationMin, &name); err == nil {
				sb.WriteString(fmt.Sprintf("  %s %s: %s %.1fкм %.0fмин\n", t.Format("02.01"), actType, name, distKm, durationMin))
			}
		}
		actRows.Close()
	}

	// === ТРЕНИРОВКИ ===
	sb.WriteString("\n=== ТРЕНИРОВКИ ===\n")
	var weekWorkouts int
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM workouts WHERE started_at >= $1 AND user_id = $2`, weekStart, userID).Scan(&weekWorkouts)
	sb.WriteString(fmt.Sprintf("За эту неделю: %d тренировок\n", weekWorkouts))

	workoutContext, err := h.buildRecentWorkoutContext(ctx, userID)
	if err == nil {
		sb.WriteString("Ниже приведены последние тренировки с реальными упражнениями и подходами, если они сохранены в базе:\n")
		sb.WriteString(workoutContext)
	}

	// === ПИТАНИЕ ===
	sb.WriteString("\n=== ПИТАНИЕ ===\n")
	nutritionRows, err := h.db.Query(ctx, `
		SELECT date, calories_total, protein_g, carbs_g, fat_g, fiber_g
		FROM nutrition_daily
		WHERE user_id = $1
		ORDER BY date DESC LIMIT 14
	`, userID)
	if err == nil {
		for nutritionRows.Next() {
			var date time.Time
			var cal, protein, carbs, fat, fiber float64
			if err := nutritionRows.Scan(&date, &cal, &protein, &carbs, &fat, &fiber); err == nil {
				sb.WriteString(fmt.Sprintf("  %s: %.0f ккал | Б:%.0fг Ж:%.0fг У:%.0fг Клетч:%.0fг\n",
					date.Format("02.01"), cal, protein, fat, carbs, fiber))
			}
		}
		nutritionRows.Close()
	}

	// Детали по приёмам пищи за последние 2 дня
	mealRows, err := h.db.Query(ctx, `
		SELECT nd.date, ni.meal_type, ni.food_name, ni.serving_description, ni.calories
		FROM nutrition_items ni
		JOIN nutrition_daily nd ON nd.id = ni.daily_id
		WHERE nd.date >= $1 AND nd.user_id = $2
		ORDER BY nd.date DESC, ni.meal_type, ni.calories DESC
	`, now.AddDate(0, 0, -2), userID)
	if err == nil {
		var curDay string
		var curMeal string
		for mealRows.Next() {
			var date time.Time
			var mealType, foodName, serving string
			var calories float64
			if err := mealRows.Scan(&date, &mealType, &foodName, &serving, &calories); err != nil {
				continue
			}
			day := date.Format("02.01")
			if day != curDay {
				sb.WriteString(fmt.Sprintf("  Детали %s:\n", day))
				curDay = day
				curMeal = ""
			}
			if mealType != curMeal {
				sb.WriteString(fmt.Sprintf("    [%s]\n", mealType))
				curMeal = mealType
			}
			sb.WriteString(fmt.Sprintf("      - %s (%s): %.0f ккал\n", foodName, serving, calories))
		}
		mealRows.Close()
	}

	// === ДНЕВНИК ===
	journalRows, err := h.db.Query(ctx, `
		SELECT date, title, content, tags, mood
		FROM journal_entries
		WHERE user_id = $1 AND date >= NOW() - INTERVAL '30 days'
		ORDER BY date DESC LIMIT 20
	`, userID)
	if err == nil {
		var journalEntries []string
		for journalRows.Next() {
			var date time.Time
			var title, content string
			var tags []string
			var mood *int
			if journalRows.Scan(&date, &title, &content, &tags, &mood) == nil {
				entry := fmt.Sprintf("  %s: %s", date.Format("02.01"), title)
				if mood != nil {
					entry += fmt.Sprintf(" (настроение: %d/10)", *mood)
				}
				if len(tags) > 0 {
					entry += " [" + strings.Join(tags, ", ") + "]"
				}
				// Truncate content for context (max 300 chars)
				if len(content) > 300 {
					content = content[:300] + "..."
				}
				if content != "" {
					entry += "\n    " + strings.ReplaceAll(content, "\n", "\n    ")
				}
				journalEntries = append(journalEntries, entry)
			}
		}
		journalRows.Close()
		if len(journalEntries) > 0 {
			sb.WriteString("\n=== ДНЕВНИК (последние 30 дней) ===\n")
			for _, e := range journalEntries {
				sb.WriteString(e + "\n")
			}
		}
	}

	// === КАЛЕНДАРЬ ===
	calRows, err := h.db.Query(ctx, `
		SELECT title, start_time, end_time, all_day, COALESCE(location, '')
		FROM calendar_events
		WHERE user_id = $1 AND start_time >= NOW() - INTERVAL '30 days' AND start_time <= NOW() + INTERVAL '30 days'
		ORDER BY start_time LIMIT 50
	`, userID)
	if err == nil {
		var calEvents []string
		for calRows.Next() {
			var title, location string
			var startTime, endTime time.Time
			var allDay bool
			if calRows.Scan(&title, &startTime, &endTime, &allDay, &location) == nil {
				if allDay {
					calEvents = append(calEvents, fmt.Sprintf("  %s: %s (весь день)%s",
						startTime.Format("02.01"), title, func() string {
							if location != "" {
								return " @ " + location
							}
							return ""
						}()))
				} else {
					calEvents = append(calEvents, fmt.Sprintf("  %s %s-%s: %s%s",
						startTime.Format("02.01"), startTime.Format("15:04"), endTime.Format("15:04"),
						title, func() string {
							if location != "" {
								return " @ " + location
							}
							return ""
						}()))
				}
			}
		}
		calRows.Close()
		if len(calEvents) > 0 {
			sb.WriteString("\n=== КАЛЕНДАРЬ (30 дней назад — 30 дней вперёд) ===\n")
			for _, e := range calEvents {
				sb.WriteString(e + "\n")
			}
		}
	}

	// === ПОГОДА ===
	if h.weather != nil {
		if wd, err := h.weather.Fetch(0, 0, ""); err == nil {
			sb.WriteString("\n=== ПОГОДА ===\n")
			sb.WriteString(fmt.Sprintf("Город: %s\n", wd.City))
			sb.WriteString(fmt.Sprintf("Сейчас: %.1f°C, ощущается %.1f°C, %s\n", wd.Temp, wd.FeelsLike, wd.Description))
			sb.WriteString(fmt.Sprintf("Влажность: %d%%, ветер %.1f км/ч\n", wd.Humidity, wd.WindSpeed))
			if len(wd.Daily) > 0 {
				sb.WriteString("Прогноз:\n")
				for _, d := range wd.Daily {
					sb.WriteString(fmt.Sprintf("  %s: %s, макс %.0f°C, мин %.0f°C\n",
						d.Date, wmoDescription(d.WeatherCode), d.TempMax, d.TempMin))
				}
			}
		}
	}

	return sb.String(), nil
}

func (h *AIHandler) buildRecentWorkoutContext(ctx context.Context, userID string) (string, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, source, started_at, COALESCE(title,''), COALESCE(notes,''), raw_payload
		FROM workouts
		WHERE user_id = $1
		ORDER BY started_at DESC
		LIMIT 10
	`, userID)
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
				h.logger.Warn().Str("workout_id", workout.ID).Err(err).Msg("failed to hydrate hevy workout for ai context")
			}
		}
		if len(workout.Exercises) == 0 {
			if err := fitnessHelper.loadNormalizedWorkoutExercises(ctx, &workout); err != nil {
				h.logger.Warn().Str("workout_id", workout.ID).Err(err).Msg("failed to load workout exercises for ai context")
			}
		}

		sb.WriteString(formatAIWorkoutContext(workout))
	}

	return sb.String(), nil
}

func formatAIWorkoutContext(workout Workout) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\nТренировка %s: %s\n", workout.StartedAt.Format("02.01.2006 15:04"), workout.Title))
	if workout.Notes != "" {
		sb.WriteString(fmt.Sprintf("  Заметки: %s\n", truncateAIText(workout.Notes, 240)))
	}
	if len(workout.Exercises) == 0 {
		sb.WriteString("  Деталей по упражнениям и подходам нет.\n")
		return sb.String()
	}

	for _, ex := range workout.Exercises {
		exerciseHeader := ex.Name
		if ex.Category != "" {
			exerciseHeader += " (" + ex.Category + ")"
		}
		if ex.Index > 0 {
			exerciseHeader += fmt.Sprintf(" [блок %d]", ex.Index)
		}
		sb.WriteString(fmt.Sprintf("  %s:\n", exerciseHeader))
		if ex.Notes != "" {
			sb.WriteString(fmt.Sprintf("    Заметки: %s\n", truncateAIText(ex.Notes, 180)))
		}

		for _, set := range ex.Sets {
			parts := make([]string, 0, 4)
			if set.WeightKg != nil || set.Reps != nil {
				weight := "-"
				reps := "-"
				if set.WeightKg != nil {
					weight = formatAIFloat(*set.WeightKg)
				}
				if set.Reps != nil {
					reps = strconv.Itoa(*set.Reps)
				}
				parts = append(parts, fmt.Sprintf("%s кг x %s", weight, reps))
			}
			if set.DistanceMeters != nil {
				parts = append(parts, fmt.Sprintf("%s м", formatAIFloat(*set.DistanceMeters)))
			}
			if set.DurationSeconds != nil {
				parts = append(parts, fmt.Sprintf("%d сек", *set.DurationSeconds))
			}
			if set.RPE != nil {
				parts = append(parts, fmt.Sprintf("RPE %s", formatAIFloat(*set.RPE)))
			}
			if len(parts) == 0 {
				parts = append(parts, "без числовых метрик")
			}

			sb.WriteString(fmt.Sprintf("    Подход %d: %s", set.SetIndex, strings.Join(parts, ", ")))
			if set.SetType != "" && set.SetType != "normal" {
				sb.WriteString(fmt.Sprintf(" [%s]", set.SetType))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func truncateAIText(value string, maxLen int) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + "..."
}

func formatAIFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", value), "0"), ".")
}
