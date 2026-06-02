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
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Type         string  `json:"type"`
	Currency     string  `json:"currency"`
	Balance      float64 `json:"balance"`
	InBalance    bool    `json:"in_balance"`
	Archived     bool    `json:"archived"`
	CompanyID    *int    `json:"company_id"`
	CompanyTitle *string `json:"company_title"`
}

type FinanceTransaction struct {
	ID           string    `json:"id"`
	OccurredAt   time.Time `json:"occurred_at"`
	Amount       float64   `json:"amount"`
	Currency     string    `json:"currency"`
	Comment      string    `json:"comment"`
	Payee        *string   `json:"payee"`
	Category     *string   `json:"category"`
	Subcategory  *string   `json:"subcategory"`
	AccountTitle *string   `json:"account_title"`
	Tags         []string  `json:"tags"`
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
			TO_CHAR(DATE_TRUNC('month', t.occurred_at), 'YYYY-MM') as month,
			COALESCE(SUM(CASE WHEN t.amount < 0 THEN ABS(t.amount) ELSE 0 END), 0) as spending,
			COALESCE(SUM(CASE WHEN t.amount > 0 THEN t.amount ELSE 0 END), 0) as income
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.currency = 'RUB'
			AND t.is_transfer = false
			AND t.occurred_at >= DATE_TRUNC('month', NOW()) - INTERVAL '5 months'
			AND t.user_id = $1
			AND COALESCE(a.in_balance, TRUE) = TRUE
		GROUP BY DATE_TRUNC('month', t.occurred_at)
		ORDER BY DATE_TRUNC('month', t.occurred_at) ASC
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
		SELECT id, title, COALESCE(type, ''), currency, COALESCE(balance, 0), in_balance, archived, company_id, company_title
		FROM accounts
		WHERE user_id = $1
		  AND COALESCE(archived, FALSE) = FALSE
		ORDER BY in_balance DESC, ABS(COALESCE(balance, 0)) DESC, title ASC
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
		if err := rows.Scan(&a.ID, &a.Title, &a.Type, &a.Currency, &a.Balance, &a.InBalance, &a.Archived, &a.CompanyID, &a.CompanyTitle); err != nil {
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
	to := q.Get("to")
	categoryType := q.Get("type")
	var monthStart time.Time
	if from != "" {
		monthStart, _ = time.Parse("2006-01-02", from)
	}
	if monthStart.IsZero() {
		monthStart = time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local)
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}

	amountCondition := "t.amount < 0"
	amountExpression := "ABS(t.amount)"
	if categoryType == "income" {
		amountCondition = "t.amount > 0"
		amountExpression = "t.amount"
	}

	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT COALESCE(NULLIF(TRIM(t.category), ''), 'Без категории') as category, SUM(%s) as total
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE %s
		  AND t.is_transfer = false
		  AND t.currency = 'RUB'
		  AND t.occurred_at >= $1
		  AND t.occurred_at < ($2::date + INTERVAL '1 day')
		  AND t.user_id = $3
		  AND COALESCE(a.in_balance, TRUE) = TRUE
		GROUP BY category
		ORDER BY total DESC
		LIMIT 15
	`, amountExpression, amountCondition), monthStart, to, userID)
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
	payee := q.Get("payee")
	search := q.Get("search")
	from := q.Get("from")
	to := q.Get("to")
	sortBy := q.Get("sort")
	order := q.Get("order")
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("page_size"))
	if limit <= 0 {
		limit = 30
	}
	if limit > 250 {
		limit = 250
	}
	offset := (page - 1) * limit

	conditions := "t.is_transfer = false AND t.user_id = $1 AND COALESCE(a.in_balance, TRUE) = TRUE"
	args := []any{userID}
	argN := 2

	switch filter {
	case "income":
		conditions += " AND t.amount > 0"
	case "expense":
		conditions += " AND t.amount < 0"
	}

	if category != "" {
		if category == "__uncategorized__" || category == "Без категории" {
			conditions += " AND NULLIF(TRIM(t.category), '') IS NULL"
		} else {
			conditions += fmt.Sprintf(" AND t.category = $%d", argN)
			args = append(args, category)
			argN++
		}
	}

	if payee != "" {
		conditions += fmt.Sprintf(" AND COALESCE(NULLIF(t.payee,''), NULLIF(t.comment,''), 'Без описания') = $%d", argN)
		args = append(args, payee)
		argN++
	}

	if search != "" {
		conditions += fmt.Sprintf(" AND (COALESCE(t.comment,'') ILIKE $%d OR COALESCE(t.payee,'') ILIKE $%d OR COALESCE(t.category,'') ILIKE $%d OR COALESCE(a.title,'') ILIKE $%d)", argN, argN, argN, argN)
		args = append(args, "%"+search+"%")
		argN++
	}

	if from != "" {
		conditions += fmt.Sprintf(" AND t.occurred_at >= $%d", argN)
		args = append(args, from)
		argN++
	}
	if to != "" {
		conditions += fmt.Sprintf(" AND t.occurred_at < ($%d::date + INTERVAL '1 day')", argN)
		args = append(args, to)
		argN++
	}

	orderBy := "t.occurred_at DESC"
	direction := "DESC"
	if order == "asc" {
		direction = "ASC"
	}
	switch sortBy {
	case "amount":
		orderBy = "ABS(t.amount) " + direction
	case "date_asc":
		orderBy = "t.occurred_at ASC"
	case "amount_asc":
		orderBy = "ABS(t.amount) ASC"
	case "date", "occurred_at":
		orderBy = "t.occurred_at " + direction
	case "category":
		orderBy = "t.category " + direction + ", t.occurred_at DESC"
	case "payee":
		orderBy = "t.payee " + direction + ", t.occurred_at DESC"
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(`
		SELECT t.id, t.occurred_at, t.amount, t.currency, COALESCE(t.comment, ''),
			t.payee, t.category, t.subcategory, a.title, COALESCE(t.tags, ARRAY[]::text[])
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
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
		if err := rows.Scan(&tx.ID, &tx.OccurredAt, &tx.Amount, &tx.Currency, &tx.Comment, &tx.Payee, &tx.Category, &tx.Subcategory, &tx.AccountTitle, &tx.Tags); err != nil {
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
		SELECT TO_CHAR(t.occurred_at::date, 'YYYY-MM-DD'),
			COALESCE(SUM(CASE WHEN t.amount < 0 THEN ABS(t.amount) ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN t.amount > 0 THEN t.amount ELSE 0 END), 0)
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.is_transfer = false
			AND t.currency = 'RUB'
			AND t.occurred_at >= $1
			AND t.occurred_at < ($2::date + INTERVAL '1 day')
			AND t.user_id = $3
			AND COALESCE(a.in_balance, TRUE) = TRUE
		GROUP BY t.occurred_at::date
		ORDER BY t.occurred_at::date
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
		SELECT COALESCE(NULLIF(t.payee,''), NULLIF(t.comment,''), 'Без описания'),
			SUM(ABS(t.amount)), COUNT(*)
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.amount < 0
			AND t.is_transfer = false
			AND t.currency = 'RUB'
			AND t.occurred_at >= $1
			AND t.occurred_at < ($2::date + INTERVAL '1 day')
			AND t.user_id = $3
			AND COALESCE(a.in_balance, TRUE) = TRUE
		GROUP BY COALESCE(NULLIF(t.payee,''), NULLIF(t.comment,''), 'Без описания')
		ORDER BY SUM(ABS(t.amount)) DESC
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
		SELECT DISTINCT COALESCE(NULLIF(TRIM(t.category), ''), 'Без категории')
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.user_id = $1
			AND t.is_transfer = false
			AND COALESCE(a.in_balance, TRUE) = TRUE
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
