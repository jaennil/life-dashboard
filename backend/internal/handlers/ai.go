package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type AIHandler struct {
	db      *pgxpool.Pool
	baseURL string
	model   string
	apiKey  string
	weather *WeatherHandler
	logger  zerolog.Logger
}

func NewAI(db *pgxpool.Pool, baseURL, model, apiKey string, weather *WeatherHandler, logger zerolog.Logger) *AIHandler {
	return &AIHandler{
		db:      db,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		apiKey:  apiKey,
		weather: weather,
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

func (h *AIHandler) Chat(w http.ResponseWriter, r *http.Request) {
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

	dataContext, err := h.buildContext(ctx)
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

	body, _ := json.Marshal(map[string]any{
		"model":    h.model,
		"messages": messages,
		"stream":   true,
	})

	apiReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		h.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	apiReq.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		apiReq.Header.Set("Authorization", "Bearer "+h.apiKey)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(apiReq)
	if err != nil {
		h.logger.Error().Err(err).Msg("ai api request")
		http.Error(w, "AI недоступен", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.logger.Error().Int("status", resp.StatusCode).Msg("ai api error")
		http.Error(w, "AI вернул ошибку", http.StatusBadGateway)
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

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
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
		fmt.Fprintf(w, "data: %s\n\n", content)
		flusher.Flush()
	}
}

func (h *AIHandler) buildContext(ctx context.Context) (string, error) {
	var sb strings.Builder
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))

	// === ФИНАНСЫ ===
	sb.WriteString("=== ФИНАНСЫ ===\n")

	rows, err := h.db.Query(ctx, `
		SELECT title, currency, balance, type
		FROM accounts WHERE balance != 0
		ORDER BY balance DESC LIMIT 10
	`)
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
	h.db.QueryRow(ctx, `SELECT COALESCE(SUM(balance),0) FROM accounts WHERE currency='RUB' AND balance > 0`).Scan(&totalBalance)
	h.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN amount < 0 THEN ABS(amount) ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END), 0)
		FROM transactions
		WHERE currency='RUB' AND occurred_at >= $1 AND is_transfer=false
	`, monthStart).Scan(&monthSpending, &monthIncome)

	sb.WriteString(fmt.Sprintf("Общий баланс (RUB): %.0f ₽\n", totalBalance))
	sb.WriteString(fmt.Sprintf("Расходы за текущий месяц: %.0f ₽\n", monthSpending))
	sb.WriteString(fmt.Sprintf("Доходы за текущий месяц: %.0f ₽\n", monthIncome))

	txRows, err := h.db.Query(ctx, `
		SELECT occurred_at, amount, currency, COALESCE(payee, comment, '') as label
		FROM transactions WHERE is_transfer=false
		ORDER BY occurred_at DESC LIMIT 30
	`)
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
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE started_at >= $1`, weekStart).Scan(&weekActivities)
	h.db.QueryRow(ctx, `SELECT COALESCE(SUM(distance_meters)/1000.0,0) FROM activities WHERE started_at >= $1`, weekStart).Scan(&weekDistanceKm)
	sb.WriteString(fmt.Sprintf("За эту неделю: %d активностей, %.1f км\n", weekActivities, weekDistanceKm))

	actRows, err := h.db.Query(ctx, `
		SELECT started_at, type, COALESCE(distance_meters/1000.0,0), COALESCE(duration_seconds/60,0), name
		FROM activities ORDER BY started_at DESC LIMIT 10
	`)
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
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM workouts WHERE started_at >= $1`, weekStart).Scan(&weekWorkouts)
	sb.WriteString(fmt.Sprintf("За эту неделю: %d тренировок\n", weekWorkouts))

	wRows, err := h.db.Query(ctx, `
		SELECT id, started_at, title FROM workouts ORDER BY started_at DESC LIMIT 10
	`)
	if err == nil {
		type workoutEntry struct {
			id       string
			startedAt time.Time
			title    string
		}
		var recentWorkouts []workoutEntry
		for wRows.Next() {
			var w workoutEntry
			if err := wRows.Scan(&w.id, &w.startedAt, &w.title); err == nil {
				recentWorkouts = append(recentWorkouts, w)
			}
		}
		wRows.Close()

		for _, w := range recentWorkouts {
			sb.WriteString(fmt.Sprintf("\nТренировка %s: %s\n", w.startedAt.Format("02.01.2006 15:04"), w.title))

			setRows, err := h.db.Query(ctx, `
				SELECT exercise_name, exercise_category, set_index, set_type,
				       COALESCE(weight_kg::text, '-'), COALESCE(reps::text, '-')
				FROM workout_sets
				WHERE workout_id = $1
				ORDER BY exercise_name, set_index
			`, w.id)
			if err != nil {
				continue
			}

			type exKey = string
			exSets := map[exKey][]string{}
			exOrder := []string{}
			for setRows.Next() {
				var exName, exCat, setIdx, setType, weightKg, reps string
				if err := setRows.Scan(&exName, &exCat, &setIdx, &setType, &weightKg, &reps); err != nil {
					continue
				}
				if _, ok := exSets[exName]; !ok {
					label := exName
					if exCat != "" {
						label += " (" + exCat + ")"
					}
					exOrder = append(exOrder, exName)
					exSets[exName] = []string{}
					_ = label
				}
				setDesc := fmt.Sprintf("Подход %s: %s кг x %s", setIdx, weightKg, reps)
				if setType != "normal" && setType != "" {
					setDesc += " [" + setType + "]"
				}
				exSets[exName] = append(exSets[exName], setDesc)
			}
			setRows.Close()

			for _, exName := range exOrder {
				sb.WriteString(fmt.Sprintf("  %s:\n", exName))
				for _, s := range exSets[exName] {
					sb.WriteString(fmt.Sprintf("    %s\n", s))
				}
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
