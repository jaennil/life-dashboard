package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type aiToolName string

const (
	aiToolFinanceOverview      aiToolName = "finance_overview"
	aiToolRecentTransactions   aiToolName = "recent_transactions"
	aiToolProductivityOverview aiToolName = "productivity_overview"
	aiToolActivityOverview     aiToolName = "activity_overview"
	aiToolRecentActivities     aiToolName = "recent_activities"
	aiToolHealthOverview       aiToolName = "health_overview"
	aiToolWorkoutOverview      aiToolName = "workout_overview"
	aiToolRecentWorkouts       aiToolName = "recent_workouts"
	aiToolRoutineOverview      aiToolName = "routine_overview"
	aiToolHabitOverview        aiToolName = "habit_overview"
	aiToolNutritionOverview    aiToolName = "nutrition_overview"
	aiToolJournalOverview      aiToolName = "journal_overview"
	aiToolCalendarOverview     aiToolName = "calendar_overview"
	aiToolWeatherOverview      aiToolName = "weather_overview"
)

const (
	aiPlannerHistoryLimit = 6
	aiPlannerMaxTools     = 4
)

type aiToolCall struct {
	Name       aiToolName `json:"name"`
	Days       int        `json:"days,omitempty"`
	Limit      int        `json:"limit,omitempty"`
	PastDays   int        `json:"past_days,omitempty"`
	FutureDays int        `json:"future_days,omitempty"`
}

type aiToolPlan struct {
	Tools []aiToolCall `json:"tools"`
}

func (h *AIHandler) buildChatContext(ctx context.Context, userID, message string, history []ChatMessage) (string, []string, error) {
	toolCalls, err := h.planToolCalls(ctx, message, history)
	if err != nil {
		h.logger.Warn().Err(err).Msg("ai planner failed, using fallback context")
		scope := selectAIContextScope(message, history)
		contextText, buildErr := h.buildContext(ctx, userID, scope)
		return contextText, scope.sectionNames(), buildErr
	}

	run, err := h.runAITools(ctx, userID, h.chatToolExecutions(ctx, userID, toolCalls))
	if err != nil {
		h.logger.Warn().Err(err).Msg("ai tool execution failed, using fallback context")
		scope := selectAIContextScope(message, history)
		fallbackText, buildErr := h.buildContext(ctx, userID, scope)
		return fallbackText, scope.sectionNames(), buildErr
	}
	contextText := run.render()
	if strings.TrimSpace(contextText) == "" {
		scope := selectAIContextScope(message, history)
		fallbackText, buildErr := h.buildContext(ctx, userID, scope)
		return fallbackText, scope.sectionNames(), buildErr
	}

	h.logger.Info().
		Str("user_id", userID).
		Strs("tools", aiToolNames(toolCalls)).
		Strs("sections", run.Sections).
		Msg("ai chat context built")

	return contextText, run.Sections, nil
}

func (h *AIHandler) planToolCalls(ctx context.Context, message string, history []ChatMessage) ([]aiToolCall, error) {
	recentHistory := recentHistoryText(history, aiPlannerHistoryLimit)
	plannerMessages := []ChatMessage{
		{
			Role:    "system",
			Content: buildAIToolPlannerPrompt(message, recentHistory),
		},
		{
			Role: "user",
			Content: fmt.Sprintf("Вопрос пользователя:\n%s\n\nНедавняя история:\n%s",
				strings.TrimSpace(message),
				emptyFallback(recentHistory, "нет")),
		},
	}

	rawPlan, err := h.complete(ctx, "tool_planner", plannerMessages)
	if err != nil {
		return nil, err
	}

	toolCalls, err := parseAIToolPlan(rawPlan)
	if err != nil {
		return nil, err
	}
	if len(toolCalls) == 0 {
		return fallbackToolPlan(message, history), nil
	}
	return toolCalls, nil
}

