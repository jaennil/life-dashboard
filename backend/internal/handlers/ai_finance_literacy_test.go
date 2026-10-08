package handlers

import (
	"strings"
	"testing"
)

func TestPercentInTitle(t *testing.T) {
	cases := []struct {
		title string
		want  float64
		ok    bool
	}{
		{"Сейв (15% • счёт)", 15, true},
		{"Накопительный 16,5 %", 16.5, true},
		{"Тинькофф Black", 0, false},
		{"Счёт 0%", 0, false},
		{"Кешбэк 99%", 0, false},
	}
	for _, tc := range cases {
		got, ok := percentInTitle(tc.title)
		if ok != tc.ok || got != tc.want {
			t.Errorf("percentInTitle(%q) = %v, %v; want %v, %v", tc.title, got, ok, tc.want, tc.ok)
		}
	}
}

func TestSplitIdleAndSavings(t *testing.T) {
	accounts := []literacyAccount{
		{title: "Сбер", accType: "ccard", balance: 40000},
		{title: "Наличные", accType: "cash", balance: 5000},
		{title: "Сейв (15% • счёт)", accType: "checking", balance: 1000},
		{title: "Накопительный 12%", accType: "checking", balance: 0},
		{title: "Брокерский счёт", accType: "checking", balance: 70000},
		{title: "Кредитка", accType: "ccard", balance: -3000},
	}
	idle, savings, rate, source := splitIdleAndSavings(accounts)
	if idle != 45000 {
		t.Errorf("idle = %v, want 45000", idle)
	}
	if savings != 1000 {
		t.Errorf("savings = %v, want 1000", savings)
	}
	if rate == nil || *rate != 15 || source != "Сейв (15% • счёт)" {
		t.Errorf("rate = %v from %q, want 15 from the Сейв account", rate, source)
	}
}

func TestSplitIdleAndSavingsUsesEmptySavingsAccountRate(t *testing.T) {
	// An empty savings account is exactly the case the signal exists for: the
	// rate is available and nothing is on it.
	_, _, rate, _ := splitIdleAndSavings([]literacyAccount{
		{title: "Сейв (15% • счёт)", accType: "checking", balance: 0},
	})
	if rate == nil || *rate != 15 {
		t.Fatalf("rate = %v, want 15", rate)
	}
}

func TestIdleMonthlyLoss(t *testing.T) {
	if idleMonthlyLoss(100000, 40000, nil) != nil {
		t.Error("loss without a known rate must stay unknown")
	}
	rate := 12.0
	// 100 000 minus a week of 40 000 a month leaves 90 000; 12% a year is 900 a month.
	if got := idleMonthlyLoss(100000, 40000, &rate); got == nil || *got != 900 {
		t.Errorf("loss = %v, want 900", got)
	}
	if got := idleMonthlyLoss(5000, 40000, &rate); got == nil || *got != 0 {
		t.Errorf("loss below the buffer = %v, want 0", got)
	}
}

func TestMedianOf(t *testing.T) {
	if medianOf(nil) != nil {
		t.Error("median of nothing must be nil")
	}
	if got := medianOf([]float64{3, 1, 2}); *got != 2 {
		t.Errorf("odd median = %v, want 2", *got)
	}
	// A single renovation month must not move the typical month.
	if got := medianOf([]float64{90000, 100000, 307000, 110000}); *got != 105000 {
		t.Errorf("even median = %v, want 105000", *got)
	}
}

func literacyFloat(value float64) *float64 { return &value }

func signalByName(t *testing.T, signals []AILiteracySignal, name string) AILiteracySignal {
	t.Helper()
	for _, signal := range signals {
		if signal.Name == name {
			return signal
		}
	}
	t.Fatalf("no signal %q", name)
	return AILiteracySignal{}
}

