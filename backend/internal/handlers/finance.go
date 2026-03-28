package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type FinanceHandler struct {
	db     *pgxpool.Pool
	logger zerolog.Logger
}

func NewFinance(db *pgxpool.Pool, logger zerolog.Logger) *FinanceHandler {
	return &FinanceHandler{db: db, logger: logger.With().Str("handler", "finance").Logger()}
}

type CategoryStat struct {
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
}

type MonthStat struct {
	Month    string  `json:"month"`
	Spending float64 `json:"spending"`
	Income   float64 `json:"income"`
}

type Account struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Type     string  `json:"type"`
	Currency string  `json:"currency"`
	Balance  float64 `json:"balance"`
}

type FinanceTransaction struct {
	ID         string    `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
	Amount     float64   `json:"amount"`
	Currency   string    `json:"currency"`
	Comment    string    `json:"comment"`
	Payee      *string   `json:"payee"`
}

func (h *FinanceHandler) GetMonthly(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := h.db.Query(ctx, `
		SELECT
			TO_CHAR(DATE_TRUNC('month', occurred_at), 'YYYY-MM') as month,
			COALESCE(SUM(CASE WHEN amount < 0 THEN ABS(amount) ELSE 0 END), 0) as spending,
			COALESCE(SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END), 0) as income
		FROM transactions
		WHERE currency = 'RUB' AND is_transfer = false
			AND occurred_at >= DATE_TRUNC('month', NOW()) - INTERVAL '5 months'
		GROUP BY DATE_TRUNC('month', occurred_at)
		ORDER BY DATE_TRUNC('month', occurred_at) ASC
	`)
	if err != nil {
		h.logger.Error().Err(err).Msg("query monthly")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	stats := make([]MonthStat, 0)
	for rows.Next() {
		var s MonthStat
		if err := rows.Scan(&s.Month, &s.Spending, &s.Income); err != nil {
			continue
		}
		stats = append(stats, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *FinanceHandler) GetAccounts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := h.db.Query(ctx, `
		SELECT id, title, COALESCE(type, ''), currency, COALESCE(balance, 0)
		FROM accounts
		WHERE balance != 0
		ORDER BY currency, balance DESC
	`)
	if err != nil {
		h.logger.Error().Err(err).Msg("query accounts")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	accounts := make([]Account, 0)
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Title, &a.Type, &a.Currency, &a.Balance); err != nil {
			continue
		}
		accounts = append(accounts, a)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(accounts)
}

func (h *FinanceHandler) GetSpendingByCategory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	monthStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local)

	rows, err := h.db.Query(ctx, `
		SELECT COALESCE(category, 'Без категории') as category, SUM(ABS(amount)) as total
		FROM transactions
		WHERE amount < 0
		  AND is_transfer = false
		  AND currency = 'RUB'
		  AND occurred_at >= $1
		GROUP BY category
		ORDER BY total DESC
		LIMIT 15
	`, monthStart)
	if err != nil {
		h.logger.Error().Err(err).Msg("query categories")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	stats := make([]CategoryStat, 0)
	for rows.Next() {
		var s CategoryStat
		if err := rows.Scan(&s.Category, &s.Amount); err != nil {
			continue
		}
		stats = append(stats, s)
	}

	h.logger.Debug().Int("categories", len(stats)).Msg("spending by category")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *FinanceHandler) GetTransactions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	q := r.URL.Query()
	filter := q.Get("type") // "income", "expense", ""=all
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 30
	offset := (page - 1) * limit

	var condition string
	switch filter {
	case "income":
		condition = "AND amount > 0"
	case "expense":
		condition = "AND amount < 0"
	default:
		condition = ""
	}

	rows, err := h.db.Query(ctx, `
		SELECT id, occurred_at, amount, currency, COALESCE(comment, ''), payee
		FROM transactions
		WHERE is_transfer = false `+condition+`
		ORDER BY occurred_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		h.logger.Error().Err(err).Msg("query transactions")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	txs := make([]FinanceTransaction, 0)
	for rows.Next() {
		var tx FinanceTransaction
		if err := rows.Scan(&tx.ID, &tx.OccurredAt, &tx.Amount, &tx.Currency, &tx.Comment, &tx.Payee); err != nil {
			continue
		}
		txs = append(txs, tx)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(txs)
}
