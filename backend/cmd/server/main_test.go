package main

import (
	"net/http"
	"testing"

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
