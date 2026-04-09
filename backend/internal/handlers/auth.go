package handlers

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"life-dashboard/internal/connectors"
	"life-dashboard/internal/middleware"
)

type AuthHandler struct {
	strava         *connectors.StravaConnector
	fatsecret      *connectors.FatSecretConnector
	zenmoney       *connectors.ZenmoneyConnector
	googleCalendar *connectors.GoogleCalendarConnector
	notion         *connectors.NotionConnector
	logger         zerolog.Logger
}

func NewAuth(strava *connectors.StravaConnector, fatsecret *connectors.FatSecretConnector, zenmoney *connectors.ZenmoneyConnector, googleCalendar *connectors.GoogleCalendarConnector, notion *connectors.NotionConnector, logger zerolog.Logger) *AuthHandler {
	return &AuthHandler{
		strava:         strava,
		fatsecret:      fatsecret,
		zenmoney:       zenmoney,
		googleCalendar: googleCalendar,
		notion:         notion,
		logger:         logger.With().Str("handler", "auth").Logger(),
	}
}

// GET /api/v1/auth/strava — redirect to Strava OAuth
func (h *AuthHandler) StravaAuthorize(w http.ResponseWriter, r *http.Request) {
	state, err := issueOAuthState(w, r, "strava")
	if err != nil {
		h.logger.Error().Err(err).Msg("issue strava oauth state")
		http.Error(w, "failed to start authorization", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, h.strava.AuthURL(state), http.StatusTemporaryRedirect)
}

// GET /api/v1/auth/fatsecret — get request token and redirect to FatSecret OAuth 1.0 authorize
func (h *AuthHandler) FatSecretAuthorize(w http.ResponseWriter, r *http.Request) {
	if h.fatsecret == nil {
		http.Error(w, "fatsecret not configured", http.StatusServiceUnavailable)
		return
	}
	userID := r.Context().Value(middleware.UserIDKey).(string)
	authURL, err := h.fatsecret.AuthURL(r.Context(), userID)
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

	fsUserID := r.Context().Value(middleware.UserIDKey).(string)
	h.logger.Info().Str("user_id", fsUserID).Msg("fatsecret authorization successful, starting initial sync")
	go func() {
		if err := h.fatsecret.Sync(context.Background(), fsUserID); err != nil {
			h.logger.Error().Err(err).Msg("fatsecret initial sync failed")
		} else {
			h.logger.Info().Msg("fatsecret initial sync complete")
		}
	}()

	redirectToSettingsAfterSync(w, r, "fatsecret")
}

// GET /api/v1/auth/strava/callback — exchange code for tokens
func (h *AuthHandler) StravaCallback(w http.ResponseWriter, r *http.Request) {
	if err := verifyOAuthState(w, r, "strava"); err != nil {
		h.logger.Warn().Err(err).Msg("strava callback invalid state")
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		h.logger.Error().Str("error", r.URL.Query().Get("error")).Msg("strava callback missing code")
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	stravaUserID := r.Context().Value(middleware.UserIDKey).(string)
	if err := h.strava.ExchangeCode(r.Context(), stravaUserID, code); err != nil {
		h.logger.Error().Err(err).Msg("strava token exchange failed")
		http.Error(w, "authorization failed", http.StatusInternalServerError)
		return
	}
	h.logger.Info().Str("user_id", stravaUserID).Msg("strava authorization successful, starting initial sync")
	go func() {
		if err := h.strava.Sync(context.Background(), stravaUserID); err != nil {
			h.logger.Error().Err(err).Msg("strava initial sync failed")
		} else {
			h.logger.Info().Msg("strava initial sync complete")
		}
	}()

	redirectToSettingsAfterSync(w, r, "strava")
}

// GET /api/v1/auth/zenmoney — redirect to ZenMoney OAuth
func (h *AuthHandler) ZenmoneyAuthorize(w http.ResponseWriter, r *http.Request) {
	if h.zenmoney == nil {
		http.Error(w, "zenmoney not configured", http.StatusServiceUnavailable)
		return
	}
	state, err := issueOAuthState(w, r, "zenmoney")
	if err != nil {
		h.logger.Error().Err(err).Msg("issue zenmoney oauth state")
		http.Error(w, "failed to start authorization", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, h.zenmoney.AuthURL(state), http.StatusTemporaryRedirect)
}

// GET /api/v1/auth/zenmoney/callback — exchange code for tokens
func (h *AuthHandler) ZenmoneyCallback(w http.ResponseWriter, r *http.Request) {
	if err := verifyOAuthState(w, r, "zenmoney"); err != nil {
		h.logger.Warn().Err(err).Msg("zenmoney callback invalid state")
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		h.logger.Error().Msg("zenmoney callback missing code")
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	zmUserID := r.Context().Value(middleware.UserIDKey).(string)
	if err := h.zenmoney.ExchangeCode(r.Context(), zmUserID, code); err != nil {
		h.logger.Error().Err(err).Msg("zenmoney token exchange failed")
		http.Error(w, "authorization failed", http.StatusInternalServerError)
		return
	}

	h.logger.Info().Str("user_id", zmUserID).Msg("zenmoney authorization successful, starting initial sync")
	go func() {
		if err := h.zenmoney.Sync(context.Background(), zmUserID); err != nil {
			h.logger.Error().Err(err).Msg("zenmoney initial sync failed")
		} else {
			h.logger.Info().Msg("zenmoney initial sync complete")
		}
	}()

	redirectToSettingsAfterSync(w, r, "zenmoney")
}

// GET /api/v1/auth/notion — redirect to Notion OAuth
func (h *AuthHandler) NotionAuthorize(w http.ResponseWriter, r *http.Request) {
	if h.notion == nil {
		http.Error(w, "notion not configured", http.StatusServiceUnavailable)
		return
	}
	state, err := issueOAuthState(w, r, "notion")
	if err != nil {
		h.logger.Error().Err(err).Msg("issue notion oauth state")
		http.Error(w, "failed to start authorization", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, h.notion.AuthURL(state), http.StatusTemporaryRedirect)
}

// GET /api/v1/auth/notion/callback — exchange code for tokens
func (h *AuthHandler) NotionCallback(w http.ResponseWriter, r *http.Request) {
	if err := verifyOAuthState(w, r, "notion"); err != nil {
		h.logger.Warn().Err(err).Msg("notion callback invalid state")
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		h.logger.Error().Str("error", r.URL.Query().Get("error")).Msg("notion callback missing code")
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	notionUserID := r.Context().Value(middleware.UserIDKey).(string)
	if err := h.notion.ExchangeCode(r.Context(), notionUserID, code); err != nil {
		h.logger.Error().Err(err).Msg("notion token exchange failed")
		http.Error(w, "authorization failed", http.StatusInternalServerError)
		return
	}

	h.logger.Info().Str("user_id", notionUserID).Msg("notion authorized, starting sync")
	go func() {
		if err := h.notion.Sync(context.Background(), notionUserID); err != nil {
			h.logger.Error().Err(err).Msg("notion initial sync failed")
		}
	}()

	redirectToSettingsAfterSync(w, r, "notion")
}

// GET /api/v1/auth/google — redirect to Google OAuth
func (h *AuthHandler) GoogleAuthorize(w http.ResponseWriter, r *http.Request) {
	if h.googleCalendar == nil {
		http.Error(w, "google calendar not configured", http.StatusServiceUnavailable)
		return
	}
	state, err := issueOAuthState(w, r, "google_calendar")
	if err != nil {
		h.logger.Error().Err(err).Msg("issue google oauth state")
		http.Error(w, "failed to start authorization", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, h.googleCalendar.AuthURL(state), http.StatusTemporaryRedirect)
}

// GET /api/v1/auth/google/callback — exchange code for tokens
func (h *AuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	if err := verifyOAuthState(w, r, "google_calendar"); err != nil {
		h.logger.Warn().Err(err).Msg("google callback invalid state")
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	gcUserID := r.Context().Value(middleware.UserIDKey).(string)
	if err := h.googleCalendar.ExchangeCode(r.Context(), gcUserID, code); err != nil {
		h.logger.Error().Err(err).Msg("google token exchange failed")
		http.Error(w, "authorization failed", http.StatusInternalServerError)
		return
	}

	h.logger.Info().Str("user_id", gcUserID).Msg("google calendar authorized, starting sync")
	go func() {
		if err := h.googleCalendar.Sync(context.Background(), gcUserID); err != nil {
			h.logger.Error().Err(err).Msg("google calendar initial sync failed")
		}
	}()

	redirectToSettingsAfterSync(w, r, "google_calendar")
}

func issueOAuthState(w http.ResponseWriter, r *http.Request, source string) (string, error) {
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", fmt.Errorf("generate oauth state: %w", err)
	}

	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName(source),
		Value:    state,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		MaxAge:   600,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
	})
	return state, nil
}

func verifyOAuthState(w http.ResponseWriter, r *http.Request, source string) error {
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie(oauthStateCookieName(source))
	clearOAuthState(w, r, source)
	if err != nil || state == "" {
		return fmt.Errorf("missing oauth state")
	}
	if subtle.ConstantTimeCompare([]byte(state), []byte(cookie.Value)) != 1 {
		return fmt.Errorf("oauth state mismatch")
	}
	return nil
}

func clearOAuthState(w http.ResponseWriter, r *http.Request, source string) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName(source),
		Value:    "",
		Path:     "/api/v1/auth",
		HttpOnly: true,
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
	})
}

func oauthStateCookieName(source string) string {
	return "oauth_state_" + source
}

func isSecureRequest(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func redirectToSettingsAfterSync(w http.ResponseWriter, r *http.Request, source string) {
	params := url.Values{
		"sync":       {source},
		"started_at": {time.Now().UTC().Format(time.RFC3339Nano)},
	}
	http.Redirect(w, r, "/settings?"+params.Encode(), http.StatusFound)
}
