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
	unleashclient "github.com/Unleash/unleash-client-go/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"life-dashboard/internal/config"
	"life-dashboard/internal/connectors"
	"life-dashboard/internal/database"
	"life-dashboard/internal/handlers"
	authmw "life-dashboard/internal/middleware"
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

	hevy := connectors.NewHevy(pool, log.Logger)
	activeConnectors = append(activeConnectors, hevy)
	log.Info().Msg("hevy connector enabled")

	zenmoney := connectors.NewZenmoney(cfg.Connectors.Zenmoney.ClientID, cfg.Connectors.Zenmoney.ClientSecret, cfg.Connectors.Zenmoney.RedirectURI, pool, log.Logger)
	activeConnectors = append(activeConnectors, zenmoney)
	log.Info().Msg("zenmoney connector enabled")

	mfpCfg := cfg.Connectors.MFP
	mfpEnabled := mfpCfg.AccessToken != "" || (mfpCfg.Username != "" && (mfpCfg.Password != "" || mfpCfg.SessionCookie != ""))
	if mfpEnabled {
		mfp := connectors.NewMFP(mfpCfg.Username, mfpCfg.Password, mfpCfg.SessionCookie, mfpCfg.AccessToken, mfpCfg.UserID, pool, log.Logger)
		activeConnectors = append(activeConnectors, mfp)
		log.Info().Msg("myfitnesspal connector enabled")
	} else {
		log.Warn().Msg("myfitnesspal connector disabled: set MFP_ACCESS_TOKEN or MFP_USERNAME+credentials")
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

	var fatSecretConn *connectors.FatSecretConnector
	fsc := cfg.Connectors.FatSecret
	if fsc.ClientID != "" && fsc.ClientSecret != "" {
		fatSecretConn = connectors.NewFatSecret(fsc.ClientID, fsc.ClientSecret, fsc.RedirectURI, pool, log.Logger)
		activeConnectors = append(activeConnectors, fatSecretConn)
		log.Info().Msg("fatsecret connector enabled")
	} else {
		log.Warn().Msg("fatsecret connector disabled: FATSECRET_CLIENT_ID not set")
	}

	var googleCalConn *connectors.GoogleCalendarConnector
	gc := cfg.Connectors.GoogleCalendar
	if gc.ClientID != "" && gc.ClientSecret != "" {
		googleCalConn = connectors.NewGoogleCalendar(gc.ClientID, gc.ClientSecret, gc.RedirectURI, pool, log.Logger)
		activeConnectors = append(activeConnectors, googleCalConn)
		log.Info().Msg("google calendar connector enabled")
	} else {
		log.Warn().Msg("google calendar connector disabled: GOOGLE_CLIENT_ID not set")
	}

	nc := cfg.Connectors.Notion
	var notionConn *connectors.NotionConnector
	notionConn = connectors.NewNotion(nc.ClientID, nc.ClientSecret, nc.RedirectURI, pool, log.Logger)
	activeConnectors = append(activeConnectors, notionConn)
	if nc.ClientID != "" && nc.ClientSecret != "" {
		log.Info().Msg("notion connector enabled (OAuth + token)")
	} else {
		log.Info().Msg("notion connector enabled (token-only, no OAuth)")
	}

	// Scheduler
	sched := scheduler.New(log.Logger)
	for _, conn := range activeConnectors {
		connCopy := conn
		if err := sched.AddJob("0 0 */2 * * *", connCopy.Name(), func() {
			ctx := context.Background()
			rows, err := pool.Query(ctx, `SELECT id FROM users`)
			if err != nil {
				log.Error().Err(err).Str("connector", connCopy.Name()).Msg("failed to query users for scheduled sync")
				return
			}
			defer rows.Close()
			for rows.Next() {
				var userID string
				if err := rows.Scan(&userID); err != nil {
					log.Error().Err(err).Str("connector", connCopy.Name()).Msg("failed to scan user id")
					continue
				}
				if !handlers.IsEnabled(ctx, pool, connCopy.Name(), userID) {
					log.Info().Str("connector", connCopy.Name()).Str("user_id", userID).Msg("skipping disabled connector for user")
					continue
				}
				log.Info().Str("connector", connCopy.Name()).Str("user_id", userID).Msg("scheduled sync for user")
				if err := connCopy.Sync(ctx, userID); err != nil {
					log.Error().Err(err).Str("connector", connCopy.Name()).Str("user_id", userID).Msg("scheduled sync failed")
				}
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
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
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

	// Auth (public)
	usersHandler := handlers.NewUsers(pool, cfg.Auth.JWTSecret, "Life Dashboard", log.Logger)
	r.Post("/api/v1/auth/register", usersHandler.Register)
	r.Post("/api/v1/auth/login", usersHandler.Login)
	r.Post("/api/v1/auth/logout", usersHandler.Logout)

	authHandler := handlers.NewAuth(stravaConn, fatSecretConn, zenmoney, googleCalConn, notionConn, log.Logger)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(authmw.Auth(cfg.Auth.JWTSecret))

		r.Get("/api/v1/auth/me", usersHandler.Me)
		r.Get("/api/v1/auth/totp/setup", usersHandler.TOTPSetup)
		r.Post("/api/v1/auth/totp/enable", usersHandler.TOTPEnable)
		r.Post("/api/v1/auth/totp/disable", usersHandler.TOTPDisable)

		syncHandler := handlers.NewSync(pool, activeConnectors, log.Logger)
		r.Post("/api/v1/sync/{source}", syncHandler.TriggerSync)

		configuredMap := map[string]bool{
			"strava":           cfg.Connectors.Strava.ClientID != "" && cfg.Connectors.Strava.ClientSecret != "",
			"hevy":             true,
			"zenmoney":         true,
			"myfitnesspal":     mfpEnabled,
			"fatsecret":        fsc.ClientID != "" && fsc.ClientSecret != "",
			"google_calendar":  gc.ClientID != "" && gc.ClientSecret != "",
			"notion":           true,
		}
		integrationsHandler := handlers.NewIntegrations(pool, activeConnectors, configuredMap, log.Logger)
		r.Get("/api/v1/integrations", integrationsHandler.GetIntegrations)
		r.Post("/api/v1/integrations/{name}/toggle", integrationsHandler.ToggleIntegration)
		r.Post("/api/v1/integrations/myfitnesspal/token", integrationsHandler.SaveMFPToken)
		r.Post("/api/v1/integrations/{name}/token", integrationsHandler.SaveToken)

		dashboardHandler := handlers.NewDashboard(pool, log.Logger)
		r.Get("/api/v1/dashboard/summary", dashboardHandler.GetSummary)
		r.Get("/api/v1/dashboard/transactions", dashboardHandler.GetRecentTransactions)

		financeHandler := handlers.NewFinance(pool, log.Logger)
		r.Get("/api/v1/finance/monthly", financeHandler.GetMonthly)
		r.Get("/api/v1/finance/accounts", financeHandler.GetAccounts)
		r.Get("/api/v1/finance/transactions", financeHandler.GetTransactions)
		r.Get("/api/v1/finance/categories", financeHandler.GetSpendingByCategory)
		r.Get("/api/v1/finance/daily", financeHandler.GetDailyTotals)
		r.Get("/api/v1/finance/top-expenses", financeHandler.GetTopExpenses)
		r.Get("/api/v1/finance/category-list", financeHandler.GetCategoryList)

		fitnessHandler := handlers.NewFitness(pool, log.Logger)
		r.Get("/api/v1/fitness/summary", fitnessHandler.GetSummary)
		r.Get("/api/v1/fitness/weekly", fitnessHandler.GetWeeklyStats)
		r.Get("/api/v1/fitness/activities", fitnessHandler.GetActivities)
		r.Get("/api/v1/fitness/workouts", fitnessHandler.GetWorkouts)

		nutritionHandler := handlers.NewNutrition(pool, log.Logger)
		r.Get("/api/v1/nutrition/summary", nutritionHandler.GetSummary)
		r.Get("/api/v1/nutrition/daily", nutritionHandler.GetDaily)

		weatherHandler := handlers.NewWeather(cfg.Weather.Lat, cfg.Weather.Lon, cfg.Weather.City, log.Logger)
		r.Get("/api/v1/weather", weatherHandler.GetWeather)

		// Unleash feature flags
		var unleashClient *unleashclient.Client
		if cfg.Unleash.URL != "" && cfg.Unleash.APIToken != "" {
			var err error
			unleashClient, err = unleashclient.NewClient(
				unleashclient.WithUrl(cfg.Unleash.URL),
				unleashclient.WithCustomHeaders(http.Header{"Authorization": {cfg.Unleash.APIToken}}),
				unleashclient.WithAppName(cfg.Unleash.AppName),
				unleashclient.WithRefreshInterval(10),
			)
			if err != nil {
				log.Warn().Err(err).Msg("unleash client failed to initialize")
			} else {
				log.Info().Str("url", cfg.Unleash.URL).Msg("unleash client initialized")
			}
		}

		aiHandler := handlers.NewAI(pool, cfg.AI.BaseURL, cfg.AI.Model, cfg.AI.APIKey, weatherHandler, unleashClient, log.Logger)
		r.Post("/api/v1/ai/chat", aiHandler.Chat)

		if stravaConn != nil {
			r.Get("/api/v1/auth/strava", authHandler.StravaAuthorize)
			r.Get("/api/v1/auth/strava/callback", authHandler.StravaCallback)
		}
		if fatSecretConn != nil {
			r.Get("/api/v1/auth/fatsecret", authHandler.FatSecretAuthorize)
			r.Get("/api/v1/auth/fatsecret/callback", authHandler.FatSecretCallback)
			r.Get("/api/v1/auth/zenmoney", authHandler.ZenmoneyAuthorize)
			r.Get("/api/v1/auth/zenmoney/callback", authHandler.ZenmoneyCallback)
			if googleCalConn != nil {
				r.Get("/api/v1/auth/google", authHandler.GoogleAuthorize)
				r.Get("/api/v1/auth/google/callback", authHandler.GoogleCallback)
			}
		}
		if nc.ClientID != "" && nc.ClientSecret != "" {
			r.Get("/api/v1/auth/notion", authHandler.NotionAuthorize)
			r.Get("/api/v1/auth/notion/callback", authHandler.NotionCallback)
		}
		healthWebhook := handlers.NewHealthWebhook(pool, log.Logger)
		r.Get("/api/v1/health/apikey", healthWebhook.GetAPIKey)
		r.Post("/api/v1/health/apikey", healthWebhook.GenerateAPIKey)
	})

	// Public webhook (auth via api_key in body)
	healthWebhookPublic := handlers.NewHealthWebhook(pool, log.Logger)
	r.Post("/api/v1/webhook/health", healthWebhookPublic.ReceiveData)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
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
