package handlers

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Financial literacy is judged from behaviour the bank records, not from what a
// person knows. Each signal below is a number from the account's own data, a
// threshold, and a status - so the model is reporting facts that can be argued
// with, not handing out an opinion.
//
// The thresholds are rules of thumb, chosen by hand. They are constants with
// their reasoning next to them so they can be moved when they turn out wrong.
const (
	// literacyMonths is how much history the savings rate is judged on. Six
	// months is long enough that one renovation does not decide the verdict.
	literacyMonths = 6
	// literacyIncomeMonths is the window for the income regular payments are
	// compared against. Income moves slowly; three months is recent enough.
	literacyIncomeMonths = 3

	// A median month that saves at least a tenth of income is the usual floor
	// for "spends less than earns" with room for something to go wrong.
	literacySavingsRateOK = 0.10
	// Three months of spending is the common size of an emergency cushion; under
	// one month, a single surprise has to be paid with a loan.
	literacyCushionOKMonths        = 3.0
	literacyCushionAttentionMonths = 1.0
	// A week of spending stays on the card on purpose - it is what gets paid with.
	// Everything above it could sit on a savings account, which is just as
	// liquid, and earn.
	literacyOperatingBufferShare = 0.25
	// Below this the interest lost is not worth mentioning.
	literacyIdleLossFloorRub = 100.0
	// A month where one category ate more than half the spending is reported as
	// such: a renovation is a decision, not a habit, and it should not read as one.
	literacyDominantCategoryShare = 0.5
)

type AILiteracyStatus string

const (
	literacyOK        AILiteracyStatus = "ok"
	literacyAttention AILiteracyStatus = "attention"
	literacyBad       AILiteracyStatus = "bad"
	literacyInfo      AILiteracyStatus = "info"
)

type AILiteracyMonth struct {
	Month       string   `json:"month"`
	IncomeRub   float64  `json:"income_rub"`
	ExpenseRub  float64  `json:"expense_rub"`
	SavingsRate *float64 `json:"savings_rate,omitempty"`
	// Set when one category ate more than half the month's spending.
	DominantCategory      string  `json:"dominant_category,omitempty"`
	DominantCategoryShare float64 `json:"dominant_category_share,omitempty"`
}

type AILiteracyFine struct {
	Date   string  `json:"date"`
	Amount float64 `json:"amount"`
	Payee  string  `json:"payee"`
}

type AILiteracySignal struct {
	Name   string           `json:"name"`
	Status AILiteracyStatus `json:"status"`
	Value  string           `json:"value"`
	Rule   string           `json:"rule"`
}

type AIFinanceLiteracyData struct {
	Months            []AILiteracyMonth `json:"months"`
	MedianSavingsRate *float64          `json:"median_savings_rate,omitempty"`
	LiquidRub         float64           `json:"liquid_rub"`
	// TypicalExpense is the median month, not the mean: the mean of the last
	// three months included a 307 000 ₽ renovation and made the cushion look a
	// third of its real size.
	TypicalExpense    float64            `json:"typical_monthly_expense_rub"`
	CushionMonths     *float64           `json:"cushion_months,omitempty"`
	IdleRub           float64            `json:"idle_rub"`
	SavingsRub        float64            `json:"savings_rub"`
	SavingsRatePct    *float64           `json:"savings_rate_pct,omitempty"`
	SavingsRateSource string             `json:"savings_rate_source,omitempty"`
	IdleMonthlyLoss   *float64           `json:"idle_monthly_loss_rub,omitempty"`
	Fines             []AILiteracyFine   `json:"fines,omitempty"`
	FinesRub          float64            `json:"fines_rub"`
	RecurringRub      float64            `json:"recurring_monthly_rub"`
	RecurringShare    *float64           `json:"recurring_share_of_income,omitempty"`
	Debts             AIDebtsData        `json:"debts"`
	BankLoansRub      float64            `json:"bank_loans_rub"`
	Signals           []AILiteracySignal `json:"signals"`
}

// literacyAccount is one account as the literacy checks see it.
type literacyAccount struct {
	title   string
	accType string
	balance float64
}

// savingsAccountPattern recognises an account meant for keeping money rather
// than spending it. Zenmoney types are not enough: Yandex's "Сейв" comes through
// as a plain checking account.
var savingsAccountPattern = regexp.MustCompile(`(?i)(накоп|сейв|save|вклад|депозит)`)

