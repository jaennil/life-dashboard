package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	authmw "life-dashboard/internal/middleware"
)

type DashboardHandler struct {
	db     *pgxpool.Pool
	logger zerolog.Logger
}

func NewDashboard(db *pgxpool.Pool, logger zerolog.Logger) *DashboardHandler {
	return &DashboardHandler{db: db, logger: logger.With().Str("handler", "dashboard").Logger()}
}

type DashboardSummary struct {
	Finance      FinanceSummary               `json:"finance"`
	Fitness      FitnessSummary               `json:"fitness"`
	Nutrition    DashboardNutritionSummary    `json:"nutrition"`
	Productivity DashboardProductivitySummary `json:"productivity"`
	Checkup      LatestCheckupResponse        `json:"checkup"`
}

type FinanceSummary struct {
	TotalBalance    float64 `json:"total_balance"`
	Currency        string  `json:"currency"`
	MonthlySpending float64 `json:"monthly_spending"`
	MonthlyIncome   float64 `json:"monthly_income"`
}

type FitnessSummary struct {
	ActivitiesThisWeek int     `json:"activities_this_week"`
	WorkoutsThisWeek   int     `json:"workouts_this_week"`
	TotalDistanceKm    float64 `json:"total_distance_km"`
}

type DashboardNutritionSummary struct {
	TodayKcal      float64  `json:"today_kcal"`
	AvgCalories    float64  `json:"avg_calories"`
	DaysTracked    int      `json:"days_tracked"`
	TargetCalories *float64 `json:"target_calories,omitempty"`
}

type DashboardProductivitySummary struct {
	ActiveTotal          int `json:"active_total"`
	OverdueTotal         int `json:"overdue_total"`
	DueTodayTotal        int `json:"due_today_total"`
	CompletedTodayTotal  int `json:"completed_today_total"`
	HabitsTotal          int `json:"habits_total"`
	HabitsCompletedToday int `json:"habits_completed_today"`
	HabitsPendingToday   int `json:"habits_pending_today"`
}

