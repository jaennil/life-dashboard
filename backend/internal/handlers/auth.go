package handlers

import (
	"net/http"

	"github.com/rs/zerolog"
	"life-dashboard/internal/connectors"
)

type AuthHandler struct {
	strava *connectors.StravaConnector
	logger zerolog.Logger
}

func NewAuth(strava *connectors.StravaConnector, logger zerolog.Logger) *AuthHandler {
	return &AuthHandler{
		strava: strava,
		logger: logger.With().Str("handler", "auth").Logger(),
	}
}

// GET /api/v1/auth/strava — redirect to Strava OAuth
func (h *AuthHandler) StravaAuthorize(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, h.strava.AuthURL(""), http.StatusTemporaryRedirect)
}

// GET /api/v1/auth/strava/callback — exchange code for tokens
func (h *AuthHandler) StravaCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		h.logger.Error().Str("error", r.URL.Query().Get("error")).Msg("strava callback missing code")
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	if err := h.strava.ExchangeCode(r.Context(), code); err != nil {
		h.logger.Error().Err(err).Msg("strava token exchange failed")
		http.Error(w, "authorization failed", http.StatusInternalServerError)
		return
	}

	h.logger.Info().Msg("strava authorization successful")
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"authorized","source":"strava"}`))
}
