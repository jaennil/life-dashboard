package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type DashboardHandler struct {
	db     *pgxpool.Pool
	logger zerolog.Logger
}

func NewDashboard(db *pgxpool.Pool, logger zerolog.Logger) *DashboardHandler {
	return &DashboardHandler{db: db, logger: logger.With().Str("handler", "dashboard").Logger()}
}

type DashboardSummary struct {
	Finance  FinanceSummary  `json:"finance"`
	Fitness  FitnessSummary  `json:"fitness"`
}

type FinanceSummary struct {
	TotalBalance     float64 `json:"total_balance"`
	Currency         string  `json:"currency"`
	MonthlySpending  float64 `json:"monthly_spending"`
	MonthlyIncome    float64 `json:"monthly_income"`
}

type FitnessSummary struct {
	ActivitiesThisWeek int     `json:"activities_this_week"`
	WorkoutsThisWeek   int     `json:"workouts_this_week"`
	TotalDistanceKm    float64 `json:"total_distance_km"`
}

func (h *DashboardHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var summary DashboardSummary
	summary.Finance.Currency = "RUB"

	// Total balance (RUB accounts only)
	_ = h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(balance), 0)
		FROM accounts
		WHERE currency = 'RUB' AND balance > 0
	`).Scan(&summary.Finance.TotalBalance)

	// Monthly spending / income
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	_ = h.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN amount < 0 THEN ABS(amount) ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END), 0)
		FROM transactions
		WHERE currency = 'RUB' AND occurred_at >= $1 AND is_transfer = false
	`, monthStart).Scan(&summary.Finance.MonthlySpending, &summary.Finance.MonthlyIncome)

	// Activities this week
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	_ = h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM activities WHERE started_at >= $1
	`, weekStart).Scan(&summary.Fitness.ActivitiesThisWeek)

	// Total distance this week (km)
	_ = h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(distance_meters) / 1000.0, 0)
		FROM activities WHERE started_at >= $1
	`, weekStart).Scan(&summary.Fitness.TotalDistanceKm)

	// Workouts this week
	_ = h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM workouts WHERE started_at >= $1
	`, weekStart).Scan(&summary.Fitness.WorkoutsThisWeek)

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

	rows, err := h.db.Query(ctx, `
		SELECT id, occurred_at, amount, currency, COALESCE(comment, ''), payee, is_transfer
		FROM transactions
		WHERE is_transfer = false
		ORDER BY occurred_at DESC
		LIMIT 10
	`)
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
