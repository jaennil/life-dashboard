package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"life-dashboard/internal/connectors"
	"life-dashboard/internal/middleware"
)

type SyncHandler struct {
	db         *pgxpool.Pool
	connectors map[string]connectors.Connector
	logger     zerolog.Logger
}

func NewSync(db *pgxpool.Pool, conns []connectors.Connector, logger zerolog.Logger) *SyncHandler {
	m := make(map[string]connectors.Connector, len(conns))
	for _, c := range conns {
		m[c.Name()] = c
	}
	return &SyncHandler{db: db, connectors: m, logger: logger.With().Str("handler", "sync").Logger()}
}

// POST /api/v1/sync/{source}
func (h *SyncHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	source := r.PathValue("source")

	conn, ok := h.connectors[source]
	if !ok {
		h.writeError(w, http.StatusNotFound, "unknown source: "+source)
		return
	}

	if !IsEnabled(r.Context(), h.db, source) {
		h.writeError(w, http.StatusForbidden, "integration is disabled")
		return
	}

	userID := r.Context().Value(middleware.UserIDKey).(string)
	h.logger.Info().Str("source", source).Str("user_id", userID).Msg("manual sync triggered")

	if err := conn.Sync(r.Context(), userID); err != nil {
		h.logger.Error().Err(err).Str("source", source).Msg("sync failed")
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "source": source})
}

func (h *SyncHandler) writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
