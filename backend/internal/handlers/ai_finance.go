package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// aiFinanceObligationLimit keeps the obligation list to what fits in a report:
// the biggest upcoming payments, not the full tail of small subscriptions.
const aiFinanceObligationLimit = 8

type AIFinanceOverviewData struct {
	CurrentBalanceRub    float64        `json:"current_balance_rub"`
	SpendingRub          float64        `json:"spending_rub"`
	IncomeRub            float64        `json:"income_rub"`
	NetRub               float64        `json:"net_rub"`
	TransactionCount     int            `json:"transaction_count"`
	TopExpenseCategories []CategoryStat `json:"top_expense_categories,omitempty"`
	TopExpensePayees     []TopExpense   `json:"top_expense_payees,omitempty"`
	// Obligations are what is already committed for the next month, which is
	// what turns a balance into an answer about whether it is enough.
	UpcomingObligations      []FinanceObligation `json:"upcoming_obligations,omitempty"`
	UpcomingObligationsTotal float64             `json:"upcoming_obligations_total,omitempty"`
	ObligationWindowDays     int                 `json:"obligation_window_days,omitempty"`
}

type AIFinanceTransaction struct {
	OccurredAt time.Time `json:"occurred_at"`
	Amount     float64   `json:"amount"`
	Currency   string    `json:"currency"`
	Label      string    `json:"label"`
	Payee      string    `json:"payee,omitempty"`
	Category   string    `json:"category,omitempty"`
	Direction  string    `json:"direction"`
}

type AIRecentTransactionsData struct {
	Count        int                    `json:"count"`
	Transactions []AIFinanceTransaction `json:"transactions,omitempty"`
}

func (h *AIHandler) buildFinanceOverviewInRange(ctx context.Context, userID string, start, end time.Time) (AIFinanceOverviewData, error) {
	data := AIFinanceOverviewData{}

	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(balance), 0)
		FROM accounts
		WHERE currency = 'RUB'
			AND in_balance = TRUE
			AND COALESCE(archived, FALSE) = FALSE
			AND user_id = $1
	`, userID).Scan(&data.CurrentBalanceRub); err != nil {
		return AIFinanceOverviewData{}, err
	}

	if err := h.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN t.amount < 0 THEN ABS(t.amount) ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN t.amount > 0 THEN t.amount ELSE 0 END), 0),
			COUNT(*)
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.user_id = $1
			AND t.currency = 'RUB'
			AND t.is_transfer = FALSE
			AND t.occurred_at >= $2
			AND t.occurred_at < $3
			AND COALESCE(a.in_balance, TRUE) = TRUE
	`, userID, start, end).Scan(&data.SpendingRub, &data.IncomeRub, &data.TransactionCount); err != nil {
		return AIFinanceOverviewData{}, err
	}
	data.NetRub = data.IncomeRub - data.SpendingRub

	categoryRows, err := h.db.Query(ctx, `
		SELECT COALESCE(NULLIF(t.category, ''), 'Без категории'), COALESCE(SUM(ABS(t.amount)), 0)
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.user_id = $1
			AND t.amount < 0
			AND t.currency = 'RUB'
			AND t.is_transfer = FALSE
			AND t.occurred_at >= $2
			AND t.occurred_at < $3
			AND COALESCE(a.in_balance, TRUE) = TRUE
		GROUP BY 1
		ORDER BY 2 DESC
		LIMIT 5
	`, userID, start, end)
	if err != nil {
		return AIFinanceOverviewData{}, err
	}
	defer categoryRows.Close()

	for categoryRows.Next() {
		var item CategoryStat
		if err := categoryRows.Scan(&item.Category, &item.Amount); err != nil {
			return AIFinanceOverviewData{}, err
		}
		data.TopExpenseCategories = append(data.TopExpenseCategories, item)
	}
	if err := categoryRows.Err(); err != nil {
		return AIFinanceOverviewData{}, err
	}

	payeeRows, err := h.db.Query(ctx, `
		SELECT COALESCE(NULLIF(t.payee, ''), NULLIF(t.comment, ''), 'Без названия'), COALESCE(SUM(ABS(t.amount)), 0), COUNT(*)
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.user_id = $1
			AND t.amount < 0
			AND t.currency = 'RUB'
			AND t.is_transfer = FALSE
			AND t.occurred_at >= $2
			AND t.occurred_at < $3
			AND COALESCE(a.in_balance, TRUE) = TRUE
		GROUP BY 1
		ORDER BY 2 DESC
		LIMIT 5
	`, userID, start, end)
	if err != nil {
		return AIFinanceOverviewData{}, err
	}
	defer payeeRows.Close()

	for payeeRows.Next() {
		var item TopExpense
		if err := payeeRows.Scan(&item.Payee, &item.Amount, &item.Count); err != nil {
			return AIFinanceOverviewData{}, err
		}
		data.TopExpensePayees = append(data.TopExpensePayees, item)
	}
	if err := payeeRows.Err(); err != nil {
		return AIFinanceOverviewData{}, err
	}

	h.attachFinanceObligations(ctx, &data, userID)

	return data, nil
}

