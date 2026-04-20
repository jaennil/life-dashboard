package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

type aiContextScope struct {
	finance      bool
	productivity bool
	activities   bool
	health       bool
	workouts     bool
	routines     bool
	habits       bool
	nutrition    bool
	journal      bool
	calendar     bool
	weather      bool
}

const (
	aiUpstreamDialTimeout     = 5 * time.Second
	aiUpstreamHeaderTimeout   = 150 * time.Second
	aiUpstreamRequestTimeout  = 150 * time.Second
	aiUpstreamResponseLogSize = 512
)

var (
	errAIUnavailable = errors.New("ai unavailable")
	errAIUpstream    = errors.New("ai upstream error")
	errAIBadResponse = errors.New("ai bad response")
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
	history := h.buildConversationHistory(ctx, userID, req.History)

	dataContext, sectionNames, err := h.buildChatContext(ctx, userID, req.Message, history)
	if err != nil {
		h.logger.Error().Err(err).Msg("build context")
		dataContext = "Данные пользователя временно недоступны."
		sectionNames = defaultAIContextScope().sectionNames()
	}

	systemPrompt := buildAISystemPromptWithSections(time.Now(), dataContext, sectionNames)

	messages := []ChatMessage{{Role: "system", Content: systemPrompt}}
	messages = append(messages, history...)
	messages = append(messages, ChatMessage{Role: "user", Content: req.Message})

	content, err := h.complete(ctx, messages)
	if err != nil {
		writeAICompletionError(w, err)
		return
	}

	respBody, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		h.logger.Error().Err(err).Msg("marshal ai response")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.storeChatExchange(ctx, userID, req.Message, content)

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(respBody); err != nil {
		h.logger.Error().Err(err).Msg("write ai response")
	}
}

func (h *AIHandler) buildConversationHistory(ctx context.Context, userID string, clientHistory []ChatMessage) []ChatMessage {
	clientHistory = sanitizeChatHistory(clientHistory, aiHistoryContextLimit)

	storedHistory, err := h.loadRecentChatMessages(ctx, userID, aiHistoryContextLimit)
	if err != nil {
		h.logger.Warn().Err(err).Str("user_id", userID).Msg("load ai chat history from db")
		return clientHistory
	}
	if len(storedHistory) == 0 {
		return clientHistory
	}

	history := mergeChatHistory(storedHistory, clientHistory, aiHistoryContextLimit)
	h.logger.Debug().
		Str("user_id", userID).
		Int("stored_history", len(storedHistory)).
		Int("client_history", len(clientHistory)).
		Int("merged_history", len(history)).
		Msg("ai chat history prepared")
	return history
}

func sanitizeChatHistory(history []ChatMessage, limit int) []ChatMessage {
	if limit <= 0 || len(history) == 0 {
		return nil
	}
	start := len(history) - limit
	if start < 0 {
		start = 0
	}

	messages := make([]ChatMessage, 0, len(history)-start)
	for _, msg := range history[start:] {
		if normalized := normalizeChatMessage(msg); normalized != nil {
			messages = append(messages, *normalized)
		}
	}
	return messages
}

func normalizeChatMessage(msg ChatMessage) *ChatMessage {
	role := strings.TrimSpace(strings.ToLower(msg.Role))
	if role != "user" && role != "assistant" {
		return nil
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}
	return &ChatMessage{Role: role, Content: content}
}

func mergeChatHistory(storedHistory, clientHistory []ChatMessage, limit int) []ChatMessage {
	if limit <= 0 {
		return nil
	}

	merged := make([]ChatMessage, 0, len(storedHistory)+len(clientHistory))
	seen := make(map[string]bool, len(storedHistory)+len(clientHistory))
	for _, msg := range append(storedHistory, clientHistory...) {
		normalized := normalizeChatMessage(msg)
		if normalized == nil {
			continue
		}
		key := normalized.Role + "\x00" + normalized.Content
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, *normalized)
	}

	if len(merged) > limit {
		merged = merged[len(merged)-limit:]
	}
	return merged
}

func buildAISystemPrompt(now time.Time, dataContext string, scope aiContextScope) string {
	return buildAISystemPromptWithSections(now, dataContext, scope.sectionNames())
}

