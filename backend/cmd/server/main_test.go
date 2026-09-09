package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"life-dashboard/internal/handlers"
)

func TestRegisterOAuthRoutesProvidersAreIndependent(t *testing.T) {
	tests := []struct {
		name      string
		available oauthRouteAvailability
		routes    [2]string
	}{
		{
			name:      "strava",
			available: oauthRouteAvailability{strava: true},
			routes:    [2]string{"/api/v1/auth/strava", "/api/v1/auth/strava/callback"},
		},
		{
			name:      "fatsecret",
			available: oauthRouteAvailability{fatSecret: true},
			routes:    [2]string{"/api/v1/auth/fatsecret", "/api/v1/auth/fatsecret/callback"},
		},
		{
			name:      "zenmoney without fatsecret",
			available: oauthRouteAvailability{zenmoney: true},
			routes:    [2]string{"/api/v1/auth/zenmoney", "/api/v1/auth/zenmoney/callback"},
		},
		{
			name:      "google calendar without fatsecret",
			available: oauthRouteAvailability{googleCalendar: true},
			routes:    [2]string{"/api/v1/auth/google", "/api/v1/auth/google/callback"},
		},
		{
			name:      "notion",
			available: oauthRouteAvailability{notion: true},
			routes:    [2]string{"/api/v1/auth/notion", "/api/v1/auth/notion/callback"},
		},
		{
			name:      "todoist",
			available: oauthRouteAvailability{todoist: true},
			routes:    [2]string{"/api/v1/auth/todoist", "/api/v1/auth/todoist/callback"},
		},
	}

	authHandler := handlers.NewAuth(nil, nil, nil, nil, nil, nil, zerolog.Nop())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := chi.NewRouter()
			registerOAuthRoutes(router, authHandler, tt.available)

			routes := registeredGETRoutes(t, router)
			if len(routes) != len(tt.routes) {
				t.Fatalf("registered %d routes, want %d: %v", len(routes), len(tt.routes), routes)
			}
			for _, route := range tt.routes {
				if !routes[route] {
					t.Errorf("route %q was not registered", route)
				}
			}
		})
	}
}

func TestRegisterOAuthRoutesSkipsUnavailableProviders(t *testing.T) {
	router := chi.NewRouter()
	authHandler := handlers.NewAuth(nil, nil, nil, nil, nil, nil, zerolog.Nop())
	registerOAuthRoutes(router, authHandler, oauthRouteAvailability{})

	if routes := registeredGETRoutes(t, router); len(routes) != 0 {
		t.Fatalf("registered unavailable OAuth routes: %v", routes)
	}
}

func registeredGETRoutes(t *testing.T, router chi.Routes) map[string]bool {
	t.Helper()
	routes := make(map[string]bool)
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodGet {
			routes[route] = true
		}
		return nil
	}); err != nil {
		t.Fatalf("walk routes: %v", err)
	}
	return routes
}

func TestConnectorSyncSpecSpreadsTheHerd(t *testing.T) {
	// Every connector keeps its quarter-hour cadence, but no two share a minute
	// until there are more than fifteen of them.
	seen := map[string]bool{}
	for index := 0; index < 12; index++ {
		spec := connectorSyncSpec(index)
		if seen[spec] {
			t.Fatalf("connector %d reuses spec %q", index, spec)
		}
		seen[spec] = true
	}

	if got := connectorSyncSpec(0); got != "0 0,15,30,45 * * * *" {
		t.Fatalf("first connector spec = %q", got)
	}
	if got := connectorSyncSpec(3); got != "0 3,18,33,48 * * * *" {
		t.Fatalf("fourth connector spec = %q", got)
	}
	// The cycle wraps rather than producing a minute past 59.
	if got := connectorSyncSpec(15); got != connectorSyncSpec(0) {
		t.Fatalf("spec %q did not wrap onto the first slot", got)
	}
}

func TestRetryStartupSucceedsAfterTheDatabaseComesBack(t *testing.T) {
	attempts := 0
	err := retryStartup(context.Background(), 200*time.Millisecond, time.Millisecond, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("connection refused")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected the third attempt to succeed, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("unexpected attempt count %d", attempts)
	}
}

func TestRetryStartupGivesUpWithTheLastError(t *testing.T) {
	attempts := 0
	want := errors.New("still down")
	err := retryStartup(context.Background(), 20*time.Millisecond, 5*time.Millisecond, func() error {
		attempts++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected the last error back, got %v", err)
	}
	if attempts < 2 {
		t.Fatalf("expected several attempts inside the budget, got %d", attempts)
	}
}

func TestRetryStartupStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := retryStartup(ctx, time.Minute, time.Millisecond, func() error {
		return errors.New("down")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the cancellation to win, got %v", err)
	}
}
