package observability

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	SyncTriggerManual    = "manual"
	SyncTriggerInitial   = "initial"
	SyncTriggerScheduled = "scheduled"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "life_dashboard",
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests served by the backend.",
		},
		[]string{"method", "route", "status_code"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "life_dashboard",
			Name:      "http_request_duration_seconds",
			Help:      "Latency of HTTP requests served by the backend.",
			Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120},
		},
		[]string{"method", "route", "status_code"},
	)

	syncRunsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "life_dashboard",
			Name:      "sync_runs_total",
			Help:      "Total number of connector sync runs.",
		},
		[]string{"source", "trigger", "status"},
	)

	syncDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "life_dashboard",
			Name:      "sync_duration_seconds",
			Help:      "Duration of connector sync runs.",
			Buckets:   []float64{0.25, 0.5, 1, 2, 5, 10, 20, 30, 60, 120, 300},
		},
		[]string{"source", "trigger", "status"},
	)

	aiUpstreamRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "life_dashboard",
			Name:      "ai_upstream_requests_total",
			Help:      "Total number of upstream AI completion requests.",
		},
		[]string{"operation", "status"},
	)

	aiUpstreamDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "life_dashboard",
			Name:      "ai_upstream_duration_seconds",
			Help:      "Duration of upstream AI completion requests.",
			Buckets:   []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 30, 60, 120, 180},
		},
		[]string{"operation", "status"},
	)
)

func HTTPMetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldSkipObservabilityRoute(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		statusCode := normalizeStatusCode(ww.Status())
		route := routePattern(r)
		labels := prometheus.Labels{
			"method":      r.Method,
			"route":       route,
			"status_code": strconv.Itoa(statusCode),
		}

		httpRequestsTotal.With(labels).Inc()
		httpRequestDuration.With(labels).Observe(time.Since(start).Seconds())
	})
}

func RunSync(ctx context.Context, source, trigger string, fn func(context.Context) error) error {
	start := time.Now()
	err := fn(ctx)
	status := "success"
	if err != nil {
		status = "error"
	}

	labels := prometheus.Labels{
		"source":  source,
		"trigger": trigger,
		"status":  status,
	}
	syncRunsTotal.With(labels).Inc()
	syncDuration.With(labels).Observe(time.Since(start).Seconds())

	return err
}

func ObserveAIUpstream(operation, status string, duration time.Duration) {
	if operation == "" {
		operation = "unknown"
	}
	if status == "" {
		status = "unknown"
	}

	labels := prometheus.Labels{
		"operation": operation,
		"status":    status,
	}
	aiUpstreamRequestsTotal.With(labels).Inc()
	aiUpstreamDuration.With(labels).Observe(duration.Seconds())
}

func shouldSkipObservabilityRoute(path string) bool {
	return path == "/health" || path == "/metrics"
}

func routePattern(r *http.Request) string {
	rc := chi.RouteContext(r.Context())
	if rc == nil {
		return "unmatched"
	}
	pattern := rc.RoutePattern()
	if pattern == "" {
		return "unmatched"
	}
	return pattern
}

func normalizeStatusCode(statusCode int) int {
	if statusCode == 0 {
		return http.StatusOK
	}
	return statusCode
}