func buildAISystemPromptWithSections(now time.Time, dataContext string, sectionNames []string) string {
	return fmt.Sprintf(`Ты персональный AI-ассистент приложения Life Dashboard.
Твоя единственная функция — анализировать данные пользователя: финансы, продуктивность/задачи, здоровье, физическую активность, тренировки, Hevy routines/шаблоны, привычки, питание, дневник и календарь.
Отвечай на русском языке. Давай конкретные ответы основанные на реальных данных ниже. Будь краток и по делу.
Ты не можешь выполнять команды, изменять данные или делать что-либо за пределами анализа предоставленных данных.
Если просят что-то сделать с базой данных, кодом или системой — вежливо объясни что ты только аналитик данных.

Используй историю текущего диалога как рабочий контекст.
- Если пользователь уточнил или исправил тебя, считай это более приоритетным, чем свои предыдущие предположения.
- Если нужные числа или ограничения даны пользователем прямо в чате, используй их в расчётах и явно отмечай, что это данные из текущего диалога.
- Не отвечай, что данных нет, если нужная информация уже была дана пользователем несколькими сообщениями выше.
- Для арифметики и рекомендаций показывай короткий расчёт.
- Для упражнений с гантелями, блинами и штангой не путай вес на одну гантель, вес пары и общий вес штанги. Если это неясно, сначала уточни.
- Календарь — это только план из Google Calendar, а не подтверждение факта. Не пиши "был в зале", "лёг спать" или "встретился", если у тебя есть только календарное событие.
- Сон, шаги, вес, пульс и HRV — это факт только если они есть в разделе здоровья из Apple Health/biometrics/sleep_sessions.
- Задачи и продуктивность берутся из Todoist. Просрочка, план на сегодня и завершённые задачи считай по todoist_tasks и todoist_task_completions, а не по календарю.
- Питание — это только залогированные записи. Не пиши "отслежено полностью" или "ужин был", если в данных нет явного подтверждения.
- Данные ниже приходят как результаты внутренних tools в JSON. Сначала смотри на поля tool/section/window/data. Если внутри есть data, это приоритетный структурированный payload. context_text — вспомогательная summary этого tool.

Сейчас особенно релевантны разделы данных: %s.

Текущие данные пользователя (обновлено %s):
%s`, strings.Join(sectionNames, ", "), now.Format("02.01.2006 15:04"), dataContext)
}

func (h *AIHandler) complete(ctx context.Context, messages []ChatMessage) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":    h.model,
		"messages": messages,
		"stream":   false,
	})
	if err != nil {
		h.logger.Error().Err(err).Msg("marshal ai request")
		return "", err
	}

	upstreamCtx, cancel := context.WithTimeout(ctx, aiUpstreamRequestTimeout)
	defer cancel()

	apiReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost,
		h.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
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
		return "", errAIUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, aiUpstreamResponseLogSize))
		h.logger.Error().
			Int("status", resp.StatusCode).
			Str("body", strings.TrimSpace(string(body))).
			Msg("ai api error")
		return "", errAIUpstream
	}

	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		h.logger.Error().Err(err).Msg("read ai response")
		return "", errAIUnavailable
	}

	var completion struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rawResp, &completion); err != nil {
		h.logger.Error().Err(err).Str("body", truncateAIText(string(rawResp), aiUpstreamResponseLogSize)).Msg("decode ai response")
		return "", errAIBadResponse
	}
	if len(completion.Choices) == 0 {
		h.logger.Error().Msg("ai response has no choices")
		return "", errAIBadResponse
	}

	content := normalizeAIContent(completion.Choices[0].Message.Content)
	if strings.TrimSpace(content) == "" {
		h.logger.Error().Msg("ai response content is empty")
		return "", errAIBadResponse
	}

	return content, nil
}

func writeAICompletionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAIUnavailable):
		http.Error(w, "AI сервис временно недоступен. Попробуй позже.", http.StatusServiceUnavailable)
	case errors.Is(err, errAIUpstream):
		http.Error(w, "AI сервис вернул ошибку. Попробуй позже.", http.StatusBadGateway)
	case errors.Is(err, errAIBadResponse):
		http.Error(w, "AI сервис не вернул корректный ответ. Попробуй позже.", http.StatusBadGateway)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func normalizeAIContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := obj["text"].(string); ok && text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func (h *AIHandler) buildContext(ctx context.Context, userID string, scope aiContextScope) (string, error) {
	var sb strings.Builder
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))

	// === ФИНАНСЫ ===
	if scope.finance {
		sb.WriteString("=== ФИНАНСЫ ===\n")

		rows, err := h.db.Query(ctx, `
			SELECT title, currency, balance, type, in_balance
			FROM accounts
			WHERE balance != 0 AND user_id = $1
			ORDER BY in_balance DESC, balance DESC LIMIT 10
		`, userID)
		if err == nil {
			sb.WriteString("Счета:\n")
			for rows.Next() {
				var title, currency, accType string
				var balance float64
				var inBalance bool
				if err := rows.Scan(&title, &currency, &balance, &accType, &inBalance); err == nil {
					visibility := ""
					if !inBalance {
						visibility = " [вне баланса]"
					}
					sb.WriteString(fmt.Sprintf("  - %s (%s%s): %.0f %s\n", title, accType, visibility, balance, currency))
				}
			}
			rows.Close()
		}

		var totalBalance, monthSpending, monthIncome float64
		h.db.QueryRow(ctx, `SELECT COALESCE(SUM(balance),0) FROM accounts WHERE currency='RUB' AND in_balance = TRUE AND user_id = $1`, userID).Scan(&totalBalance)
		h.db.QueryRow(ctx, `
			SELECT
				COALESCE(SUM(CASE WHEN t.amount < 0 THEN ABS(t.amount) ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN t.amount > 0 THEN t.amount ELSE 0 END), 0)
			FROM transactions t
			LEFT JOIN accounts a ON a.id = t.account_id
			WHERE t.currency='RUB'
				AND t.occurred_at >= $1
				AND t.is_transfer=false
				AND t.user_id = $2
				AND COALESCE(a.in_balance, TRUE) = TRUE
		`, monthStart, userID).Scan(&monthSpending, &monthIncome)

		sb.WriteString(fmt.Sprintf("Общий баланс (RUB): %.0f ₽\n", totalBalance))
		sb.WriteString(fmt.Sprintf("Расходы за текущий месяц: %.0f ₽\n", monthSpending))
		sb.WriteString(fmt.Sprintf("Доходы за текущий месяц: %.0f ₽\n", monthIncome))

		txRows, err := h.db.Query(ctx, `
			SELECT t.occurred_at, t.amount, t.currency, COALESCE(t.payee, t.comment, '') as label
			FROM transactions t
			LEFT JOIN accounts a ON a.id = t.account_id
			WHERE t.is_transfer=false
				AND t.user_id = $1
				AND COALESCE(a.in_balance, TRUE) = TRUE
			ORDER BY t.occurred_at DESC LIMIT 30
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
	}

	// === АКТИВНОСТИ ===
	if scope.activities {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("=== АКТИВНОСТИ ===\n")
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
	}

	if scope.productivity {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		if err := h.appendProductivityContextInRange(ctx, &sb, userID, now.AddDate(0, 0, -14), now, "=== ПРОДУКТИВНОСТЬ ===", 12); err != nil {
			return "", err
		}
	}

	if scope.health {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		h.appendHealthContextInRange(ctx, &sb, userID, now.AddDate(0, 0, -30), now, "=== ЗДОРОВЬЕ ===")
	}

	// === ТРЕНИРОВКИ ===
	if scope.workouts {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("=== ТРЕНИРОВКИ ===\n")
		var weekWorkouts int
		h.db.QueryRow(ctx, `SELECT COUNT(*) FROM workouts WHERE started_at >= $1 AND user_id = $2`, weekStart, userID).Scan(&weekWorkouts)
		sb.WriteString(fmt.Sprintf("За эту неделю: %d тренировок\n", weekWorkouts))

		workoutContext, err := h.buildRecentWorkoutContext(ctx, userID)
		if err == nil {
			sb.WriteString("Ниже приведены последние тренировки с реальными упражнениями и подходами, если они сохранены в базе:\n")
			sb.WriteString(workoutContext)
		}
	}

	if scope.routines {
		routineContext, err := h.buildRoutineContext(ctx, userID, 4)
		if err == nil && strings.TrimSpace(routineContext) != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString("=== HEVY ROUTINES ===\n")
			sb.WriteString("Ниже приведены шаблоны/routines из Hevy. Это плановые упражнения и веса, а не подтверждение факта выполнения.\n")
			sb.WriteString(routineContext)
		}
	}

	if scope.habits {
		habitContext, err := h.buildHabitContext(ctx, userID, 30)
		if err == nil && strings.TrimSpace(habitContext) != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(habitContext)
		}
	}

	// === ПИТАНИЕ ===
	if scope.nutrition {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("=== ПИТАНИЕ ===\n")
		if targets, err := loadNutritionTargets(ctx, h.db, userID); err == nil {
			sb.WriteString(renderNutritionTargetsForAI(targets))
		}
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
	}

	// === ДНЕВНИК ===
	if scope.journal {
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
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString("=== ДНЕВНИК (последние 30 дней) ===\n")
				for _, e := range journalEntries {
					sb.WriteString(e + "\n")
				}
			}
		}
	}

	// === КАЛЕНДАРЬ ===
	if scope.calendar {
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
					calEvents = append(calEvents, formatAICalendarEvent(startTime, endTime, allDay, title, location))
				}
			}
			calRows.Close()
			if len(calEvents) > 0 {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString("=== КАЛЕНДАРЬ (30 дней назад — 30 дней вперёд; это план, не факт) ===\n")
				for _, e := range calEvents {
					sb.WriteString(e + "\n")
				}
			}
		}
	}

	// === ПОГОДА ===
	if scope.weather && h.weather != nil {
		if wd, err := h.weather.Fetch(0, 0, ""); err == nil {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString("=== ПОГОДА ===\n")
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

func defaultAIContextScope() aiContextScope {
	return aiContextScope{
		finance:      true,
		productivity: true,
		activities:   true,
		health:       true,
		workouts:     true,
		routines:     true,
		habits:       true,
		nutrition:    true,
		journal:      true,
		calendar:     true,
		weather:      true,
	}
}

func (s aiContextScope) empty() bool {
	return !s.finance && !s.productivity && !s.activities && !s.health && !s.workouts && !s.routines && !s.habits && !s.nutrition && !s.journal && !s.calendar && !s.weather
}

func (s aiContextScope) sectionNames() []string {
	names := make([]string, 0, 11)
	if s.finance {
		names = append(names, "финансы")
	}
	if s.productivity {
		names = append(names, "продуктивность")
	}
	if s.activities {
		names = append(names, "активности")
	}
	if s.health {
		names = append(names, "здоровье")
	}
	if s.workouts {
		names = append(names, "тренировки")
	}
	if s.routines {
		names = append(names, "hevy routines")
	}
	if s.habits {
		names = append(names, "привычки")
	}
	if s.nutrition {
		names = append(names, "питание")
	}
	if s.journal {
		names = append(names, "дневник")
	}
	if s.calendar {
		names = append(names, "календарь")
	}
	if s.weather {
		names = append(names, "погода")
	}
	if len(names) == 0 {
		return defaultAIContextScope().sectionNames()
	}
	return names
}

func selectAIContextScope(message string, history []ChatMessage) aiContextScope {
	scope := aiContextScope{}
	text := strings.ToLower(message)
	recentHistory := recentHistoryText(history, 6)
	combined := strings.TrimSpace(strings.Join([]string{text, recentHistory}, "\n"))

	financeKeywords := []string{"финанс", "деньг", "расход", "доход", "баланс", "трат", "бюджет", "транзак", "счет", "счёт", "руб"}
	productivityKeywords := []string{"задач", "todo", "task", "todoist", "продуктив", "дедлайн", "срок", "просроч", "сделать сегодня", "перегруз", "нагрузка по задачам", "completed today", "overdue", "висят", "висит", "план на день", "план задач"}
	activityKeywords := []string{"актив", "бег", "пробеж", "килом", "км", "ходьб", "вел", "плав", "дистанц", "шаг", "strava", "run", "ride"}
	healthKeywords := []string{"здоров", "сон", "спал", "сплю", "пульс", "сердц", "hrv", "вес", "взвеш", "шаг", "apple health", "health", "zepp", "amazfit", "кислород", "spo2", "vo2"}
	workoutKeywords := []string{"тренир", "упражнен", "жим", "тяга", "присед", "гантел", "штанг", "блин", "гриф", "подход", "повтор", "hevy", "workout", "pull", "push", "legs", "зал", "вес"}
	routineKeywords := []string{"routine", "routines", "рутин", "шаблон", "сплит", "программ", "план трениров", "template"}
	habitKeywords := []string{"привыч", "habit", "habitify", "todoist", "зуб", "умы", "лиц", "уход", "skincare", "cleanser", "чеклист", "дейли", "daily"}
	nutritionKeywords := []string{"питан", "калор", "кбжу", "бжу", "еда", "ккал", "углев", "белк", "жир", "fatsecret", "myfitnesspal", "mfp"}
	journalKeywords := []string{"дневник", "journal", "ноушн", "notion", "настроен", "рефлекс", "запис"}
	calendarKeywords := []string{"календар", "встреч", "событи", "созвон", "митинг", "расписан", "план"}
	weatherKeywords := []string{"погод", "температ", "дожд", "ветер", "на улице"}
	generalKeywords := []string{"сводк", "обзор", "проанализ", "анализ", "итог", "общ", "что происходит", "что нового", "удивил"}

	if containsAny(combined, financeKeywords...) {
		scope.finance = true
	}
	if containsAny(combined, productivityKeywords...) {
		scope.productivity = true
	}
	if containsAny(combined, activityKeywords...) {
		scope.activities = true
	}
	if containsAny(combined, healthKeywords...) {
		scope.health = true
	}
	if containsAny(combined, workoutKeywords...) {
		scope.workouts = true
	}
	if containsAny(combined, routineKeywords...) {
		scope.routines = true
	}
	if containsAny(combined, habitKeywords...) {
		scope.habits = true
	}
	if containsAny(combined, nutritionKeywords...) {
		scope.nutrition = true
	}
	if containsAny(combined, journalKeywords...) {
		scope.journal = true
	}
	if containsAny(combined, calendarKeywords...) {
		scope.calendar = true
	}
	if containsAny(combined, weatherKeywords...) {
		scope.weather = true
	}

	if strings.Contains(combined, "фитнес") || strings.Contains(combined, "нагруз") {
		scope.activities = true
		scope.workouts = true
	}

	if scope.empty() && containsAny(combined, generalKeywords...) {
		return defaultAIContextScope()
	}
	if scope.empty() {
		return defaultAIContextScope()
	}
	return scope
}

func recentHistoryText(history []ChatMessage, limit int) string {
	if len(history) == 0 || limit <= 0 {
		return ""
	}
	start := len(history) - limit
	if start < 0 {
		start = 0
	}
	parts := make([]string, 0, len(history)-start)
	for _, msg := range history[start:] {
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		parts = append(parts, strings.ToLower(msg.Content))
	}
	return strings.Join(parts, "\n")
}

func containsAny(text string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func (h *AIHandler) buildRecentWorkoutContext(ctx context.Context, userID string) (string, error) {
	return h.buildRecentWorkoutContextLimit(ctx, userID, 10)
}

func (h *AIHandler) buildRecentWorkoutContextLimit(ctx context.Context, userID string, limit int) (string, error) {
	data, err := h.buildRecentWorkoutsData(ctx, userID, limit)
	if err != nil {
		return "", err
	}
	return renderRecentWorkoutsText("", data), nil
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

func formatAITimestampLocal(t time.Time, layout string) string {
	return t.In(time.Local).Format(layout)
}

func formatAICalendarEvent(startTime, endTime time.Time, allDay bool, title, location string) string {
	locationPart := ""
	if location != "" {
		locationPart = " @ " + location
	}
	if allDay {
		return fmt.Sprintf("  %s: %s (весь день)%s", formatAITimestampLocal(startTime, "02.01"), title, locationPart)
	}
	return fmt.Sprintf("  %s %s-%s: %s%s",
		formatAITimestampLocal(startTime, "02.01"),
		formatAITimestampLocal(startTime, "15:04"),
		formatAITimestampLocal(endTime, "15:04"),
		title,
		locationPart,
	)
}

func (h *AIHandler) buildRoutineContext(ctx context.Context, userID string, limit int) (string, error) {
	data, err := h.buildRoutineOverviewData(ctx, userID, limit)
	if err != nil {
		return "", err
	}
	return renderRoutineOverviewText("", data), nil
}
