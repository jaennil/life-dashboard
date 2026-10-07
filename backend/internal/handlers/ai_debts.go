package handlers

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

// AIDebtPerson is one open debt with one person. A positive amount is owed to
// the user, a negative one is owed by them.
type AIDebtPerson struct {
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
}

// AIDebtsData is the debt account read the way a person reads it: what they owe
// and what they are owed, separately, and to whom.
//
// The account itself carries one signed balance, and that is what used to reach
// the model - "Debts: -118366". It is arithmetically right and tells nothing: it
// was 150 000 borrowed from one person minus 31 634 lent to three others, and a
// loan that had been fully repaid looked like two open ones because the same
// name was spelled with and without a final dot.
type AIDebtsData struct {
	IOweRub     float64        `json:"i_owe_rub"`
	OwedToMeRub float64        `json:"owed_to_me_rub"`
	People      []AIDebtPerson `json:"people,omitempty"`
}

// aiDebtSettledThreshold treats anything smaller as a repaid debt. Repayments
// rarely land on the exact kopeck, and "you owe 0.4 ₽" is noise.
const aiDebtSettledThreshold = 1.0

type aiDebtMovement struct {
	payee  string
	amount float64
}

func (h *AIHandler) buildDebts(ctx context.Context, userID string) (AIDebtsData, error) {
	rows, err := h.db.Query(ctx, `
		SELECT COALESCE(t.payee, ''), t.amount
		FROM transactions t
		JOIN accounts a ON a.id = t.account_id
		WHERE a.type = 'debt'
		  AND a.user_id = $1
		  AND COALESCE(a.archived, FALSE) = FALSE
	`, userID)
	if err != nil {
		return AIDebtsData{}, fmt.Errorf("load debt movements: %w", err)
	}
	defer rows.Close()

	movements := make([]aiDebtMovement, 0, 32)
	for rows.Next() {
		var movement aiDebtMovement
		if err := rows.Scan(&movement.payee, &movement.amount); err != nil {
			return AIDebtsData{}, fmt.Errorf("scan debt movement: %w", err)
		}
		movements = append(movements, movement)
	}
	if err := rows.Err(); err != nil {
		return AIDebtsData{}, err
	}
	return groupDebtsByPerson(movements), nil
}

// debtPersonKey decides when two spellings are the same person.
//
// Only the differences that are certainly cosmetic are folded: case, spaces and
// a trailing dot. Zenmoney itself holds "Мария Евгеньевна Д." and "Мария
// Евгеньевна Д" as two payees and the app shows them as one. "Ярослав Г." and
// "Ярослав Иванович Г" might be the same man as well, but nothing in the data
// says so, and merging two people by guess would move money between them.
func debtPersonKey(name string) string {
	key := strings.ToLower(strings.TrimSpace(name))
	key = strings.TrimRight(key, ". ")
	return strings.Join(strings.Fields(key), " ")
}

func groupDebtsByPerson(movements []aiDebtMovement) AIDebtsData {
	type person struct {
		name   string
		amount float64
	}
	byKey := make(map[string]*person)
	for _, movement := range movements {
		name := strings.TrimSpace(movement.payee)
		if name == "" {
			name = "без имени"
		}
		key := debtPersonKey(name)
		entry, ok := byKey[key]
		if !ok {
			entry = &person{name: name}
			byKey[key] = entry
		}
		// The longest spelling is shown: it is the complete one, and it is the one
		// the app shows too.
		if len([]rune(name)) > len([]rune(entry.name)) {
			entry.name = name
		}
		entry.amount += movement.amount
	}

	data := AIDebtsData{}
	for _, entry := range byKey {
		amount := math.Round(entry.amount*100) / 100
		if math.Abs(amount) < aiDebtSettledThreshold {
			continue
		}
		if amount < 0 {
			data.IOweRub += -amount
		} else {
			data.OwedToMeRub += amount
		}
		data.People = append(data.People, AIDebtPerson{Name: entry.name, Amount: amount})
	}

	// Own debts first, largest first; then what is owed to the user, largest first.
	sort.Slice(data.People, func(i, j int) bool {
		a, b := data.People[i].Amount, data.People[j].Amount
		if (a < 0) != (b < 0) {
			return a < 0
		}
		if math.Abs(a) != math.Abs(b) {
			return math.Abs(a) > math.Abs(b)
		}
		return data.People[i].Name < data.People[j].Name
	})
	return data
}

func renderDebtsText(data AIDebtsData) string {
	if len(data.People) == 0 {
		return "Долги: открытых нет.\n"
	}

	var owe, owed []string
	for _, person := range data.People {
		line := fmt.Sprintf("%s %.0f ₽", person.Name, math.Abs(person.Amount))
		if person.Amount < 0 {
			owe = append(owe, line)
		} else {
			owed = append(owed, line)
		}
	}

	var sb strings.Builder
	sb.WriteString("Долги (вне баланса, по людям):\n")
	if len(owe) > 0 {
		sb.WriteString(fmt.Sprintf("  - пользователь должен %.0f ₽: %s\n", data.IOweRub, strings.Join(owe, ", ")))
	}
	if len(owed) > 0 {
		sb.WriteString(fmt.Sprintf("  - должны пользователю %.0f ₽: %s\n", data.OwedToMeRub, strings.Join(owed, ", ")))
	}
	return sb.String()
}