func buildAIToolPlannerPrompt(message, recentHistory string) string {
	var sb strings.Builder
	sb.WriteString("Ты планировщик внутренних data-tools для Life Dashboard.\n")
	sb.WriteString("Твоя задача — по вопросу пользователя выбрать минимальный набор внутренних tools, чтобы потом другой шаг дал точный ответ.\n")
	sb.WriteString("Верни только JSON без markdown и без пояснений.\n")
	sb.WriteString("Формат ответа:\n")
	sb.WriteString("{\"tools\":[{\"name\":\"finance_overview\",\"days\":30},{\"name\":\"recent_transactions\",\"days\":30,\"limit\":8}]}\n")
	sb.WriteString("Разрешённые tools:\n")
	sb.WriteString("- finance_overview: баланс, доходы/расходы, агрегаты по финансам за период; args: days\n")
	sb.WriteString("- recent_transactions: последние транзакции за период; args: days, limit\n")
	sb.WriteString("- productivity_overview: Todoist задачи, просрочка, сделано за период, план на ближайшие дни; args: days\n")
	sb.WriteString("- activity_overview: сводка по активностям за период; args: days\n")
	sb.WriteString("- recent_activities: последние активности; args: days, limit\n")
	sb.WriteString("- health_overview: фактические метрики здоровья из Apple Health/biometrics: шаги, сон, пульс, HRV, вес; args: days\n")
	sb.WriteString("- workout_overview: сводка по тренировкам и упражнениям за период; args: days\n")
	sb.WriteString("- recent_workouts: последние тренировки с упражнениями и подходами; args: limit\n")
	sb.WriteString("- routine_overview: Hevy routines/шаблоны с плановыми упражнениями и весами; args: limit\n")
	sb.WriteString("- habit_overview: привычки и рутины (Life Dashboard / Habitify / Todoist) с дневными статусами за период; args: days\n")
	sb.WriteString("- nutrition_overview: калории и макросы за период; args: days\n")
	sb.WriteString("- journal_overview: недавние записи дневника; args: days, limit\n")
	sb.WriteString("- calendar_overview: недавние и будущие события календаря; args: past_days, future_days, limit\n")
	sb.WriteString("- weather_overview: текущая погода и краткий прогноз; args: none\n")
	sb.WriteString("Правила:\n")
	sb.WriteString("- Выбирай от 1 до 4 tools.\n")
	sb.WriteString("- Не добавляй tools наугад.\n")
	sb.WriteString("- Для вопросов про рабочие веса, упражнения, последнюю тренировку и прогресс по залу обязательно используй recent_workouts; при общей оценке тренинга добавь workout_overview.\n")
	sb.WriteString("- Для вопросов про Hevy routine, шаблон тренировки, сплит, плановые веса и план упражнений используй routine_overview; при сравнении плана с фактом добавь recent_workouts или workout_overview.\n")
	sb.WriteString("- Для вопросов про привычки, чеклисты, зубы, душ, уход за лицом, умывание, встроенные рутины Life Dashboard и Habitify используй habit_overview.\n")
	sb.WriteString("- Для вопросов про траты, баланс, расходы по категориям или подозрительные операции используй finance_overview; при вопросах про конкретные покупки/операции добавь recent_transactions.\n")
	sb.WriteString("- Для вопросов про задачи, overdue, что сделать сегодня, что висит давно, продуктивность и перегруз по Todoist используй productivity_overview.\n")
	sb.WriteString("- Для бега, дистанции, активности и шагов используй activity_overview и при необходимости recent_activities.\n")
	sb.WriteString("- Для вопросов про сон, пульс, HRV, вес, Apple Health, Zepp и шаги за день используй health_overview. Шаги из health_overview приоритетнее календаря и Strava.\n")
	sb.WriteString("- Для питания используй nutrition_overview.\n")
	sb.WriteString("- Если вопрос общий, но не checkup, всё равно выбирай только реально нужные domains.\n")
	sb.WriteString("- Если вопрос не требует погоды, не выбирай weather_overview.\n")
	sb.WriteString("- Если вопрос не требует дневника или календаря, не выбирай их.\n")
	sb.WriteString("Вопрос пользователя:\n")
	sb.WriteString(strings.TrimSpace(message))
	if strings.TrimSpace(recentHistory) != "" {
		sb.WriteString("\n\nНедавняя история:\n")
		sb.WriteString(recentHistory)
	}
	return sb.String()
}

func parseAIToolPlan(raw string) ([]aiToolCall, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	start := strings.Index(cleaned, "{")
	end := strings.LastIndex(cleaned, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("planner returned non-json response")
	}

	var plan aiToolPlan
	if err := json.Unmarshal([]byte(cleaned[start:end+1]), &plan); err != nil {
		return nil, fmt.Errorf("decode planner response: %w", err)
	}

	return sanitizeAIToolPlan(plan), nil
}

