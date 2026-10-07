package handlers

import (
	"strings"
	"testing"
)

// realDebtMovements is the debt account as it stood on 07.10.2026, verified
// line by line against the Zenmoney app: you owe 172 000, you are owed 16 634.
var realDebtMovements = []aiDebtMovement{
	{"Елена Ивановна Р.", -50000}, {"Елена Ивановна Р.", -100000},
	// One woman, spelled two ways - Zenmoney stores both, the app shows one.
	{"Мария Евгеньевна Д", 6000}, {"Мария Евгеньевна Д", 15000},
	{"Мария Евгеньевна Д", -15000}, {"Мария Евгеньевна Д", -22000},
	{"Мария Евгеньевна Д.", -6000}, {"Мария Евгеньевна Д.", 16000},
	{"Мария Евгеньевна Д.", -16000}, {"Мария Евгеньевна Д.", -12000},
	{"Мария Евгеньевна Д.", 12000},
	{"Евгений Д.", 20000}, {"Евгений Д.", 5000}, {"Евгений Д.", 5000}, {"Евгений Д.", -15000},
	{"Виталий Фомин", 984},
	{"Ярослав Г.", 1000}, {"Ярослав Г.", -1000}, {"Ярослав Г.", 150}, {"Ярослав Г.", 500},
	// Repaid in full.
	{"Никита Ч.", -500}, {"Никита Ч.", 500},
}

func TestDebtsMatchTheZenmoneyApp(t *testing.T) {
	data := groupDebtsByPerson(realDebtMovements)

	if data.IOweRub != 172000 {
		t.Errorf("you owe %.0f, the app says 172 000", data.IOweRub)
	}
	if data.OwedToMeRub != 16634 {
		t.Errorf("you are owed %.0f, the app says 16 634", data.OwedToMeRub)
	}

	want := map[string]float64{
		"Елена Ивановна Р.":   -150000,
		"Мария Евгеньевна Д.": -22000,
		"Евгений Д.":          15000,
		"Виталий Фомин":       984,
		"Ярослав Г.":          650,
	}
	if len(data.People) != len(want) {
		t.Fatalf("%d people, want %d: %+v", len(data.People), len(want), data.People)
	}
	for _, person := range data.People {
		if amount, ok := want[person.Name]; !ok || amount != person.Amount {
			t.Errorf("%s: %.0f, want %.0f", person.Name, person.Amount, amount)
		}
	}
}

func TestOneNameSpelledTwoWaysIsOnePerson(t *testing.T) {
	data := groupDebtsByPerson([]aiDebtMovement{
		{"Мария Евгеньевна Д", -6000},
		{"Мария Евгеньевна Д.", -16000},
	})
	if len(data.People) != 1 {
		t.Fatalf("one person became %d: %+v", len(data.People), data.People)
	}
	// The complete spelling is the one shown.
	if data.People[0].Name != "Мария Евгеньевна Д." || data.People[0].Amount != -22000 {
		t.Errorf("got %+v", data.People[0])
	}
}

func TestDifferentMiddleNamesStayApart(t *testing.T) {
	// Possibly the same man, but nothing in the data says so. Merging by guess
	// would move money from one person to another.
	data := groupDebtsByPerson([]aiDebtMovement{
		{"Ярослав Г.", 650},
		{"Ярослав Иванович Г", 300},
	})
	if len(data.People) != 2 {
		t.Errorf("guessed a merge: %+v", data.People)
	}
}

func TestDebtsRenderBothSidesNotOneSum(t *testing.T) {
	text := renderDebtsText(groupDebtsByPerson(realDebtMovements))

	for _, want := range []string{
		"пользователь должен 172000 ₽", "Елена Ивановна Р. 150000 ₽",
		"должны пользователю 16634 ₽", "Евгений Д. 15000 ₽",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	// The net figure is what misled: it must not appear at all.
	if strings.Contains(text, "155366") {
		t.Errorf("the net balance leaked into the text:\n%s", text)
	}
	// A repaid debt is not news.
	if strings.Contains(text, "Никита") {
		t.Errorf("a settled debt is listed:\n%s", text)
	}
}

func TestNoOpenDebtsSaysSo(t *testing.T) {
	text := renderDebtsText(groupDebtsByPerson([]aiDebtMovement{{"Никита Ч.", -500}, {"Никита Ч.", 500}}))
	if !strings.Contains(text, "открытых нет") {
		t.Errorf("text = %q", text)
	}
}
