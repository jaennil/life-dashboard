package handlers

import (
	"testing"
	"time"
)

func TestDetectFinanceObligationsMonthly(t *testing.T) {
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	records := []financeExpenseRecord{
		{OccurredAt: time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC), Amount: 499, Payee: "Яндекс Плюс", Category: "Subscriptions"},
		{OccurredAt: time.Date(2026, 2, 4, 9, 0, 0, 0, time.UTC), Amount: 499, Payee: "Яндекс Плюс", Category: "Subscriptions"},
		{OccurredAt: time.Date(2026, 3, 6, 9, 0, 0, 0, time.UTC), Amount: 499, Payee: "Яндекс Плюс", Category: "Subscriptions"},
		{OccurredAt: time.Date(2026, 4, 5, 9, 0, 0, 0, time.UTC), Amount: 499, Payee: "Яндекс Плюс", Category: "Subscriptions"},
		{OccurredAt: time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC), Amount: 320, Payee: "Кофе", Category: "Eating out"},
		{OccurredAt: time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC), Amount: 340, Payee: "Кофе", Category: "Eating out"},
	}

	summary := detectFinanceObligations(records, now, 30, nil)
	if summary.Count != 1 {
		t.Fatalf("expected 1 obligation, got %d", summary.Count)
	}
	if got := summary.Items[0].Name; got != "Яндекс Плюс" {
		t.Fatalf("expected obligation name to be kept, got %q", got)
	}
	if got := summary.Items[0].CadenceDays; got != 30 {
		t.Fatalf("expected monthly cadence, got %d", got)
	}
	if got := summary.Items[0].ExpectedOccurrences; got != 1 {
		t.Fatalf("expected one occurrence in next 30 days, got %d", got)
	}
	if got := summary.Items[0].ProjectedTotal; got != 499 {
		t.Fatalf("expected projected total 499, got %.2f", got)
	}
}

func TestDetectFinanceObligationsWeeklyProjectedMultipleTimes(t *testing.T) {
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	records := []financeExpenseRecord{
		{OccurredAt: time.Date(2026, 3, 25, 9, 0, 0, 0, time.UTC), Amount: 890, Payee: "Спортзал", Category: "Fitness"},
		{OccurredAt: time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC), Amount: 890, Payee: "Спортзал", Category: "Fitness"},
		{OccurredAt: time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC), Amount: 890, Payee: "Спортзал", Category: "Fitness"},
		{OccurredAt: time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC), Amount: 890, Payee: "Спортзал", Category: "Fitness"},
		{OccurredAt: time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC), Amount: 890, Payee: "Спортзал", Category: "Fitness"},
	}

	summary := detectFinanceObligations(records, now, 30, nil)
	if summary.Count != 1 {
		t.Fatalf("expected 1 obligation, got %d", summary.Count)
	}
	if got := summary.Items[0].CadenceDays; got != 7 {
		t.Fatalf("expected weekly cadence, got %d", got)
	}
	if got := summary.Items[0].ExpectedOccurrences; got != 4 {
		t.Fatalf("expected 4 occurrences in next 30 days, got %d", got)
	}
	if got := summary.Items[0].ProjectedTotal; got != 3560 {
		t.Fatalf("expected projected total 3560, got %.2f", got)
	}
}

func TestDetectFinanceObligationsIgnoresNoisyHistory(t *testing.T) {
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	records := []financeExpenseRecord{
		{OccurredAt: time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC), Amount: 1200, Payee: "Маркет", Category: "Shopping"},
		{OccurredAt: time.Date(2026, 1, 17, 9, 0, 0, 0, time.UTC), Amount: 650, Payee: "Маркет", Category: "Shopping"},
		{OccurredAt: time.Date(2026, 2, 2, 9, 0, 0, 0, time.UTC), Amount: 2100, Payee: "Маркет", Category: "Shopping"},
		{OccurredAt: time.Date(2026, 3, 19, 9, 0, 0, 0, time.UTC), Amount: 980, Payee: "Маркет", Category: "Shopping"},
		{OccurredAt: time.Date(2026, 4, 11, 9, 0, 0, 0, time.UTC), Amount: 4500, Payee: "Маркет", Category: "Shopping"},
	}

	summary := detectFinanceObligations(records, now, 30, nil)
	if summary.Count != 0 {
		t.Fatalf("expected noisy history to be ignored, got %d obligations", summary.Count)
	}
}

func TestDetectFinanceObligationsSkipsLowSignalGroceries(t *testing.T) {
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	records := []financeExpenseRecord{
		{OccurredAt: time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC), Amount: 429, Payee: "MA0004883725", Category: "Groceries"},
		{OccurredAt: time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC), Amount: 429, Payee: "MA0004883725", Category: "Groceries"},
		{OccurredAt: time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC), Amount: 429, Payee: "MA0004883725", Category: "Groceries"},
	}

	summary := detectFinanceObligations(records, now, 30, nil)
	if summary.Count != 0 {
		t.Fatalf("expected low-signal groceries to be ignored, got %d obligations", summary.Count)
	}
}

func TestDetectFinanceObligationsAppliesIgnoreRule(t *testing.T) {
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	records := []financeExpenseRecord{
		{OccurredAt: time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC), Amount: 499, Payee: "Яндекс Плюс", Category: "Subscriptions"},
		{OccurredAt: time.Date(2026, 2, 4, 9, 0, 0, 0, time.UTC), Amount: 499, Payee: "Яндекс Плюс", Category: "Subscriptions"},
		{OccurredAt: time.Date(2026, 3, 6, 9, 0, 0, 0, time.UTC), Amount: 499, Payee: "Яндекс Плюс", Category: "Subscriptions"},
	}
	rules := []FinanceObligationRule{
		{Key: "яндекс плюс", Label: "Яндекс Плюс", Action: "ignore"},
	}

	summary := detectFinanceObligations(records, now, 30, rules)
	if summary.Count != 0 {
		t.Fatalf("expected ignore rule to hide obligation, got %d", summary.Count)
	}
}

func TestDetectFinanceObligationsMarksForcedRule(t *testing.T) {
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	records := []financeExpenseRecord{
		{OccurredAt: time.Date(2026, 3, 25, 9, 0, 0, 0, time.UTC), Amount: 890, Payee: "Спортзал", Category: "Fitness"},
		{OccurredAt: time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC), Amount: 890, Payee: "Спортзал", Category: "Fitness"},
		{OccurredAt: time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC), Amount: 890, Payee: "Спортзал", Category: "Fitness"},
	}
	rules := []FinanceObligationRule{
		{Key: "спортзал", Label: "Спортзал", Action: "force"},
	}

	summary := detectFinanceObligations(records, now, 30, rules)
	if summary.Count != 1 {
		t.Fatalf("expected forced rule to keep obligation, got %d", summary.Count)
	}
	if summary.Items[0].RuleAction != "force" {
		t.Fatalf("expected force action to be reflected in item, got %q", summary.Items[0].RuleAction)
	}
}