func sanitizeAIToolPlan(plan aiToolPlan) []aiToolCall {
	calls := make([]aiToolCall, 0, len(plan.Tools))
	seen := make(map[aiToolName]bool)

	for _, call := range plan.Tools {
		call.Name = aiToolName(strings.TrimSpace(string(call.Name)))
		if !isAllowedAITool(call.Name) || seen[call.Name] {
			continue
		}

		switch call.Name {
		case aiToolFinanceOverview:
			call.Days = normalizeAIDays(call.Days, 30, 365)
		case aiToolRecentTransactions:
			call.Days = normalizeAIDays(call.Days, 30, 365)
			call.Limit = normalizeAILimit(call.Limit, 10, 20)
		case aiToolProductivityOverview:
			call.Days = normalizeAIDays(call.Days, 14, 90)
		case aiToolActivityOverview:
			call.Days = normalizeAIDays(call.Days, 14, 180)
		case aiToolRecentActivities:
			call.Days = normalizeAIDays(call.Days, 30, 180)
			call.Limit = normalizeAILimit(call.Limit, 8, 15)
		case aiToolHealthOverview:
			call.Days = normalizeAIDays(call.Days, 14, 180)
		case aiToolWorkoutOverview:
			call.Days = normalizeAIDays(call.Days, 30, 180)
		case aiToolRecentWorkouts:
			call.Limit = normalizeAILimit(call.Limit, 4, 8)
		case aiToolRoutineOverview:
			call.Limit = normalizeAILimit(call.Limit, 4, 8)
		case aiToolHabitOverview:
			call.Days = normalizeAIDays(call.Days, 21, 120)
		case aiToolNutritionOverview:
			call.Days = normalizeAIDays(call.Days, 14, 90)
		case aiToolJournalOverview:
			call.Days = normalizeAIDays(call.Days, 30, 90)
			call.Limit = normalizeAILimit(call.Limit, 8, 12)
		case aiToolCalendarOverview:
			call.PastDays = normalizeAIDays(call.PastDays, 7, 30)
			call.FutureDays = normalizeAIDays(call.FutureDays, 30, 90)
			call.Limit = normalizeAILimit(call.Limit, 20, 30)
		case aiToolWeatherOverview:
			call.Days = 0
			call.Limit = 0
			call.PastDays = 0
			call.FutureDays = 0
		}

		calls = append(calls, call)
		seen[call.Name] = true
		if len(calls) >= aiPlannerMaxTools {
			break
		}
	}

	return calls
}

func fallbackToolPlan(message string, history []ChatMessage) []aiToolCall {
	scope := selectAIContextScope(message, history)
	calls := make([]aiToolCall, 0, aiPlannerMaxTools)

	appendCall := func(call aiToolCall) {
		if len(calls) >= aiPlannerMaxTools {
			return
		}
		for _, existing := range calls {
			if existing.Name == call.Name {
				return
			}
		}
		calls = append(calls, call)
	}

	if scope.finance {
		appendCall(aiToolCall{Name: aiToolFinanceOverview, Days: 30})
		appendCall(aiToolCall{Name: aiToolRecentTransactions, Days: 30, Limit: 10})
	}
	if scope.productivity {
		appendCall(aiToolCall{Name: aiToolProductivityOverview, Days: 14})
	}
	if scope.activities {
		appendCall(aiToolCall{Name: aiToolActivityOverview, Days: 14})
		appendCall(aiToolCall{Name: aiToolRecentActivities, Days: 30, Limit: 8})
	}
	if scope.health {
		appendCall(aiToolCall{Name: aiToolHealthOverview, Days: 14})
	}
	if scope.workouts {
		appendCall(aiToolCall{Name: aiToolWorkoutOverview, Days: 30})
		appendCall(aiToolCall{Name: aiToolRecentWorkouts, Limit: 4})
	}
	if scope.routines {
		appendCall(aiToolCall{Name: aiToolRoutineOverview, Limit: 4})
	}
	if scope.habits {
		appendCall(aiToolCall{Name: aiToolHabitOverview, Days: 21})
	}
	if scope.nutrition {
		appendCall(aiToolCall{Name: aiToolNutritionOverview, Days: 14})
	}
	if scope.journal {
		appendCall(aiToolCall{Name: aiToolJournalOverview, Days: 30, Limit: 8})
	}
	if scope.calendar {
		appendCall(aiToolCall{Name: aiToolCalendarOverview, PastDays: 7, FutureDays: 30, Limit: 20})
	}
	if scope.weather {
		appendCall(aiToolCall{Name: aiToolWeatherOverview})
	}

	if len(calls) == 0 {
		appendCall(aiToolCall{Name: aiToolFinanceOverview, Days: 30})
		appendCall(aiToolCall{Name: aiToolProductivityOverview, Days: 14})
		appendCall(aiToolCall{Name: aiToolWorkoutOverview, Days: 30})
		appendCall(aiToolCall{Name: aiToolHealthOverview, Days: 14})
		appendCall(aiToolCall{Name: aiToolActivityOverview, Days: 14})
	}

	return calls
}

