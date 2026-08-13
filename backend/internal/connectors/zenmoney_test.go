package connectors

import (
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
)

func TestZenmoneyOAuthConfigured(t *testing.T) {
	tests := []struct {
		name         string
		clientID     string
		clientSecret string
		want         bool
	}{
		{name: "configured", clientID: "client-id", clientSecret: "client-secret", want: true},
		{name: "missing client id", clientSecret: "client-secret"},
		{name: "missing client secret", clientID: "client-id"},
		{name: "blank credentials", clientID: " ", clientSecret: "\t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connector := NewZenmoney(tt.clientID, tt.clientSecret, "", nil, zerolog.Nop())
			if got := connector.OAuthConfigured(); got != tt.want {
				t.Fatalf("OAuthConfigured() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestZenmoneyAccountInBalanceDefaultsToTrue(t *testing.T) {
	var acc zenmoneyAccount
	if err := json.Unmarshal([]byte(`{"id":"1","title":"Cash","type":"cash","currency":2,"balance":100}`), &acc); err != nil {
		t.Fatalf("unmarshal account: %v", err)
	}

	if !zenmoneyAccountInBalance(&acc) {
		t.Fatalf("expected missing inBalance to default to true")
	}
}

func TestZenmoneyAccountInBalanceReadsExplicitFalse(t *testing.T) {
	var acc zenmoneyAccount
	if err := json.Unmarshal([]byte(`{"id":"1","title":"Cash","type":"cash","currency":2,"balance":100,"inBalance":false}`), &acc); err != nil {
		t.Fatalf("unmarshal account: %v", err)
	}

	if zenmoneyAccountInBalance(&acc) {
		t.Fatalf("expected explicit inBalance=false to be preserved")
	}
}

func TestZenmoneyAccountReadsArchivedFlag(t *testing.T) {
	var acc zenmoneyAccount
	if err := json.Unmarshal([]byte(`{"id":"1","title":"Split","type":"ccard","currency":2,"balance":0,"archived":true}`), &acc); err != nil {
		t.Fatalf("unmarshal account: %v", err)
	}

	if !acc.Archived {
		t.Fatalf("expected archived=true to be preserved")
	}
}

func TestZenmoneyTransactionLegs(t *testing.T) {
	t.Run("expense produces one negative leg", func(t *testing.T) {
		legs := zenmoneyTransactionLegs(&zenmoneyTransaction{
			ID: "abc", Outcome: 400, OutcomeAccount: "card", OutcomeCurrency: 2,
		})
		if len(legs) != 1 {
			t.Fatalf("legs = %d, want 1", len(legs))
		}
		if legs[0].externalID != "abc" || legs[0].amount != -400 ||
			legs[0].accountExternalID != "card" || legs[0].isTransfer {
			t.Fatalf("leg = %+v", legs[0])
		}
	})

	t.Run("income produces one positive leg", func(t *testing.T) {
		legs := zenmoneyTransactionLegs(&zenmoneyTransaction{
			ID: "abc", Income: 1000, IncomeAccount: "card", IncomeCurrency: 2,
		})
		if len(legs) != 1 || legs[0].amount != 1000 || legs[0].isTransfer {
			t.Fatalf("legs = %+v", legs)
		}
	})

	t.Run("transfer produces both sides and nets to zero", func(t *testing.T) {
		// Storing only the income side is what made every account's reconstructed
		// balance drift by the full transferred amount.
		legs := zenmoneyTransactionLegs(&zenmoneyTransaction{
			ID:     "t1",
			Income: 5000, IncomeAccount: "savings", IncomeCurrency: 2,
			Outcome: 5000, OutcomeAccount: "card", OutcomeCurrency: 2,
		})
		if len(legs) != 2 {
			t.Fatalf("legs = %d, want 2", len(legs))
		}

		var net float64
		ids := map[string]bool{}
		for _, leg := range legs {
			net += leg.amount
			ids[leg.externalID] = true
			if !leg.isTransfer {
				t.Errorf("leg %s not flagged as a transfer", leg.externalID)
			}
		}
		if net != 0 {
			t.Errorf("legs net to %v, want 0", net)
		}
		if !ids["t1:out"] || !ids["t1:in"] {
			t.Errorf("external ids = %v, want t1:out and t1:in", ids)
		}

		// The negative leg must sit on the source account and the positive on the
		// destination, otherwise both balances move the wrong way.
		for _, leg := range legs {
			if leg.amount < 0 && leg.accountExternalID != "card" {
				t.Errorf("outgoing leg on %q, want card", leg.accountExternalID)
			}
			if leg.amount > 0 && leg.accountExternalID != "savings" {
				t.Errorf("incoming leg on %q, want savings", leg.accountExternalID)
			}
		}
	})

	t.Run("cross currency transfer keeps each side's currency", func(t *testing.T) {
		legs := zenmoneyTransactionLegs(&zenmoneyTransaction{
			ID:     "t2",
			Income: 100, IncomeAccount: "eur", IncomeCurrency: 3,
			Outcome: 10000, OutcomeAccount: "rub", OutcomeCurrency: 2,
		})
		if len(legs) != 2 {
			t.Fatalf("legs = %d, want 2", len(legs))
		}
		for _, leg := range legs {
			if leg.amount < 0 && leg.currencyID != 2 {
				t.Errorf("outgoing currency = %d, want 2", leg.currencyID)
			}
			if leg.amount > 0 && leg.currencyID != 3 {
				t.Errorf("incoming currency = %d, want 3", leg.currencyID)
			}
		}
	})
}
