package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"life-dashboard/internal/config"
	"life-dashboard/internal/connectors"
	"life-dashboard/internal/database"
	"life-dashboard/internal/handlers"
	"life-dashboard/internal/scheduler"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	level, err := zerolog.ParseLevel(cfg.Log.Level)
	if err != nil {
		level = zerolog.DebugLevel
	}
	zerolog.SetGlobalLevel(level)

	if cfg.Server.Env == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	log.Info().Str("env", cfg.Server.Env).Str("log_level", level.String()).Msg("starting life-dashboard")

	ctx := context.Background()

	pool, err := database.New(ctx, cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()
	log.Info().Msg("database connected")

	migrateURL := "pgx5://" + cfg.Database.URL[len("postgres://"):]
	m, err := migrate.New("file://migrations", migrateURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to init migrations")
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatal().Err(err).Msg("failed to run migrations")
	}
	log.Info().Msg("migrations applied")

	// Connectors
	var activeConnectors []connectors.Connector

	if cfg.Connectors.Hevy.APIKey != "" {
		hevy := connectors.NewHevy(cfg.Connectors.Hevy.APIKey, pool, log.Logger)
		activeConnectors = append(activeConnectors, hevy)
		log.Info().Msg("hevy connector enabled")
	} else {
		log.Warn().Msg("hevy connector disabled: HEVY_API_KEY not set")
	}

	if cfg.Connectors.Zenmoney.Token != "" {
		zenmoney := connectors.NewZenmoney(cfg.Connectors.Zenmoney.Token, pool, log.Logger)
		activeConnectors = append(activeConnectors, zenmoney)
		log.Info().Msg("zenmoney connector enabled")
	} else {
		log.Warn().Msg("zenmoney connector disabled: ZENMONEY_TOKEN not set")
	}

	var stravaConn *connectors.StravaConnector
	sc := cfg.Connectors.Strava
	if sc.ClientID != "" && sc.ClientSecret != "" {
		stravaConn = connectors.NewStrava(sc.ClientID, sc.ClientSecret, sc.RedirectURI, pool, log.Logger)
		activeConnectors = append(activeConnectors, stravaConn)
		log.Info().Msg("strava connector enabled")
	} else {
		log.Warn().Msg("strava connector disabled: STRAVA_CLIENT_ID or STRAVA_CLIENT_SECRET not set")
	}

	// Scheduler
	sched := scheduler.New(log.Logger)
	for _, conn := range activeConnectors {
		if err := sched.AddJob("0 0 */2 * * *", conn.Name(), func() {
			if err := conn.Sync(context.Background()); err != nil {
				log.Error().Err(err).Str("connector", conn.Name()).Msg("scheduled sync failed")
			}
		}); err != nil {
			log.Fatal().Err(err).Str("connector", conn.Name()).Msg("failed to register sync job")
		}
	}
	sched.Start()
	defer sched.Stop()

	// Router
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if req.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, req)
		})
	})
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, req.ProtoMajor)
			next.ServeHTTP(ww, req)
			log.Info().
				Str("method", req.Method).
				Str("path", req.URL.Path).
				Int("status", ww.Status()).
				Dur("duration", time.Since(start)).
				Str("request_id", middleware.GetReqID(req.Context())).
				Msg("request")
		})
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			log.Error().Err(err).Msg("health check failed")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"unhealthy"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	syncHandler := handlers.NewSync(activeConnectors, log.Logger)
	r.Post("/api/v1/sync/{source}", syncHandler.TriggerSync)

	dashboardHandler := handlers.NewDashboard(pool, log.Logger)
	r.Get("/api/v1/dashboard/summary", dashboardHandler.GetSummary)
	r.Get("/api/v1/dashboard/transactions", dashboardHandler.GetRecentTransactions)

	aiHandler := handlers.NewAI(pool, cfg.AI.BaseURL, cfg.AI.Model, cfg.AI.APIKey, log.Logger)
	r.Post("/api/v1/ai/chat", aiHandler.Chat)

	if stravaConn != nil {
		authHandler := handlers.NewAuth(stravaConn, log.Logger)
		r.Get("/api/v1/auth/strava", authHandler.StravaAuthorize)
		r.Get("/api/v1/auth/strava/callback", authHandler.StravaCallback)
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info().Int("port", cfg.Server.Port).Msg("server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	<-quit
	log.Info().Msg("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatal().Err(err).Msg("forced shutdown")
	}
	log.Info().Msg("server stopped")
}