func isAllowedAITool(name aiToolName) bool {
	switch name {
	case aiToolFinanceOverview, aiToolRecentTransactions, aiToolProductivityOverview, aiToolActivityOverview, aiToolRecentActivities,
		aiToolHealthOverview, aiToolWorkoutOverview, aiToolRecentWorkouts, aiToolRoutineOverview, aiToolHabitOverview, aiToolNutritionOverview, aiToolJournalOverview,
		aiToolCalendarOverview, aiToolWeatherOverview:
		return true
	default:
		return false
	}
}

func normalizeAIDays(value, fallback, max int) int {
	if value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func normalizeAILimit(value, fallback, max int) int {
	if value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func (h *AIHandler) chatToolExecutions(ctx context.Context, userID string, tools []aiToolCall) []aiToolExecution {
	executions := make([]aiToolExecution, 0, len(tools))

	for _, call := range tools {
		call := call

		switch call.Name {
		case aiToolFinanceOverview:
			end := time.Now()
			start := end.AddDate(0, 0, -call.Days)
			var cached *AIFinanceOverviewData
			load := func() (AIFinanceOverviewData, error) {
				if cached != nil {
					return *cached, nil
				}
				data, err := h.buildFinanceOverviewInRange(ctx, userID, start, end)
				if err != nil {
					return AIFinanceOverviewData{}, err
				}
				cached = &data
				return data, nil
			}
			executions = append(executions, aiToolExecution{
				Name:    call.Name,
				Section: "финансы",
				Days:    call.Days,
				Run: func(sb *strings.Builder) error {
					data, err := load()
					if err != nil {
						return err
					}
					sb.WriteString(renderFinanceOverviewText(fmt.Sprintf("=== ФИНАНСЫ (%d дней) ===", call.Days), data))
					return nil
				},
				Data: func() (any, error) {
					data, err := load()
					if err != nil {
						return nil, err
					}
					return data, nil
				},
			})
		case aiToolRecentTransactions:
			end := time.Now()
			start := end.AddDate(0, 0, -call.Days)
			var cached *AIRecentTransactionsData
			load := func() (AIRecentTransactionsData, error) {
				if cached != nil {
					return *cached, nil
				}
				data, err := h.buildRecentTransactionsInRange(ctx, userID, start, end, call.Limit)
				if err != nil {
					return AIRecentTransactionsData{}, err
				}
				cached = &data
				return data, nil
			}
			executions = append(executions, aiToolExecution{
				Name:    call.Name,
				Section: "финансы",
				Days:    call.Days,
				Limit:   call.Limit,
				Run: func(sb *strings.Builder) error {
					data, err := load()
					if err != nil {
						return err
					}
					sb.WriteString(renderRecentTransactionsText(fmt.Sprintf("=== ПОСЛЕДНИЕ ТРАНЗАКЦИИ (%d дней, %d шт.) ===", call.Days, call.Limit), data))
					return nil
				},
				Data: func() (any, error) {
					data, err := load()
					if err != nil {
						return nil, err
					}
					return data, nil
				},
			})
		case aiToolProductivityOverview:
			end := time.Now()
			start := end.AddDate(0, 0, -call.Days)
			var cached *AIProductivityOverviewData
			load := func() (AIProductivityOverviewData, error) {
				if cached != nil {
					return *cached, nil
				}
				data, err := h.buildProductivityOverviewInRange(ctx, userID, start, end, 10)
				if err != nil {
					return AIProductivityOverviewData{}, err
				}
				cached = &data
				return data, nil
			}
			executions = append(executions, aiToolExecution{
				Name:    call.Name,
				Section: "продуктивность",
				Days:    call.Days,
				Run: func(sb *strings.Builder) error {
					data, err := load()
					if err != nil {
						return err
					}
					sb.WriteString(renderProductivityOverviewText(fmt.Sprintf("=== ПРОДУКТИВНОСТЬ (%d дней) ===", call.Days), data))
					return nil
				},
				Data: func() (any, error) {
					data, err := load()
					if err != nil {
						return nil, err
					}
					return data, nil
				},
			})
		case aiToolActivityOverview:
			executions = append(executions, aiToolExecution{
				Name:    call.Name,
				Section: "активности",
				Days:    call.Days,
				Run: func(sb *strings.Builder) error {
					h.appendActivityOverviewTool(ctx, sb, userID, call.Days)
					return nil
				},
			})
		case aiToolRecentActivities:
			executions = append(executions, aiToolExecution{
				Name:    call.Name,
				Section: "активности",
				Days:    call.Days,
				Limit:   call.Limit,
				Run: func(sb *strings.Builder) error {
					h.appendRecentActivitiesTool(ctx, sb, userID, call.Days, call.Limit)
					return nil
				},
			})
		case aiToolHealthOverview:
			executions = append(executions, aiToolExecution{
				Name:    call.Name,
				Section: "здоровье",
				Days:    call.Days,
				Run: func(sb *strings.Builder) error {
					h.appendHealthOverviewTool(ctx, sb, userID, call.Days)
					return nil
				},
			})
		case aiToolWorkoutOverview:
			end := time.Now()
			start := end.AddDate(0, 0, -call.Days)
			var cached *AIWorkoutOverviewData
			load := func() (AIWorkoutOverviewData, error) {
				if cached != nil {
					return *cached, nil
				}
				data, err := h.buildWorkoutOverviewInRange(ctx, userID, start, end)
				if err != nil {
					return AIWorkoutOverviewData{}, err
				}
				cached = &data
				return data, nil
			}
			executions = append(executions, aiToolExecution{
				Name:    call.Name,
				Section: "тренировки",
				Days:    call.Days,
				Run: func(sb *strings.Builder) error {
					data, err := load()
					if err != nil {
						return err
					}
					sb.WriteString(renderWorkoutOverviewText(fmt.Sprintf("=== ТРЕНИРОВКИ (%d дней) ===", call.Days), data))
					return nil
				},
				Data: func() (any, error) {
					data, err := load()
					if err != nil {
						return nil, err
					}
					return data, nil
				},
			})
		case aiToolRecentWorkouts:
			var cached *AIRecentWorkoutsData
			load := func() (AIRecentWorkoutsData, error) {
				if cached != nil {
					return *cached, nil
				}
				data, err := h.buildRecentWorkoutsData(ctx, userID, call.Limit)
				if err != nil {
					return AIRecentWorkoutsData{}, err
				}
				cached = &data
				return data, nil
			}
			executions = append(executions, aiToolExecution{
				Name:    call.Name,
				Section: "тренировки",
				Limit:   call.Limit,
				Run: func(sb *strings.Builder) error {
					data, err := load()
					if err != nil {
						return err
					}
					sb.WriteString(renderRecentWorkoutsText(fmt.Sprintf("=== ПОСЛЕДНИЕ ТРЕНИРОВКИ (%d шт.) ===", call.Limit), data))
					return nil
				},
				Data: func() (any, error) {
					data, err := load()
					if err != nil {
						return nil, err
					}
					return data, nil
				},
			})
		case aiToolRoutineOverview:
			var cached *AIRoutineOverviewData
			load := func() (AIRoutineOverviewData, error) {
				if cached != nil {
					return *cached, nil
				}
				data, err := h.buildRoutineOverviewData(ctx, userID, call.Limit)
				if err != nil {
					return AIRoutineOverviewData{}, err
				}
				cached = &data
				return data, nil
			}
			executions = append(executions, aiToolExecution{
				Name:    call.Name,
				Section: "hevy routines",
				Limit:   call.Limit,
				Run: func(sb *strings.Builder) error {
					data, err := load()
					if err != nil {
						return err
					}
					sb.WriteString(renderRoutineOverviewText(fmt.Sprintf("=== HEVY ROUTINES (%d шт.) ===", call.Limit), data))
					return nil
				},
				Data: func() (any, error) {
					data, err := load()
					if err != nil {
						return nil, err
					}
					return data, nil
				},
			})
		case aiToolHabitOverview:
			executions = append(executions, aiToolExecution{
				Name:    call.Name,
				Section: "привычки",
				Days:    call.Days,
				Run: func(sb *strings.Builder) error {
					return h.appendHabitOverviewTool(ctx, sb, userID, call.Days)
				},
			})
		case aiToolNutritionOverview:
			var cached *AINutritionOverviewData
			load := func() (AINutritionOverviewData, error) {
				if cached != nil {
					return *cached, nil
				}
				data, err := h.buildNutritionOverviewData(ctx, userID, call.Days)
				if err != nil {
					return AINutritionOverviewData{}, err
				}
				cached = &data
				return data, nil
			}
			executions = append(executions, aiToolExecution{
				Name:    call.Name,
				Section: "питание",
				Days:    call.Days,
				Run: func(sb *strings.Builder) error {
					data, err := load()
					if err != nil {
						return err
					}
					sb.WriteString(renderNutritionOverviewText(fmt.Sprintf("=== ПИТАНИЕ (%d дней) ===", call.Days), data))
					return nil
				},
				Data: func() (any, error) {
					data, err := load()
					if err != nil {
						return nil, err
					}
					return data, nil
				},
			})
		case aiToolJournalOverview:
			executions = append(executions, aiToolExecution{
				Name:    call.Name,
				Section: "дневник",
				Days:    call.Days,
				Limit:   call.Limit,
				Run: func(sb *strings.Builder) error {
					h.appendJournalOverviewTool(ctx, sb, userID, call.Days, call.Limit)
					return nil
				},
			})
		case aiToolCalendarOverview:
			executions = append(executions, aiToolExecution{
				Name:       call.Name,
				Section:    "календарь",
				PastDays:   call.PastDays,
				FutureDays: call.FutureDays,
				Limit:      call.Limit,
				Run: func(sb *strings.Builder) error {
					h.appendCalendarOverviewTool(ctx, sb, userID, call.PastDays, call.FutureDays, call.Limit)
					return nil
				},
			})
		case aiToolWeatherOverview:
			executions = append(executions, aiToolExecution{
				Name:    call.Name,
				Section: "погода",
				Run: func(sb *strings.Builder) error {
					h.appendWeatherOverviewTool(sb)
					return nil
				},
			})
		}
	}

	return executions
}

func aiToolNames(calls []aiToolCall) []string {
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, string(call.Name))
	}
	return names
}

