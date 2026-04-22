package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	authmw "life-dashboard/internal/middleware"
)

type FinanceObligation struct {
	Name                string    `json:"name"`
	Category            string    `json:"category,omitempty"`
	Amount              float64   `json:"amount"`
	ProjectedTotal      float64   `json:"projected_total"`
	NextDueAt           time.Time `json:"next_due_at"`
	CadenceDays         int       `json:"cadence_days"`
	CadenceLabel        string    `json:"cadence_label"`
	Occurrences         int       `json:"occurrences"`
	ExpectedOccurrences int       `json:"expected_occurrences"`
}

type FinanceObligationsSummary struct {
	WindowDays    int                 `json:"window_days"`
	UpcomingTotal float64             `json:"upcoming_total"`
	Count         int                 `json:"count"`
	Items         []FinanceObligation `json:"items"`
}

type financeExpenseRecord struct {
	OccurredAt time.Time
	Amount     float64
	Payee      string
	Comment    string
	Category   string
}

type financeRecurringGroup struct {
	Key        string
	Name       string
	Category   string
	Fallback   bool
	KnownLabel bool
	Records    []financeExpenseRecord
}

var (
	financeRecurringStripLongDigits = regexp.MustCompile(`\b\d{4,}\b`)
	financeRecurringNonWord         = regexp.MustCompile(`[^\p{L}\p{N}]+`)
	financeRecurringSpaces          = regexp.MustCompile(`\s+`)
)

