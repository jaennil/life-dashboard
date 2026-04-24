package connectors

import (
	"encoding/json"
	"testing"
)

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
