package handlers

import (
	"fmt"
	"net/url"
	"strconv"
)

const (
	defaultFinanceTransactionPageSize = 30
	maxFinanceTransactionPageSize     = 250
)

type financeTransactionQueryParams struct {
	Type     string
	Category string
	Payee    string
	Search   string
	From     string
	To       string
	Sort     string
	Order    string
	Page     int
	PageSize int
}

type financeTransactionQuery struct {
	SQL  string
	Args []any
}

func parseFinanceTransactionQuery(values url.Values) financeTransactionQueryParams {
	page, _ := strconv.Atoi(values.Get("page"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(values.Get("page_size"))
	if pageSize <= 0 {
		pageSize = defaultFinanceTransactionPageSize
	}
	if pageSize > maxFinanceTransactionPageSize {
		pageSize = maxFinanceTransactionPageSize
	}

	return financeTransactionQueryParams{
		Type:     values.Get("type"),
		Category: values.Get("category"),
		Payee:    values.Get("payee"),
		Search:   values.Get("search"),
		From:     values.Get("from"),
		To:       values.Get("to"),
		Sort:     values.Get("sort"),
		Order:    values.Get("order"),
		Page:     page,
		PageSize: pageSize,
	}
}

func buildFinanceTransactionQuery(userID string, params financeTransactionQueryParams) financeTransactionQuery {
	conditions := "t.is_transfer = false AND t.user_id = $1 AND COALESCE(a.in_balance, TRUE) = TRUE"
	args := []any{userID}
	argN := 2

	switch params.Type {
	case "income":
		conditions += " AND t.amount > 0"
	case "expense":
		conditions += " AND t.amount < 0"
	}

	if params.Category != "" {
		if params.Category == "__uncategorized__" || params.Category == "Без категории" {
			conditions += " AND NULLIF(TRIM(t.category), '') IS NULL"
		} else {
			conditions += fmt.Sprintf(" AND t.category = $%d", argN)
			args = append(args, params.Category)
			argN++
		}
	}

	if params.Payee != "" {
		conditions += fmt.Sprintf(" AND COALESCE(NULLIF(t.payee,''), NULLIF(t.comment,''), 'Без описания') = $%d", argN)
		args = append(args, params.Payee)
		argN++
	}

	if params.Search != "" {
		conditions += fmt.Sprintf(" AND (COALESCE(t.comment,'') ILIKE $%d OR COALESCE(t.payee,'') ILIKE $%d OR COALESCE(t.category,'') ILIKE $%d OR COALESCE(a.title,'') ILIKE $%d)", argN, argN, argN, argN)
		args = append(args, "%"+params.Search+"%")
		argN++
	}

	if params.From != "" {
		conditions += fmt.Sprintf(" AND t.occurred_at >= $%d", argN)
		args = append(args, params.From)
		argN++
	}
	if params.To != "" {
		conditions += fmt.Sprintf(" AND t.occurred_at < ($%d::date + INTERVAL '1 day')", argN)
		args = append(args, params.To)
		argN++
	}

	orderBy := financeTransactionOrderBy(params)
	offset := (params.Page - 1) * params.PageSize
	args = append(args, params.PageSize, offset)

	return financeTransactionQuery{
		SQL: fmt.Sprintf(`
		SELECT t.id, t.occurred_at, t.amount, t.currency, COALESCE(t.comment, ''),
			t.payee, t.category, t.subcategory, a.title, COALESCE(t.tags, ARRAY[]::text[])
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, conditions, orderBy, argN, argN+1),
		Args: args,
	}
}

func financeTransactionOrderBy(params financeTransactionQueryParams) string {
	orderBy := "t.occurred_at DESC"
	direction := "DESC"
	if params.Order == "asc" {
		direction = "ASC"
	}

	switch params.Sort {
	case "amount":
		orderBy = "ABS(t.amount) " + direction
	case "signed_amount":
		orderBy = "t.amount " + direction
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

	return orderBy
}
