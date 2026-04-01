package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	authmw "life-dashboard/internal/middleware"
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
	Category   *string   `json:"category"`
}

type DailyTotal struct {
	Date     string  `json:"date"`
	Spending float64 `json:"spending"`
	Income   float64 `json:"income"`
}

type TopExpense struct {
	Payee  string  `json:"payee"`
	Amount float64 `json:"amount"`
	Count  int     `json:"count"`
}

func (h *FinanceHandler) GetMonthly(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)

	rows, err := h.db.Query(ctx, `
		SELECT
			TO_CHAR(DATE_TRUNC('month', occurred_at), 'YYYY-MM') as month,
			COALESCE(SUM(CASE WHEN amount < 0 THEN ABS(amount) ELSE 0 END), 0) as spending,
			COALESCE(SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END), 0) as income
		FROM transactions
		WHERE currency = 'RUB' AND is_transfer = false
			AND occurred_at >= DATE_TRUNC('month', NOW()) - INTERVAL '5 months'
			AND user_id = $1
		GROUP BY DATE_TRUNC('month', occurred_at)
		ORDER BY DATE_TRUNC('month', occurred_at) ASC
	`, userID)
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
	userID := r.Context().Value(authmw.UserIDKey).(string)

	rows, err := h.db.Query(ctx, `
		SELECT id, title, COALESCE(type, ''), currency, COALESCE(balance, 0)
		FROM accounts
		WHERE balance != 0 AND user_id = $1
		ORDER BY currency, balance DESC
	`, userID)
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
	userID := r.Context().Value(authmw.UserIDKey).(string)
	q := r.URL.Query()
	from := q.Get("from")
	var monthStart time.Time
	if from != "" {
		monthStart, _ = time.Parse("2006-01-02", from)
	}
	if monthStart.IsZero() {
		monthStart = time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local)
	}

	rows, err := h.db.Query(ctx, `
		SELECT COALESCE(category, 'Без категории') as category, SUM(ABS(amount)) as total
		FROM transactions
		WHERE amount < 0
		  AND is_transfer = false
		  AND currency = 'RUB'
		  AND occurred_at >= $1
		  AND user_id = $2
		GROUP BY category
		ORDER BY total DESC
		LIMIT 15
	`, monthStart, userID)
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
	userID := r.Context().Value(authmw.UserIDKey).(string)

	q := r.URL.Query()
	filter := q.Get("type")
	category := q.Get("category")
	search := q.Get("search")
	from := q.Get("from")
	to := q.Get("to")
	sortBy := q.Get("sort")
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 30
	offset := (page - 1) * limit

	conditions := "is_transfer = false AND user_id = $1"
	args := []any{userID}
	argN := 2

	switch filter {
	case "income":
		conditions += " AND amount > 0"
	case "expense":
		conditions += " AND amount < 0"
	}

	if category != "" {
		conditions += fmt.Sprintf(" AND category = $%d", argN)
		args = append(args, category)
		argN++
	}

	if search != "" {
		conditions += fmt.Sprintf(" AND (COALESCE(comment,'') ILIKE $%d OR COALESCE(payee,'') ILIKE $%d)", argN, argN)
		args = append(args, "%"+search+"%")
		argN++
	}

	if from != "" {
		conditions += fmt.Sprintf(" AND occurred_at >= $%d", argN)
		args = append(args, from)
		argN++
	}
	if to != "" {
		conditions += fmt.Sprintf(" AND occurred_at <= $%d", argN)
		args = append(args, to)
		argN++
	}

	orderBy := "occurred_at DESC"
	switch sortBy {
	case "amount":
		orderBy = "ABS(amount) DESC"
	case "amount_asc":
		orderBy = "ABS(amount) ASC"
	case "date_asc":
		orderBy = "occurred_at ASC"
	case "category":
		orderBy = "category, occurred_at DESC"
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(`
		SELECT id, occurred_at, amount, currency, COALESCE(comment, ''), payee, category
		FROM transactions
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, conditions, orderBy, argN, argN+1)

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		h.logger.Error().Err(err).Msg("query transactions")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	txs := make([]FinanceTransaction, 0)
	for rows.Next() {
		var tx FinanceTransaction
		if err := rows.Scan(&tx.ID, &tx.OccurredAt, &tx.Amount, &tx.Currency, &tx.Comment, &tx.Payee, &tx.Category); err != nil {
			continue
		}
		txs = append(txs, tx)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(txs)
}

// GET /api/v1/finance/daily — daily spending/income totals
func (h *FinanceHandler) GetDailyTotals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	q := r.URL.Query()
	from := q.Get("from")
	if from == "" {
		from = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	}
	to := q.Get("to")
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}

	rows, err := h.db.Query(ctx, `
		SELECT TO_CHAR(occurred_at::date, 'YYYY-MM-DD'),
			COALESCE(SUM(CASE WHEN amount < 0 THEN ABS(amount) ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END), 0)
		FROM transactions
		WHERE is_transfer = false AND currency = 'RUB'
			AND occurred_at >= $1 AND occurred_at <= $2
			AND user_id = $3
		GROUP BY occurred_at::date
		ORDER BY occurred_at::date
	`, from, to, userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	stats := make([]DailyTotal, 0)
	for rows.Next() {
		var s DailyTotal
		rows.Scan(&s.Date, &s.Spending, &s.Income)
		stats = append(stats, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GET /api/v1/finance/top-expenses
func (h *FinanceHandler) GetTopExpenses(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	q := r.URL.Query()
	from := q.Get("from")
	if from == "" {
		from = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	}
	to := q.Get("to")
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}

	rows, err := h.db.Query(ctx, `
		SELECT COALESCE(NULLIF(payee,''), NULLIF(comment,''), 'Без описания'),
			SUM(ABS(amount)), COUNT(*)
		FROM transactions
		WHERE amount < 0 AND is_transfer = false AND currency = 'RUB'
			AND occurred_at >= $1 AND occurred_at <= $2
			AND user_id = $3
		GROUP BY COALESCE(NULLIF(payee,''), NULLIF(comment,''), 'Без описания')
		ORDER BY SUM(ABS(amount)) DESC
		LIMIT 10
	`, from, to, userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	expenses := make([]TopExpense, 0)
	for rows.Next() {
		var e TopExpense
		rows.Scan(&e.Payee, &e.Amount, &e.Count)
		expenses = append(expenses, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(expenses)
}

// GET /api/v1/finance/category-list — unique categories for filter
func (h *FinanceHandler) GetCategoryList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)

	rows, err := h.db.Query(ctx, `
		SELECT DISTINCT COALESCE(category, 'Без категории')
		FROM transactions WHERE user_id = $1 AND is_transfer = false
		ORDER BY 1
	`, userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	cats := make([]string, 0)
	for rows.Next() {
		var c string
		rows.Scan(&c)
		cats = append(cats, c)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cats)
}