func (h *DashboardHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)

	var summary DashboardSummary
	summary.Finance.Currency = "RUB"
	summary.Checkup.HasReport = false

	// Total balance (RUB accounts only)
	_ = h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(balance), 0)
		FROM accounts
		WHERE currency = 'RUB' AND in_balance = TRUE AND user_id = $1
	`, userID).Scan(&summary.Finance.TotalBalance)

	// Monthly spending / income
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	_ = h.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN amount < 0 THEN ABS(amount) ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END), 0)
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.currency = 'RUB'
			AND t.occurred_at >= $1
			AND t.is_transfer = false
			AND t.user_id = $2
			AND COALESCE(a.in_balance, TRUE) = TRUE
	`, monthStart, userID).Scan(&summary.Finance.MonthlySpending, &summary.Finance.MonthlyIncome)

	// Activities this week
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	_ = h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM activities WHERE started_at >= $1 AND user_id = $2
	`, weekStart, userID).Scan(&summary.Fitness.ActivitiesThisWeek)

	// Total distance this week (km)
	_ = h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(distance_meters) / 1000.0, 0)
		FROM activities WHERE started_at >= $1 AND user_id = $2
	`, weekStart, userID).Scan(&summary.Fitness.TotalDistanceKm)

	// Workouts this week
	_ = h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM workouts WHERE started_at >= $1 AND user_id = $2
	`, weekStart, userID).Scan(&summary.Fitness.WorkoutsThisWeek)

	// Nutrition overview (last 7 tracked days + today)
	weekNutritionStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -6)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	_ = h.db.QueryRow(ctx, `
		SELECT
			COALESCE(AVG(calories_total), 0),
			COUNT(*)
		FROM nutrition_daily
		WHERE date >= $1 AND user_id = $2
	`, weekNutritionStart, userID).Scan(&summary.Nutrition.AvgCalories, &summary.Nutrition.DaysTracked)
	_ = h.db.QueryRow(ctx, `
		SELECT COALESCE(calories_total, 0)
		FROM nutrition_daily
		WHERE date = $1 AND user_id = $2
	`, todayStart, userID).Scan(&summary.Nutrition.TodayKcal)
	if targets, err := loadNutritionTargets(ctx, h.db, userID); err == nil && targets != nil {
		summary.Nutrition.TargetCalories = targets.TargetCalories
	}

	// Todoist task pulse
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	_ = h.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE is_active = TRUE),
			COUNT(*) FILTER (
				WHERE is_active = TRUE
					AND (
						(due_at IS NOT NULL AND due_at < $2)
						OR (due_at IS NULL AND due_date IS NOT NULL AND due_date < $3::date)
					)
			),
			COUNT(*) FILTER (
				WHERE is_active = TRUE
					AND (
						(due_at IS NOT NULL AND due_at >= $2 AND due_at < $3)
						OR (due_at IS NULL AND due_date = $2::date)
					)
			)
		FROM todoist_tasks
		WHERE user_id = $1
	`, userID, todayStart, tomorrowStart).Scan(
		&summary.Productivity.ActiveTotal,
		&summary.Productivity.OverdueTotal,
		&summary.Productivity.DueTodayTotal,
	)
	_ = h.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM todoist_task_completions
		WHERE user_id = $1
			AND completed_at >= $2
			AND completed_at < $3
	`, userID, todayStart, tomorrowStart).Scan(&summary.Productivity.CompletedTodayTotal)

	// Local routines pulse
	_ = h.db.QueryRow(ctx, `
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE COALESCE(s.status, 'none') = 'completed') AS completed_today
		FROM habits h
		LEFT JOIN habit_daily_statuses s
			ON s.habit_id = h.id
			AND s.target_date = $2::date
		WHERE h.user_id = $1
			AND h.source = 'manual'
			AND h.archived = FALSE
	`, userID, todayStart).Scan(
		&summary.Productivity.HabitsTotal,
		&summary.Productivity.HabitsCompletedToday,
	)
	summary.Productivity.HabitsPendingToday = summary.Productivity.HabitsTotal - summary.Productivity.HabitsCompletedToday

	// Latest checkup
	var requestedPeriod string
	var checkupCreatedAt time.Time
	err := h.db.QueryRow(ctx, `
		SELECT requested_period, created_at
		FROM ai_checkup_reports
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, userID).Scan(&requestedPeriod, &checkupCreatedAt)
	if err == nil {
		summary.Checkup.HasReport = true
		summary.Checkup.Period = requestedPeriod
		summary.Checkup.PeriodLabel = checkupPeriodLabel(requestedPeriod)
		summary.Checkup.GeneratedAt = &checkupCreatedAt
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

type Transaction struct {
	ID         string    `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
	Amount     float64   `json:"amount"`
	Currency   string    `json:"currency"`
	Comment    string    `json:"comment"`
	Payee      *string   `json:"payee"`
	IsTransfer bool      `json:"is_transfer"`
}

func (h *DashboardHandler) GetRecentTransactions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)

	rows, err := h.db.Query(ctx, `
		SELECT t.id, t.occurred_at, t.amount, t.currency, COALESCE(t.comment, ''), t.payee, t.is_transfer
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.is_transfer = false
			AND t.user_id = $1
			AND COALESCE(a.in_balance, TRUE) = TRUE
		ORDER BY t.occurred_at DESC
		LIMIT 10
	`, userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("query transactions")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	txs := make([]Transaction, 0)
	for rows.Next() {
		var tx Transaction
		if err := rows.Scan(&tx.ID, &tx.OccurredAt, &tx.Amount, &tx.Currency, &tx.Comment, &tx.Payee, &tx.IsTransfer); err != nil {
			continue
		}
		txs = append(txs, tx)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(txs)
}
