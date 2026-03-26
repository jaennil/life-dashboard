package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog"
	"life-dashboard/internal/connectors"
)

type SyncHandler struct {
	connectors map[string]connectors.Connector
	logger     zerolog.Logger
}

func NewSync(conns []connectors.Connector, logger zerolog.Logger) *SyncHandler {
	m := make(map[string]connectors.Connector, len(conns))
	for _, c := range conns {
		m[c.Name()] = c
	}
	return &SyncHandler{connectors: m, logger: logger.With().Str("handler", "sync").Logger()}
}

// POST /api/v1/sync/{source}
func (h *SyncHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	source := r.PathValue("source")

	conn, ok := h.connectors[source]
	if !ok {
		h.writeError(w, http.StatusNotFound, "unknown source: "+source)
		return
	}

	h.logger.Info().Str("source", source).Msg("manual sync triggered")

	if err := conn.Sync(r.Context()); err != nil {
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
