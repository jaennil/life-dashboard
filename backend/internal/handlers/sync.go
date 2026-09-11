package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"golang.org/x/sync/singleflight"
	"life-dashboard/internal/connectors"
	"life-dashboard/internal/middleware"
	"life-dashboard/internal/observability"
	"life-dashboard/internal/syncstate"
)

var (
	errUnknownSyncSource = errors.New("unknown source")
	errSyncDisabled      = errors.New("integration is disabled")
)

type SyncHandler struct {
	db         *pgxpool.Pool
	connectors map[string]connectors.Connector
	// inFlight collapses concurrent runs of the same source for the same user
	// into one. Two callers now ask for a sync - the button and the refresh that
	// precedes an AI answer - and running both at once would mean two sets of
	// upserts racing over the same rows for no extra data.
	inFlight singleflight.Group
	logger   zerolog.Logger
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
	userID := r.Context().Value(middleware.UserIDKey).(string)

	h.logger.Info().Str("source", source).Str("user_id", userID).Msg("manual sync triggered")

	err := h.SyncNow(r.Context(), source, userID, connectors.SyncTriggerManual)
	switch {
	case errors.Is(err, errUnknownSyncSource):
		h.writeError(w, http.StatusNotFound, "unknown source: "+source)
		return
	case errors.Is(err, errSyncDisabled):
		h.writeError(w, http.StatusForbidden, errSyncDisabled.Error())
		return
	case err != nil:
		h.logger.Error().Err(err).Str("source", source).Msg("sync failed")
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "source": source})
}

// SyncNow runs one source and records the outcome.
//
// A context the caller cancelled is not a provider failure: it must not count
// towards the consecutive-failure backoff, or an impatient refresh would make a
// healthy integration look broken and eventually page about it.
func (h *SyncHandler) SyncNow(ctx context.Context, source, userID string, trigger connectors.SyncTrigger) error {
	conn, ok := h.connectors[source]
	if !ok {
		return fmt.Errorf("%w: %s", errUnknownSyncSource, source)
	}
	if !IsEnabled(ctx, h.db, source, userID) {
		return errSyncDisabled
	}

	_, err, _ := h.inFlight.Do(source+"|"+userID, func() (any, error) {
		return nil, h.runSync(ctx, conn, source, userID, trigger)
	})
	return err
}

func (h *SyncHandler) runSync(ctx context.Context, conn connectors.Connector, source, userID string, trigger connectors.SyncTrigger) error {
	err := observability.RunSync(ctx, source, string(trigger), func(ctx context.Context) error {
		return conn.Sync(connectors.WithSyncTrigger(ctx, trigger), userID)
	})
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		if recordErr := syncstate.RecordSyncFailure(ctx, h.db, source, userID, time.Now()); recordErr != nil {
			h.logger.Warn().Err(recordErr).Str("source", source).Str("user_id", userID).Msg("record sync failure")
		}
		return err
	}
	if err := syncstate.RecordSyncSuccess(ctx, h.db, source, userID, time.Now()); err != nil {
		h.logger.Warn().Err(err).Str("source", source).Str("user_id", userID).Msg("record sync success")
	}
	return nil
}

func (h *SyncHandler) writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