func (h *AIHandler) appendHealthOverviewTool(ctx context.Context, sb *strings.Builder, userID string, days int) {
	end := time.Now()
	start := end.AddDate(0, 0, -days)
	h.appendHealthContextInRange(ctx, sb, userID, start, end, fmt.Sprintf("=== ЗДОРОВЬЕ (%d дней) ===", days))
}

func (h *AIHandler) appendActivityOverviewTool(ctx context.Context, sb *strings.Builder, userID string, days int) {
	since := time.Now().AddDate(0, 0, -days)
	sb.WriteString(fmt.Sprintf("=== АКТИВНОСТИ (%d дней) ===\n", days))

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
	`, userID, since).Scan(&count, &distanceKm, &durationHours, &calories)
	sb.WriteString(fmt.Sprintf("За период: %d активностей, %.1f км, %.1f ч, %.0f ккал\n", count, distanceKm, durationHours, calories))

	rows, err := h.db.Query(ctx, `
		SELECT COALESCE(NULLIF(sport_type, ''), type), COUNT(*), COALESCE(SUM(distance_meters) / 1000.0, 0)
		FROM activities
		WHERE user_id = $1
			AND started_at >= $2
		GROUP BY 1
		ORDER BY 2 DESC, 3 DESC
		LIMIT 5
	`, userID, since)
	if err == nil {
		defer rows.Close()
		sb.WriteString("Топ типов:\n")
		hasRows := false
		for rows.Next() {
			hasRows = true
			var activityType string
			var typeCount int
			var km float64
			if rows.Scan(&activityType, &typeCount, &km) == nil {
				sb.WriteString(fmt.Sprintf("  - %s: %d раз, %.1f км\n", activityType, typeCount, km))
			}
		}
		if !hasRows {
			sb.WriteString("  - Нет активностей за период\n")
		}
	}
}

func (h *AIHandler) appendRecentActivitiesTool(ctx context.Context, sb *strings.Builder, userID string, days, limit int) {
	since := time.Now().AddDate(0, 0, -days)
	sb.WriteString(fmt.Sprintf("=== ПОСЛЕДНИЕ АКТИВНОСТИ (%d дней, %d шт.) ===\n", days, limit))

	rows, err := h.db.Query(ctx, `
		SELECT started_at, COALESCE(NULLIF(sport_type, ''), type), COALESCE(name, ''), COALESCE(distance_meters / 1000.0, 0), COALESCE(duration_seconds / 60.0, 0)
		FROM activities
		WHERE user_id = $1
			AND started_at >= $2
		ORDER BY started_at DESC
		LIMIT $3
	`, userID, since, limit)
	if err != nil {
		sb.WriteString("Данные по активностям временно недоступны.\n")
		return
	}
	defer rows.Close()

	hasRows := false
	for rows.Next() {
		hasRows = true
		var startedAt time.Time
		var activityType, name string
		var km, minutes float64
		if rows.Scan(&startedAt, &activityType, &name, &km, &minutes) == nil {
			sb.WriteString(fmt.Sprintf("  %s %s: %s %.1f км %.0f мин\n", startedAt.Format("02.01"), activityType, name, km, minutes))
		}
	}
	if !hasRows {
		sb.WriteString("  Нет активностей за период\n")
	}
}

func (h *AIHandler) appendWorkoutOverviewTool(ctx context.Context, sb *strings.Builder, userID string, days int) {
	end := time.Now()
	start := end.AddDate(0, 0, -days)
	data, err := h.buildWorkoutOverviewInRange(ctx, userID, start, end)
	if err != nil {
		sb.WriteString(fmt.Sprintf("=== ТРЕНИРОВКИ (%d дней) ===\nДанные по тренировкам временно недоступны.\n", days))
		return
	}
	sb.WriteString(renderWorkoutOverviewText(fmt.Sprintf("=== ТРЕНИРОВКИ (%d дней) ===", days), data))
}

func (h *AIHandler) appendRecentWorkoutsTool(ctx context.Context, sb *strings.Builder, userID string, limit int) error {
	data, err := h.buildRecentWorkoutsData(ctx, userID, limit)
	if err != nil {
		return err
	}
	sb.WriteString(renderRecentWorkoutsText(fmt.Sprintf("=== ПОСЛЕДНИЕ ТРЕНИРОВКИ (%d шт.) ===", limit), data))
	return nil
}

func (h *AIHandler) appendRoutineOverviewTool(ctx context.Context, sb *strings.Builder, userID string, limit int) error {
	data, err := h.buildRoutineOverviewData(ctx, userID, limit)
	if err != nil {
		return err
	}
	sb.WriteString(renderRoutineOverviewText(fmt.Sprintf("=== HEVY ROUTINES (%d шт.) ===", limit), data))
	return nil
}

func (h *AIHandler) appendHabitOverviewTool(ctx context.Context, sb *strings.Builder, userID string, days int) error {
	habitContext, err := h.buildHabitContext(ctx, userID, days)
	if err != nil {
		return err
	}
	if strings.TrimSpace(habitContext) == "" {
		sb.WriteString(fmt.Sprintf("=== ПРИВЫЧКИ (%d дней) ===\nНет данных Habitify.\n", days))
		return nil
	}
	sb.WriteString(habitContext)
	sb.WriteString("\n")
	return nil
}

func (h *AIHandler) appendNutritionOverviewTool(ctx context.Context, sb *strings.Builder, userID string, days int) {
	data, err := h.buildNutritionOverviewData(ctx, userID, days)
	if err != nil {
		sb.WriteString(fmt.Sprintf("=== ПИТАНИЕ (%d дней) ===\nДанные питания временно недоступны.\n", days))
		return
	}
	sb.WriteString(renderNutritionOverviewText(fmt.Sprintf("=== ПИТАНИЕ (%d дней) ===", days), data))
}

func (h *AIHandler) appendJournalOverviewTool(ctx context.Context, sb *strings.Builder, userID string, days, limit int) {
	since := time.Now().AddDate(0, 0, -days)
	sb.WriteString(fmt.Sprintf("=== ДНЕВНИК (%d дней, %d записей) ===\n", days, limit))

	rows, err := h.db.Query(ctx, `
		SELECT date, title, content, tags, mood
		FROM journal_entries
		WHERE user_id = $1
			AND date >= $2
		ORDER BY date DESC
		LIMIT $3
	`, userID, since, limit)
	if err != nil {
		sb.WriteString("Данные дневника временно недоступны.\n")
		return
	}
	defer rows.Close()

	hasRows := false
	for rows.Next() {
		hasRows = true
		var date time.Time
		var title, content string
		var tags []string
		var mood *int
		if rows.Scan(&date, &title, &content, &tags, &mood) == nil {
			entry := fmt.Sprintf("  %s: %s", date.Format("02.01"), title)
			if mood != nil {
				entry += fmt.Sprintf(" (настроение %d/10)", *mood)
			}
			if len(tags) > 0 {
				entry += " [" + strings.Join(tags, ", ") + "]"
			}
			sb.WriteString(entry + "\n")
			if strings.TrimSpace(content) != "" {
				sb.WriteString("    " + truncateAIText(content, 180) + "\n")
			}
		}
	}
	if !hasRows {
		sb.WriteString("  Нет записей за период\n")
	}
}

func (h *AIHandler) appendCalendarOverviewTool(ctx context.Context, sb *strings.Builder, userID string, pastDays, futureDays, limit int) {
	since := time.Now().AddDate(0, 0, -pastDays)
	until := time.Now().AddDate(0, 0, futureDays)
	sb.WriteString(fmt.Sprintf("=== КАЛЕНДАРЬ (-%d / +%d дней; это план, не факт) ===\n", pastDays, futureDays))
	sb.WriteString("События календаря отражают расписание из Google Calendar и сами по себе не подтверждают, что действие реально произошло.\n")

	rows, err := h.db.Query(ctx, `
		SELECT title, start_time, end_time, all_day, COALESCE(location, '')
		FROM calendar_events
		WHERE user_id = $1
			AND start_time >= $2
			AND start_time <= $3
		ORDER BY start_time
		LIMIT $4
	`, userID, since, until, limit)
	if err != nil {
		sb.WriteString("Данные календаря временно недоступны.\n")
		return
	}
	defer rows.Close()

	hasRows := false
	for rows.Next() {
		hasRows = true
		var title, location string
		var startTime, endTime time.Time
		var allDay bool
		if rows.Scan(&title, &startTime, &endTime, &allDay, &location) == nil {
			sb.WriteString(formatAICalendarEvent(startTime, endTime, allDay, title, location) + "\n")
		}
	}
	if !hasRows {
		sb.WriteString("  Нет событий в выбранном окне\n")
	}
}

func (h *AIHandler) appendWeatherOverviewTool(sb *strings.Builder) {
	sb.WriteString("=== ПОГОДА ===\n")
	if h.weather == nil {
		sb.WriteString("Погодный источник не подключён.\n")
		return
	}
	wd, err := h.weather.Fetch(0, 0, "")
	if err != nil {
		sb.WriteString("Погодные данные временно недоступны.\n")
		return
	}

	sb.WriteString(fmt.Sprintf("Город: %s\n", wd.City))
	sb.WriteString(fmt.Sprintf("Сейчас: %.1f°C, ощущается %.1f°C, %s\n", wd.Temp, wd.FeelsLike, wd.Description))
	sb.WriteString(fmt.Sprintf("Влажность: %d%%, ветер %.1f км/ч\n", wd.Humidity, wd.WindSpeed))
	for i, d := range wd.Daily {
		if i >= 3 {
			break
		}
		sb.WriteString(fmt.Sprintf("  %s: %s, макс %.0f°C, мин %.0f°C\n", d.Date, wmoDescription(d.WeatherCode), d.TempMax, d.TempMin))
	}
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func formatAILocation(location string) string {
	if location == "" {
		return ""
	}
	return " @ " + location
}