func (h *FinanceHandler) GetObligations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	windowDays := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			switch {
			case parsed < 7:
				windowDays = 7
			case parsed > 90:
				windowDays = 90
			default:
				windowDays = parsed
			}
		}
	}

	now := time.Now()
	historyFrom := now.AddDate(0, 0, -210)

	rows, err := h.db.Query(ctx, `
		SELECT
			t.occurred_at,
			ABS(t.amount),
			COALESCE(t.payee, ''),
			COALESCE(t.comment, ''),
			COALESCE(t.category, '')
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.user_id = $1
			AND t.amount < 0
			AND t.currency = 'RUB'
			AND t.is_transfer = FALSE
			AND t.occurred_at >= $2
			AND t.occurred_at < $3
			AND COALESCE(a.in_balance, TRUE) = TRUE
		ORDER BY t.occurred_at ASC
	`, userID, historyFrom, now.Add(24*time.Hour))
	if err != nil {
		h.logger.Error().Err(err).Msg("query obligations source expenses")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	records := make([]financeExpenseRecord, 0, 128)
	for rows.Next() {
		var item financeExpenseRecord
		if err := rows.Scan(&item.OccurredAt, &item.Amount, &item.Payee, &item.Comment, &item.Category); err != nil {
			h.logger.Warn().Err(err).Msg("scan obligations source expense")
			continue
		}
		records = append(records, item)
	}
	if err := rows.Err(); err != nil {
		h.logger.Error().Err(err).Msg("iterate obligations source expenses")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	summary := detectFinanceObligations(records, now, windowDays)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func detectFinanceObligations(records []financeExpenseRecord, now time.Time, windowDays int) FinanceObligationsSummary {
	if windowDays <= 0 {
		windowDays = 30
	}
	start := financeStartOfDay(now)
	end := start.AddDate(0, 0, windowDays)

	groups := make(map[string]*financeRecurringGroup)
	for _, record := range records {
		key, name, fallback, ok := financeRecurringKey(record)
		if !ok {
			continue
		}
		group := groups[key]
		if group == nil {
			group = &financeRecurringGroup{
				Key:        key,
				Name:       name,
				Category:   strings.TrimSpace(record.Category),
				Fallback:   fallback,
				KnownLabel: financeLooksLikeMandatory(name, record.Category),
				Records:    make([]financeExpenseRecord, 0, 8),
			}
			groups[key] = group
		}
		group.Records = append(group.Records, record)
	}

	items := make([]FinanceObligation, 0, len(groups))
	upcomingTotal := 0.0
	for _, group := range groups {
		if item, ok := financeBuildObligation(group, start, end); ok {
			items = append(items, item)
			upcomingTotal += item.ProjectedTotal
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].NextDueAt.Equal(items[j].NextDueAt) {
			return items[i].ProjectedTotal > items[j].ProjectedTotal
		}
		return items[i].NextDueAt.Before(items[j].NextDueAt)
	})

	return FinanceObligationsSummary{
		WindowDays:    windowDays,
		UpcomingTotal: upcomingTotal,
		Count:         len(items),
		Items:         items,
	}
}

func financeBuildObligation(group *financeRecurringGroup, start, end time.Time) (FinanceObligation, bool) {
	if group == nil || len(group.Records) == 0 {
		return FinanceObligation{}, false
	}
	sort.Slice(group.Records, func(i, j int) bool {
		return group.Records[i].OccurredAt.Before(group.Records[j].OccurredAt)
	})

	minOccurrences := 3
	if group.KnownLabel {
		minOccurrences = 2
	}
	if len(group.Records) < minOccurrences {
		return FinanceObligation{}, false
	}

	intervals := financeIntervals(group.Records)
	cadenceDays, cadenceLabel, ok := financeDetectCadence(intervals, group.KnownLabel)
	if !ok {
		return FinanceObligation{}, false
	}

	medianAmount, avgDeviation := financeAmountStats(group.Records)
	deviationLimit := 0.38
	if group.KnownLabel {
		deviationLimit = 0.48
	}
	if avgDeviation > deviationLimit {
		return FinanceObligation{}, false
	}

	lastAt := group.Records[len(group.Records)-1].OccurredAt
	nextDue := lastAt.AddDate(0, 0, cadenceDays)
	for nextDue.Before(start) {
		nextDue = nextDue.AddDate(0, 0, cadenceDays)
	}
	if nextDue.After(end) {
		return FinanceObligation{}, false
	}

	expectedOccurrences := 0
	projectedTotal := 0.0
	for due := nextDue; !due.After(end); due = due.AddDate(0, 0, cadenceDays) {
		expectedOccurrences++
		projectedTotal += medianAmount
	}
	if expectedOccurrences == 0 {
		return FinanceObligation{}, false
	}

	return FinanceObligation{
		Name:                group.Name,
		Category:            group.Category,
		Amount:              medianAmount,
		ProjectedTotal:      projectedTotal,
		NextDueAt:           nextDue,
		CadenceDays:         cadenceDays,
		CadenceLabel:        cadenceLabel,
		Occurrences:         len(group.Records),
		ExpectedOccurrences: expectedOccurrences,
	}, true
}

func financeIntervals(records []financeExpenseRecord) []int {
	if len(records) < 2 {
		return nil
	}
	intervals := make([]int, 0, len(records)-1)
	for i := 1; i < len(records); i++ {
		prev := financeStartOfDay(records[i-1].OccurredAt)
		next := financeStartOfDay(records[i].OccurredAt)
		diff := int(math.Round(next.Sub(prev).Hours() / 24))
		if diff <= 0 {
			continue
		}
		intervals = append(intervals, diff)
	}
	return intervals
}

func financeDetectCadence(intervals []int, knownLabel bool) (int, string, bool) {
	type cadenceOption struct {
		days      int
		label     string
		tolerance int
	}
	options := []cadenceOption{
		{days: 7, label: "каждую неделю", tolerance: 2},
		{days: 14, label: "раз в 2 недели", tolerance: 3},
		{days: 30, label: "раз в месяц", tolerance: 6},
		{days: 90, label: "раз в квартал", tolerance: 12},
	}

	bestScore := 0.0
	bestDays := 0
	bestLabel := ""
	for _, option := range options {
		matches := 0
		for _, interval := range intervals {
			if absInt(interval-option.days) <= option.tolerance {
				matches++
			}
		}
		if matches == 0 {
			continue
		}
		score := float64(matches) / float64(len(intervals))
		threshold := 0.7
		if knownLabel {
			threshold = 0.5
		}
		if score < threshold {
			continue
		}
		if score > bestScore || (score == bestScore && option.days < bestDays) {
			bestScore = score
			bestDays = option.days
			bestLabel = option.label
		}
	}
	if bestDays == 0 {
		return 0, "", false
	}
	return bestDays, bestLabel, true
}

func financeAmountStats(records []financeExpenseRecord) (float64, float64) {
	if len(records) == 0 {
		return 0, 0
	}
	values := make([]float64, 0, len(records))
	for _, record := range records {
		values = append(values, math.Abs(record.Amount))
	}
	sort.Float64s(values)
	median := values[len(values)/2]
	if len(values)%2 == 0 {
		median = (values[len(values)/2-1] + values[len(values)/2]) / 2
	}
	if median == 0 {
		return 0, 0
	}
	totalDeviation := 0.0
	for _, value := range values {
		totalDeviation += math.Abs(value-median) / median
	}
	return median, totalDeviation / float64(len(values))
}

func financeRecurringKey(record financeExpenseRecord) (key string, name string, fallback bool, ok bool) {
	candidates := []string{
		strings.TrimSpace(record.Payee),
		strings.TrimSpace(record.Comment),
	}
	for _, candidate := range candidates {
		if normalized := financeNormalizeRecurringText(candidate); normalized != "" {
			return normalized, candidate, false, true
		}
	}

	category := strings.TrimSpace(record.Category)
	if !financeCategoryCanStandAlone(category) {
		return "", "", false, false
	}
	if normalized := financeNormalizeRecurringText(category); normalized != "" {
		return normalized, category, true, true
	}
	return "", "", false, false
}

func financeNormalizeRecurringText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	value = financeRecurringStripLongDigits.ReplaceAllString(value, " ")
	value = financeRecurringNonWord.ReplaceAllString(value, " ")
	value = financeRecurringSpaces.ReplaceAllString(value, " ")
	value = strings.TrimSpace(value)
	if len([]rune(value)) < 3 {
		return ""
	}
	return value
}

func financeCategoryCanStandAlone(category string) bool {
	if category == "" {
		return false
	}
	category = strings.ToLower(category)
	keywords := []string{
		"subscription", "подпис", "fees", "charges", "коммун", "utility", "internet",
		"интернет", "mobile", "связ", "tax", "налог", "loan", "кредит", "rent", "аренд",
		"insurance", "страх", "housing", "жкх", "mortgage", "ипот",
	}
	for _, keyword := range keywords {
		if strings.Contains(category, keyword) {
			return true
		}
	}
	return false
}

func financeLooksLikeMandatory(name, category string) bool {
	source := strings.ToLower(strings.TrimSpace(name + " " + category))
	keywords := []string{
		"subscription", "подпис", "плюс", "plus", "premium", "icloud", "google one",
		"netflix", "spotify", "youtube", "apple", "яндекс", "tax", "налог",
		"loan", "кредит", "mortgage", "ипот", "rent", "аренд", "insurance",
		"страх", "internet", "интернет", "mobile", "связ", "utilities", "коммун",
		"gym", "fitness", "зал", "membership", "взнос",
	}
	for _, keyword := range keywords {
		if strings.Contains(source, keyword) {
			return true
		}
	}
	return false
}

func financeStartOfDay(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