func TestLiteracySignals(t *testing.T) {
	data := AIFinanceLiteracyData{
		MedianSavingsRate: literacyFloat(0.05),
		CushionMonths:     literacyFloat(0.6),
		LiquidRub:         80000,
		IdleRub:           80000,
		SavingsRatePct:    literacyFloat(15),
		SavingsRateSource: "Сейв (15% • счёт)",
		IdleMonthlyLoss:   literacyFloat(800),
		FinesRub:          1692.5,
		Fines:             []AILiteracyFine{{Amount: 567.5}, {Amount: 1125}},
		Debts:             AIDebtsData{IOweRub: 150000, OwedToMeRub: 16634},
	}
	signals := literacySignals(data)
	if len(signals) != 6 {
		t.Fatalf("got %d signals, want 6", len(signals))
	}

	want := map[string]AILiteracyStatus{
		"норма сбережений":    literacyAttention,
		"подушка":             literacyBad,
		"деньги без процента": literacyAttention,
		"штрафы":              literacyAttention,
		"регулярные платежи":  literacyInfo,
		"долги":               literacyAttention,
	}
	for name, status := range want {
		if got := signalByName(t, signals, name).Status; got != status {
			t.Errorf("%s = %s, want %s", name, got, status)
		}
	}
	if value := signalByName(t, signals, "долги").Value; !strings.Contains(value, "кредитов в банках нет") {
		t.Errorf("personal debts must not read as bank loans: %q", value)
	}
}

func TestLiteracySignalsHealthy(t *testing.T) {
	data := AIFinanceLiteracyData{
		MedianSavingsRate: literacyFloat(0.2),
		CushionMonths:     literacyFloat(4),
		LiquidRub:         400000,
		IdleRub:           20000,
		SavingsRatePct:    literacyFloat(15),
		IdleMonthlyLoss:   literacyFloat(0),
		Debts:             AIDebtsData{IOweRub: 50000},
	}
	for _, signal := range literacySignals(data) {
		if signal.Status != literacyOK && signal.Status != literacyInfo {
			t.Errorf("%s = %s, want ok", signal.Name, signal.Status)
		}
	}
}

func TestLiteracySignalsWithoutRate(t *testing.T) {
	// Nothing known about the rate: the signal says so instead of inventing one.
	signal := signalByName(t, literacySignals(AIFinanceLiteracyData{IdleRub: 50000}), "деньги без процента")
	if signal.Status != literacyInfo {
		t.Errorf("status = %s, want info", signal.Status)
	}
}

func TestRenderFinanceLiteracyText(t *testing.T) {
	data := AIFinanceLiteracyData{
		Months: []AILiteracyMonth{
			{Month: "2026-07", IncomeRub: 150000, ExpenseRub: 300000, SavingsRate: literacyFloat(-1), DominantCategory: "Ремонт", DominantCategoryShare: 0.57},
			{Month: "2026-08", IncomeRub: 150000, ExpenseRub: 120000, SavingsRate: literacyFloat(0.2)},
		},
		Fines: []AILiteracyFine{{Date: "27.04.26", Amount: 1125, Payee: "Штрафы ГАИ"}},
		Signals: []AILiteracySignal{
			{Name: "норма сбережений", Status: literacyOK},
			{Name: "подушка", Status: literacyBad},
			{Name: "регулярные платежи", Status: literacyInfo},
		},
	}
	text := renderFinanceLiteracyText(data)
	for _, want := range []string{
		"=== ФИНАНСОВАЯ ГРАМОТНОСТЬ ===",
		"В норме 1 из 2 оцениваемых сигналов.",
		"2026-07: доход 150000 ₽, расход 300000 ₽, сбережения -100% - разовый месяц: \"Ремонт\" 57% расходов",
		"2026-08: доход 150000 ₽, расход 120000 ₽, сбережения 20%",
		"27.04.26: 1125 ₽ (Штрафы ГАИ)",
		"Долги: открытых нет.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func TestSanitizeKeepsFinanceLiteracyWithoutArgs(t *testing.T) {
	calls := sanitizeAIToolPlan(aiToolPlan{Tools: []aiToolCall{{Name: aiToolFinanceLiteracy, Days: 90}}})
	if len(calls) != 1 || calls[0].Name != aiToolFinanceLiteracy || calls[0].Days != 0 {
		t.Fatalf("calls = %+v, want finance_literacy with no days", calls)
	}
}
