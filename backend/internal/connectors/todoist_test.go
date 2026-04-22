package connectors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestTodoistAuthURLIncludesStateAndRedirectURI(t *testing.T) {
	connector := NewTodoist("client-id", "client-secret", "https://lifedash.dubrovskih.ru/api/v1/auth/todoist/callback", nil, zerolog.Nop())

	authURL := connector.AuthURL("state-123")
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}

	query := parsed.Query()
	if got := query.Get("client_id"); got != "client-id" {
		t.Fatalf("unexpected client_id %q", got)
	}
	if got := query.Get("scope"); got != todoistOAuthScope {
		t.Fatalf("unexpected scope %q", got)
	}
	if got := query.Get("state"); got != "state-123" {
		t.Fatalf("unexpected state %q", got)
	}
	if got := query.Get("redirect_uri"); got != "https://lifedash.dubrovskih.ru/api/v1/auth/todoist/callback" {
		t.Fatalf("unexpected redirect_uri %q", got)
	}
}

func TestTodoistIsRecurringHabit(t *testing.T) {
	item := todoistItem{ID: "1", Content: "Brush teeth", Due: &todoistDue{Recurring: true}}
	if !todoistIsRecurringHabit(item) {
		t.Fatalf("expected recurring task to be treated as habit")
	}

	item = todoistItem{ID: "2", Content: "Wash face", Due: &todoistDue{IsRecurring: true}}
	if !todoistIsRecurringHabit(item) {
		t.Fatalf("expected api v1 is_recurring task to be treated as habit")
	}

	item.Checked = true
	if todoistIsRecurringHabit(item) {
		t.Fatalf("expected completed task to be excluded from active habits")
	}
}

func TestTodoistTimeOfDay(t *testing.T) {
	result := todoistTimeOfDay(&todoistDue{DateTime: "2026-04-16T06:30:00Z"})
	if len(result) != 1 || result[0] != "06:30" {
		t.Fatalf("unexpected time_of_day %#v", result)
	}

	result = todoistTimeOfDay(&todoistDue{Date: "2026-04-16T06:30:00.000000"})
	if len(result) != 1 || result[0] != "06:30" {
		t.Fatalf("unexpected floating time_of_day %#v", result)
	}
}

func TestTodoistSyncStart(t *testing.T) {
	connector := NewTodoist("", "", "", nil, zerolog.Nop())
	now := time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC)

	start := connector.syncStart(time.Time{}, now)
	if start.Format("2006-01-02") != "2026-02-16" {
		t.Fatalf("unexpected initial lookback start %s", start.Format("2006-01-02"))
	}

	lastSync := time.Date(2026, 4, 14, 18, 0, 0, 0, time.UTC)
	start = connector.syncStart(lastSync, now)
	if start.Format("2006-01-02") != "2026-03-31" {
		t.Fatalf("unexpected incremental lookback start %s", start.Format("2006-01-02"))
	}
}

func TestTodoistDoTreatsCompletedArchive503AsTemporaryUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream temporary failure", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	connector := NewTodoist("", "", "", nil, zerolog.Nop())
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	_, err = connector.do(req, true)
	if err == nil {
		t.Fatalf("expected error")
	}
	if err != errTodoistCompletedArchiveTemporaryUnavailable {
		t.Fatalf("expected temporary unavailable error, got %v", err)
	}
}