// attachFinanceObligations runs the same detection the finance page uses. A
// failure is left silent on purpose: the rest of the overview is still worth
// reporting, and obligations are a projection rather than a fact.
func (h *AIHandler) attachFinanceObligations(ctx context.Context, data *AIFinanceOverviewData, userID string) {
	now := time.Now()
	records, err := loadFinanceExpenseRecords(ctx, h.db, userID, now)
	if err != nil {
		h.logger.Warn().Err(err).Msg("load obligations for ai context")
		return
	}
	rules, err := loadFinanceObligationRules(ctx, h.db, userID)
	if err != nil {
		h.logger.Warn().Err(err).Msg("load obligation rules for ai context")
		return
	}

	summary := detectFinanceObligations(records, now, 30, rules)
	if summary.Count == 0 {
		return
	}
	items := summary.Items
	if len(items) > aiFinanceObligationLimit {
		items = items[:aiFinanceObligationLimit]
	}
	data.UpcomingObligations = items
	data.UpcomingObligationsTotal = summary.UpcomingTotal
	data.ObligationWindowDays = summary.WindowDays
}

func (h *AIHandler) buildRecentTransactionsInRange(ctx context.Context, userID string, start, end time.Time, limit int) (AIRecentTransactionsData, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := h.db.Query(ctx, `
		SELECT
			t.occurred_at,
			t.amount,
			t.currency,
			COALESCE(NULLIF(t.payee, ''), NULLIF(t.comment, ''), 'Без названия'),
			COALESCE(NULLIF(t.payee, ''), ''),
			COALESCE(NULLIF(t.category, ''), '')
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.user_id = $1
			AND t.is_transfer = FALSE
			AND t.occurred_at >= $2
			AND t.occurred_at < $3
			AND COALESCE(a.in_balance, TRUE) = TRUE
		ORDER BY t.occurred_at DESC
		LIMIT $4
	`, userID, start, end, limit)
	if err != nil {
		return AIRecentTransactionsData{}, err
	}
	defer rows.Close()

	data := AIRecentTransactionsData{
		Transactions: make([]AIFinanceTransaction, 0, limit),
	}
	for rows.Next() {
		var item AIFinanceTransaction
		if err := rows.Scan(&item.OccurredAt, &item.Amount, &item.Currency, &item.Label, &item.Payee, &item.Category); err != nil {
			return AIRecentTransactionsData{}, err
		}
		if item.Amount > 0 {
			item.Direction = "income"
		} else {
			item.Direction = "expense"
		}
		data.Transactions = append(data.Transactions, item)
	}
	if err := rows.Err(); err != nil {
		return AIRecentTransactionsData{}, err
	}
	data.Count = len(data.Transactions)

	return data, nil
}

func renderFinanceOverviewText(title string, data AIFinanceOverviewData) string {
	var sb strings.Builder
	sb.WriteString(title + "\n")
	sb.WriteString(fmt.Sprintf("Текущий баланс: %.0f ₽\n", data.CurrentBalanceRub))
	sb.WriteString(fmt.Sprintf("За период: %d транзакций, расходы %.0f ₽, доходы %.0f ₽, net %.0f ₽\n",
		data.TransactionCount, data.SpendingRub, data.IncomeRub, data.NetRub))

	sb.WriteString("Топ категорий:\n")
	if len(data.TopExpenseCategories) == 0 {
		sb.WriteString("  - Нет расходов за период\n")
	} else {
		for _, item := range data.TopExpenseCategories {
			sb.WriteString(fmt.Sprintf("  - %s: %.0f ₽\n", item.Category, item.Amount))
		}
	}

	sb.WriteString(renderFinanceObligationsText(data))

	sb.WriteString("Крупные получатели денег:\n")
	if len(data.TopExpensePayees) == 0 {
		sb.WriteString("  - Нет заметных расходных получателей за период\n")
	} else {
		for _, item := range data.TopExpensePayees {
			sb.WriteString(fmt.Sprintf("  - %s: %.0f ₽ (%d)\n", item.Payee, item.Amount, item.Count))
		}
	}

	return strings.TrimSpace(sb.String())
}

func renderFinanceObligationsText(data AIFinanceOverviewData) string {
	if len(data.UpcomingObligations) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Регулярные платежи на ближайшие %d дн.: %d шт. на %.0f ₽ (прогноз по истории, не факт списания)\n",
		data.ObligationWindowDays, len(data.UpcomingObligations), data.UpcomingObligationsTotal))
	for _, item := range data.UpcomingObligations {
		sb.WriteString(fmt.Sprintf("  - %s: %.0f ₽, ожидается %s, %s\n",
			item.Name, item.Amount, item.NextDueAt.In(aiDisplayLocation).Format("02.01"), item.CadenceLabel))
	}
	return sb.String()
}

func renderRecentTransactionsText(title string, data AIRecentTransactionsData) string {
	var sb strings.Builder
	sb.WriteString(title + "\n")

	if len(data.Transactions) == 0 {
		sb.WriteString("  Нет транзакций за период\n")
		return strings.TrimSpace(sb.String())
	}

	for _, item := range data.Transactions {
		sign := ""
		if item.Amount > 0 {
			sign = "+"
		}
		sb.WriteString(fmt.Sprintf("  %s %s%.0f %s %s\n",
			formatAITimestampLocal(item.OccurredAt, "02.01 15:04"),
			sign,
			item.Amount,
			item.Currency,
			item.Label,
		))
	}

	return strings.TrimSpace(sb.String())
}
