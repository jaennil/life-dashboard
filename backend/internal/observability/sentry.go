package observability

import (
	"net/http"
	"strconv"
	"time"

	sentry "github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"life-dashboard/internal/config"
)

func InitSentry(cfg config.SentryConfig, logger zerolog.Logger) error {
	if cfg.BackendDSN == "" {
		logger.Info().Msg("sentry backend disabled: SENTRY_BACKEND_DSN not set")
		return nil
	}

	environment := cfg.Environment
	if environment == "" {
		environment = "production"
	}

	if err := sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.BackendDSN,
		Environment:      environment,
		Release:          cfg.Release,
		AttachStacktrace: true,
		TracesSampleRate: cfg.TracesSampleRate,
	}); err != nil {
		return err
	}

	logger.Info().
		Str("environment", environment).
		Str("release", cfg.Release).
		Float64("traces_sample_rate", cfg.TracesSampleRate).
		Msg("sentry backend enabled")

	return nil
}

func FlushSentry(timeout time.Duration) bool {
	if sentry.CurrentHub().Client() == nil {
		return true
	}
	return sentry.Flush(timeout)
}

func SentryMiddleware() func(http.Handler) http.Handler {
	if sentry.CurrentHub().Client() == nil {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	handler := sentryhttp.New(sentryhttp.Options{
		Repanic:         true,
		WaitForDelivery: false,
	})

	return func(next http.Handler) http.Handler {
		return handler.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shouldSkipObservabilityRoute(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			statusCode := normalizeStatusCode(ww.Status())
			if statusCode < http.StatusInternalServerError {
				return
			}

			hub := sentry.GetHubFromContext(r.Context())
			if hub == nil {
				return
			}

			route := routePattern(r)
			requestID := chimiddleware.GetReqID(r.Context())

			hub.WithScope(func(scope *sentry.Scope) {
				scope.SetLevel(sentry.LevelError)
				scope.SetRequest(r)
				scope.SetTag("method", r.Method)
				scope.SetTag("route", route)
				scope.SetTag("status_code", strconv.Itoa(statusCode))
				scope.SetTag("request_id", requestID)
				scope.SetContext("http_response", map[string]any{
					"status_code": statusCode,
					"route":       route,
					"request_id":  requestID,
				})
				hub.CaptureMessage("http server returned 5xx")
			})
		}))
	}
}