// percentInTitlePattern finds the rate a bank writes into the account's own
// name, as in "Сейв (15% • счёт)".
var percentInTitlePattern = regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*%`)

func isSavingsAccount(account literacyAccount) bool {
	return account.accType == "deposit" || savingsAccountPattern.MatchString(account.title)
}

// splitIdleAndSavings separates money that earns from money that does not, and
// finds the best rate the person already has an account for.
func splitIdleAndSavings(accounts []literacyAccount) (idle, savings float64, rate *float64, source string) {
	for _, account := range accounts {
		if account.balance <= 0 {
			if isSavingsAccount(account) {
				if pct, ok := percentInTitle(account.title); ok && (rate == nil || pct > *rate) {
					rate, source = &pct, account.title
				}
			}
			continue
		}
		switch {
		case isSavingsAccount(account):
			savings += account.balance
			if pct, ok := percentInTitle(account.title); ok && (rate == nil || pct > *rate) {
				rate, source = &pct, account.title
			}
		case account.accType == "ccard" || account.accType == "checking" || account.accType == "cash":
			// A brokerage account is not idle money - it is invested elsewhere.
			if strings.Contains(strings.ToLower(account.title), "брокер") {
				continue
			}
			idle += account.balance
		}
	}
	return idle, savings, rate, source
}

func percentInTitle(title string) (float64, bool) {
	match := percentInTitlePattern.FindStringSubmatch(title)
	if len(match) < 2 {
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", "."), 64)
	if err != nil || value <= 0 || value > 50 {
		return 0, false
	}
	return value, true
}

// idleMonthlyLoss is what the money above a week of spending would earn on the
// person's own savings account. Nil when there is no rate to compute it with:
// inventing one would be the opposite of the point.
func idleMonthlyLoss(idle, typicalExpense float64, ratePct *float64) *float64 {
	if ratePct == nil {
		return nil
	}
	excess := idle - typicalExpense*literacyOperatingBufferShare
	if excess <= 0 {
		zero := 0.0
		return &zero
	}
	loss := math.Round(excess * *ratePct / 100 / 12)
	return &loss
}

func medianOf(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	median := sorted[middle]
	if len(sorted)%2 == 0 {
		median = (sorted[middle-1] + sorted[middle]) / 2
	}
	return &median
}

// literacySignals turns the numbers into the six verdicts the model reports.
func literacySignals(data AIFinanceLiteracyData) []AILiteracySignal {
	signals := make([]AILiteracySignal, 0, 6)

	savings := AILiteracySignal{Name: "норма сбережений", Rule: "медиана за 6 месяцев: от 10% - норма, 0-10% - внимание, ниже нуля - плохо"}
	switch {
	case data.MedianSavingsRate == nil:
		savings.Status, savings.Value = literacyInfo, "нет месяцев с доходом"
	case *data.MedianSavingsRate >= literacySavingsRateOK:
		savings.Status, savings.Value = literacyOK, formatPercent(*data.MedianSavingsRate)
	case *data.MedianSavingsRate >= 0:
		savings.Status, savings.Value = literacyAttention, formatPercent(*data.MedianSavingsRate)
	default:
		savings.Status, savings.Value = literacyBad, formatPercent(*data.MedianSavingsRate)
	}
	signals = append(signals, savings)

	cushion := AILiteracySignal{Name: "подушка", Rule: "свободные деньги в месяцах обычных трат (медиана за полгода): от 3 - норма, 1-3 - внимание, меньше 1 - плохо"}
	switch {
	case data.CushionMonths == nil:
		cushion.Status, cushion.Value = literacyInfo, "не из чего считать средние траты"
	case *data.CushionMonths >= literacyCushionOKMonths:
		cushion.Status, cushion.Value = literacyOK, fmt.Sprintf("%.1f мес.", *data.CushionMonths)
	case *data.CushionMonths >= literacyCushionAttentionMonths:
		cushion.Status, cushion.Value = literacyAttention, fmt.Sprintf("%.1f мес.", *data.CushionMonths)
	default:
		cushion.Status, cushion.Value = literacyBad, fmt.Sprintf("%.1f мес.", *data.CushionMonths)
	}
	signals = append(signals, cushion)

	idle := AILiteracySignal{Name: "деньги без процента", Rule: "всё сверх недели трат на картах можно держать на накопительном счёте, он так же доступен"}
	switch {
	case data.IdleMonthlyLoss == nil:
		idle.Status, idle.Value = literacyInfo, fmt.Sprintf("%.0f ₽ на картах, ставка накопительного неизвестна", data.IdleRub)
	case *data.IdleMonthlyLoss < literacyIdleLossFloorRub:
		idle.Status, idle.Value = literacyOK, fmt.Sprintf("%.0f ₽ на картах, лишнего почти нет", data.IdleRub)
	default:
		idle.Status = literacyAttention
		idle.Value = fmt.Sprintf("%.0f ₽ на картах; на \"%s\" (%.0f%% по названию счёта, не проверено) это ~%.0f ₽ в месяц",
			data.IdleRub, data.SavingsRateSource, *data.SavingsRatePct, *data.IdleMonthlyLoss)
	}
	signals = append(signals, idle)

	fines := AILiteracySignal{Name: "штрафы", Rule: "деньги, потерянные без покупки; правильная цель - ноль. Считаются только операции со словом \"штраф\", а не вся категория комиссий"}
	if data.FinesRub == 0 {
		fines.Status, fines.Value = literacyOK, "за 6 месяцев не было"
	} else {
		fines.Status, fines.Value = literacyAttention, fmt.Sprintf("%.0f ₽ за 6 месяцев, %d шт.", data.FinesRub, len(data.Fines))
	}
	signals = append(signals, fines)

	recurring := AILiteracySignal{Name: "регулярные платежи", Rule: "подписки и обязательные платежи как доля дохода; справочно, без оценки - пользуется ли он сервисом, данные не знают"}
	recurring.Status = literacyInfo
	if data.RecurringShare != nil {
		recurring.Value = fmt.Sprintf("%.0f ₽ в месяц, %s дохода", data.RecurringRub, formatPercent(*data.RecurringShare))
	} else {
		recurring.Value = fmt.Sprintf("%.0f ₽ в месяц", data.RecurringRub)
	}
	signals = append(signals, recurring)

	debts := AILiteracySignal{Name: "долги", Rule: "долги людям из раздела Debts считаются беспроцентными - гасить их досрочно в ущерб подушке невыгодно; банковские кредиты - отдельно"}
	switch {
	case data.BankLoansRub > 0:
		debts.Status = literacyAttention
		debts.Value = fmt.Sprintf("банковских кредитов %.0f ₽; людям должен %.0f ₽", data.BankLoansRub, data.Debts.IOweRub)
	case data.Debts.IOweRub > data.LiquidRub && data.Debts.IOweRub > 0:
		debts.Status = literacyAttention
		debts.Value = fmt.Sprintf("людям должен %.0f ₽ - больше, чем все свободные деньги (%.0f ₽); кредитов в банках нет", data.Debts.IOweRub, data.LiquidRub)
	default:
		debts.Status = literacyOK
		debts.Value = fmt.Sprintf("людям должен %.0f ₽, ему должны %.0f ₽; кредитов в банках нет", data.Debts.IOweRub, data.Debts.OwedToMeRub)
	}
	signals = append(signals, debts)

	return signals
}

func formatPercent(share float64) string {
	return fmt.Sprintf("%.0f%%", share*100)
}

func (h *AIHandler) buildFinanceLiteracy(ctx context.Context, userID string, now time.Time) (AIFinanceLiteracyData, error) {
	data := AIFinanceLiteracyData{}
	local := now.In(aiDisplayLocation)
	currentMonthStart := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, aiDisplayLocation)
	historyStart := currentMonthStart.AddDate(0, -literacyMonths, 0)

	months, err := h.literacyMonths(ctx, userID, historyStart, currentMonthStart)
	if err != nil {
		return data, err
	}
	data.Months = months

	rates := make([]float64, 0, len(months))
	for _, month := range months {
		if month.SavingsRate != nil {
			rates = append(rates, *month.SavingsRate)
		}
	}
	data.MedianSavingsRate = medianOf(rates)

	expenses := make([]float64, 0, len(months))
	for _, month := range months {
		if month.ExpenseRub > 0 {
			expenses = append(expenses, month.ExpenseRub)
		}
	}
	if typical := medianOf(expenses); typical != nil {
		data.TypicalExpense = math.Round(*typical)
	}

	recent := months
	if len(recent) > literacyIncomeMonths {
		recent = recent[len(recent)-literacyIncomeMonths:]
	}

	accounts, liquid, loans, err := h.literacyAccounts(ctx, userID)
	if err != nil {
		return data, err
	}
	data.LiquidRub = liquid
	data.BankLoansRub = loans
	if data.TypicalExpense > 0 {
		cushion := math.Round(liquid/data.TypicalExpense*10) / 10
		data.CushionMonths = &cushion
	}
	data.IdleRub, data.SavingsRub, data.SavingsRatePct, data.SavingsRateSource = splitIdleAndSavings(accounts)
	data.IdleMonthlyLoss = idleMonthlyLoss(data.IdleRub, data.TypicalExpense, data.SavingsRatePct)

	fines, err := h.literacyFines(ctx, userID, historyStart)
	if err != nil {
		return data, err
	}
	data.Fines = fines
	for _, fine := range fines {
		data.FinesRub += fine.Amount
	}

	overview := AIFinanceOverviewData{}
	h.attachFinanceObligations(ctx, &overview, userID)
	data.RecurringRub = math.Round(overview.UpcomingObligationsTotal)
	var incomeSum float64
	for _, month := range recent {
		incomeSum += month.IncomeRub
	}
	if len(recent) > 0 && incomeSum > 0 {
		share := data.RecurringRub / (incomeSum / float64(len(recent)))
		data.RecurringShare = &share
	}

	debts, err := h.buildDebts(ctx, userID)
	if err != nil {
		return data, err
	}
	data.Debts = debts

	data.Signals = literacySignals(data)
	return data, nil
}

// literacyMonths reads full months only: a month in progress would always look
// like it saves everything, because the rent has not been paid yet.
func (h *AIHandler) literacyMonths(ctx context.Context, userID string, start, end time.Time) ([]AILiteracyMonth, error) {
	rows, err := h.db.Query(ctx, `
		WITH ops AS (
			SELECT to_char(t.occurred_at AT TIME ZONE 'Europe/Moscow', 'YYYY-MM') AS month,
			       t.amount, COALESCE(NULLIF(t.category, ''), 'без категории') AS category
			FROM transactions t
			JOIN accounts a ON a.id = t.account_id
			WHERE t.user_id = $1
			  AND NOT t.is_transfer
			  AND a.in_balance
			  AND t.currency = 'RUB'
			  AND t.occurred_at >= $2 AND t.occurred_at < $3
		),
		totals AS (
			SELECT month,
			       SUM(amount) FILTER (WHERE amount > 0) AS income,
			       -SUM(amount) FILTER (WHERE amount < 0) AS expense
			FROM ops GROUP BY month
		),
		top_category AS (
			SELECT DISTINCT ON (month) month, category, -SUM(amount) AS spent
			FROM ops WHERE amount < 0
			GROUP BY month, category
			ORDER BY month, -SUM(amount) DESC
		)
		SELECT t.month, COALESCE(t.income, 0), COALESCE(t.expense, 0),
		       COALESCE(c.category, ''), COALESCE(c.spent, 0)
		FROM totals t LEFT JOIN top_category c ON c.month = t.month
		ORDER BY t.month
	`, userID, start, end)
	if err != nil {
		return nil, fmt.Errorf("load literacy months: %w", err)
	}
	defer rows.Close()

	months := make([]AILiteracyMonth, 0, literacyMonths)
	for rows.Next() {
		var month AILiteracyMonth
		var topCategory string
		var topSpent float64
		if err := rows.Scan(&month.Month, &month.IncomeRub, &month.ExpenseRub, &topCategory, &topSpent); err != nil {
			return nil, fmt.Errorf("scan literacy month: %w", err)
		}
		month.IncomeRub, month.ExpenseRub = math.Round(month.IncomeRub), math.Round(month.ExpenseRub)
		if month.IncomeRub > 0 {
			rate := (month.IncomeRub - month.ExpenseRub) / month.IncomeRub
			month.SavingsRate = &rate
		}
		if month.ExpenseRub > 0 && topSpent/month.ExpenseRub > literacyDominantCategoryShare {
			month.DominantCategory = topCategory
			month.DominantCategoryShare = topSpent / month.ExpenseRub
		}
		months = append(months, month)
	}
	return months, rows.Err()
}

func (h *AIHandler) literacyAccounts(ctx context.Context, userID string) ([]literacyAccount, float64, float64, error) {
	rows, err := h.db.Query(ctx, `
		SELECT title, COALESCE(type, ''), balance, in_balance
		FROM accounts
		WHERE user_id = $1 AND COALESCE(archived, FALSE) = FALSE AND currency = 'RUB'
	`, userID)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("load literacy accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]literacyAccount, 0, 16)
	var liquid, loans float64
	for rows.Next() {
		var account literacyAccount
		var inBalance bool
		if err := rows.Scan(&account.title, &account.accType, &account.balance, &inBalance); err != nil {
			return nil, 0, 0, fmt.Errorf("scan literacy account: %w", err)
		}
		if account.accType == "loan" {
			loans += math.Abs(account.balance)
			continue
		}
		if account.accType == "debt" || !inBalance {
			continue
		}
		liquid += account.balance
		accounts = append(accounts, account)
	}
	return accounts, math.Round(liquid), math.Round(loans), rows.Err()
}

// literacyFines finds fines by what the operation says it is. The category is
// no help: "Fees & Charges" holds parking, state duties and a services
// marketplace alongside the actual penalties, and calling all of it fines
// tripled the figure. The database collates in C, so both cases are spelled out.
func (h *AIHandler) literacyFines(ctx context.Context, userID string, start time.Time) ([]AILiteracyFine, error) {
	rows, err := h.db.Query(ctx, `
		SELECT to_char(t.occurred_at AT TIME ZONE 'Europe/Moscow', 'DD.MM.YY'), -t.amount,
		       COALESCE(NULLIF(t.payee, ''), NULLIF(t.comment, ''), '-')
		FROM transactions t
		WHERE t.user_id = $1 AND t.amount < 0 AND t.occurred_at >= $2
		  AND (t.comment LIKE '%Штраф%' OR t.comment LIKE '%штраф%'
		       OR t.payee LIKE '%Штраф%' OR t.payee LIKE '%штраф%')
		ORDER BY t.occurred_at DESC
	`, userID, start)
	if err != nil {
		return nil, fmt.Errorf("load fines: %w", err)
	}
	defer rows.Close()

	fines := make([]AILiteracyFine, 0, 4)
	for rows.Next() {
		var fine AILiteracyFine
		if err := rows.Scan(&fine.Date, &fine.Amount, &fine.Payee); err != nil {
			return nil, fmt.Errorf("scan fine: %w", err)
		}
		fines = append(fines, fine)
	}
	return fines, rows.Err()
}

func renderFinanceLiteracyText(data AIFinanceLiteracyData) string {
	var sb strings.Builder
	sb.WriteString("=== ФИНАНСОВАЯ ГРАМОТНОСТЬ ===\n")
	sb.WriteString("Оценка по поведению в данных, не по знаниям. Пороги - практические ориентиры, а не закон; называй число и порог, не морализируй.\n")

	ok, judged := 0, 0
	for _, signal := range data.Signals {
		if signal.Status == literacyInfo {
			continue
		}
		judged++
		if signal.Status == literacyOK {
			ok++
		}
	}
	sb.WriteString(fmt.Sprintf("В норме %d из %d оцениваемых сигналов.\n", ok, judged))

	for _, signal := range data.Signals {
		sb.WriteString(fmt.Sprintf("- %s [%s]: %s. Порог: %s.\n", signal.Name, signal.Status, signal.Value, signal.Rule))
	}

	if len(data.Months) > 0 {
		sb.WriteString("Месяцы (только полные):\n")
		for _, month := range data.Months {
			line := fmt.Sprintf("  - %s: доход %.0f ₽, расход %.0f ₽", month.Month, month.IncomeRub, month.ExpenseRub)
			if month.SavingsRate != nil {
				line += ", сбережения " + formatPercent(*month.SavingsRate)
			}
			if month.DominantCategory != "" {
				line += fmt.Sprintf(" - разовый месяц: \"%s\" %.0f%% расходов", month.DominantCategory, month.DominantCategoryShare*100)
			}
			sb.WriteString(line + "\n")
		}
	}

	if len(data.Fines) > 0 {
		sb.WriteString("Штрафы:\n")
		for _, fine := range data.Fines {
			sb.WriteString(fmt.Sprintf("  - %s: %.0f ₽ (%s)\n", fine.Date, fine.Amount, fine.Payee))
		}
	}

	sb.WriteString(renderDebtsText(data.Debts))
	return sb.String()
}

func (h *AIHandler) financeLiteracyExecution(ctx context.Context, userID string) aiToolExecution {
	var cached *AIFinanceLiteracyData
	load := func() (AIFinanceLiteracyData, error) {
		if cached != nil {
			return *cached, nil
		}
		data, err := h.buildFinanceLiteracy(ctx, userID, time.Now())
		if err != nil {
			return AIFinanceLiteracyData{}, err
		}
		cached = &data
		return data, nil
	}
	return aiToolExecution{
		Name:    aiToolFinanceLiteracy,
		Section: "финансовая грамотность",
		Run: func(sb *strings.Builder) error {
			data, err := load()
			if err != nil {
				return err
			}
			sb.WriteString(renderFinanceLiteracyText(data))
			return nil
		},
		Data: func() (any, error) {
			data, err := load()
			if err != nil {
				return nil, err
			}
			return data, nil
		},
	}
}
