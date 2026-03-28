package handlers

import (
	"net/http"

	"github.com/rs/zerolog"
	"life-dashboard/internal/connectors"
)

type AuthHandler struct {
	strava    *connectors.StravaConnector
	fatsecret *connectors.FatSecretConnector
	logger    zerolog.Logger
}

func NewAuth(strava *connectors.StravaConnector, fatsecret *connectors.FatSecretConnector, logger zerolog.Logger) *AuthHandler {
	return &AuthHandler{
		strava:    strava,
		fatsecret: fatsecret,
		logger:    logger.With().Str("handler", "auth").Logger(),
	}
}

// GET /api/v1/auth/strava — redirect to Strava OAuth
func (h *AuthHandler) StravaAuthorize(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, h.strava.AuthURL(""), http.StatusTemporaryRedirect)
}

// GET /api/v1/auth/fatsecret — get request token and redirect to FatSecret OAuth 1.0 authorize
func (h *AuthHandler) FatSecretAuthorize(w http.ResponseWriter, r *http.Request) {
	if h.fatsecret == nil {
		http.Error(w, "fatsecret not configured", http.StatusServiceUnavailable)
		return
	}
	authURL, err := h.fatsecret.AuthURL(r.Context())
	if err != nil {
		h.logger.Error().Err(err).Msg("fatsecret request token failed")
		http.Error(w, "failed to get request token", http.StatusInternalServerError)
		return
	}
	h.logger.Info().Str("url", authURL).Msg("redirecting to fatsecret authorize")
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// GET /api/v1/auth/fatsecret/callback — exchange oauth_token + oauth_verifier for access token
func (h *AuthHandler) FatSecretCallback(w http.ResponseWriter, r *http.Request) {
	oauthToken := r.URL.Query().Get("oauth_token")
	oauthVerifier := r.URL.Query().Get("oauth_verifier")

	if oauthToken == "" || oauthVerifier == "" {
		h.logger.Error().
			Str("oauth_token", oauthToken).
			Str("oauth_verifier", oauthVerifier).
			Msg("fatsecret callback missing params")
		http.Error(w, "missing oauth_token or oauth_verifier", http.StatusBadRequest)
		return
	}

	if err := h.fatsecret.ExchangeToken(r.Context(), oauthToken, oauthVerifier); err != nil {
		h.logger.Error().Err(err).Msg("fatsecret token exchange failed")
		http.Error(w, "authorization failed", http.StatusInternalServerError)
		return
	}

	h.logger.Info().Msg("fatsecret authorization successful")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<html><head><meta charset="utf-8"></head><body><h2>✅ FatSecret подключён!</h2><p>Можно закрыть эту вкладку.</p></body></html>`))
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
