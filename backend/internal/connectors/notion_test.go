package connectors

import (
	"net/url"
	"testing"

	"github.com/rs/zerolog"
)

func TestNotionAuthURLIncludesState(t *testing.T) {
	connector := NewNotion("client-id", "client-secret", "https://lifedash.dubrovskih.ru/api/v1/auth/notion/callback", nil, zerolog.Nop())

	authURL := connector.AuthURL("state-123")
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}

	query := parsed.Query()
	if got := query.Get("state"); got != "state-123" {
		t.Fatalf("expected state query param to be propagated, got %q", got)
	}
	if got := query.Get("redirect_uri"); got != "https://lifedash.dubrovskih.ru/api/v1/auth/notion/callback" {
		t.Fatalf("unexpected redirect_uri %q", got)
	}
	if got := query.Get("owner"); got != "user" {
		t.Fatalf("unexpected owner %q", got)
	}
}
