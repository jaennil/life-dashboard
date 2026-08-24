package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp/totp"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	"life-dashboard/internal/middleware"
	"life-dashboard/internal/session"
	"life-dashboard/internal/syncstate"
)

type UsersHandler struct {
	db        *pgxpool.Pool
	jwtSecret string
	appName   string
	logger    zerolog.Logger
}

func NewUsers(db *pgxpool.Pool, jwtSecret, appName string, logger zerolog.Logger) *UsersHandler {
	return &UsersHandler{
		db:        db,
		jwtSecret: jwtSecret,
		appName:   appName,
		logger:    logger.With().Str("handler", "users").Logger(),
	}
}

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code"`
}

type userResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	TOTPEnabled bool   `json:"totp_enabled"`
}

func (h *UsersHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 || len(req.Password) < 6 {
		http.Error(w, "username min 3 chars, password min 6 chars", http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		h.logger.Error().Err(err).Msg("bcrypt")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var id string
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING id
	`, req.Username, string(hash)).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			http.Error(w, "username already taken", http.StatusConflict)
			return
		}
		h.logger.Error().Err(err).Msg("insert user")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.logger.Info().Str("username", req.Username).Msg("user registered")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(userResponse{ID: id, Username: req.Username})
}

func (h *UsersHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var id, username, passwordHash string
	var totpSecret *string
	var totpEnabled bool

	err := h.db.QueryRow(r.Context(), `
		SELECT id, username, password_hash, totp_secret, totp_enabled
		FROM users WHERE username = $1
	`, strings.TrimSpace(req.Username)).Scan(&id, &username, &passwordHash, &totpSecret, &totpEnabled)
	if err != nil {
		h.logger.Warn().Str("username", req.Username).Msg("user not found")
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		h.logger.Warn().Str("username", req.Username).Msg("wrong password")
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	// TOTP check
	if totpEnabled && totpSecret != nil {
		if req.TOTPCode == "" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"needs_totp": true})
			return
		}
		if !totp.Validate(req.TOTPCode, *totpSecret) {
			h.logger.Warn().Str("username", req.Username).Msg("invalid totp")
			http.Error(w, "invalid totp code", http.StatusUnauthorized)
			return
		}
	}

	h.issueToken(r.Context(), w, r, id, username, totpEnabled)
}

func (h *UsersHandler) issueToken(ctx context.Context, w http.ResponseWriter, r *http.Request, id, username string, totpEnabled bool) {
	if err := syncstate.ForceUserActivity(ctx, h.db, id); err != nil {
		h.logger.Warn().Err(err).Str("user_id", id).Msg("touch user activity on login")
	}

	if err := session.Issue(w, r, h.jwtSecret, id, username, time.Now()); err != nil {
		h.logger.Error().Err(err).Msg("sign jwt")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.logger.Info().Str("username", username).Msg("user logged in")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(userResponse{ID: id, Username: username, TOTPEnabled: totpEnabled})
}

func (h *UsersHandler) Logout(w http.ResponseWriter, r *http.Request) {
	session.Clear(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (h *UsersHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var id, username string
	var totpEnabled bool
	err := h.db.QueryRow(r.Context(), `
		SELECT id, username, totp_enabled FROM users WHERE id = $1
	`, userID).Scan(&id, &username, &totpEnabled)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(userResponse{ID: id, Username: username, TOTPEnabled: totpEnabled})
}

func (h *UsersHandler) TOTPSetup(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var username string
	h.db.QueryRow(r.Context(), `SELECT username FROM users WHERE id = $1`, userID).Scan(&username)

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      h.appName,
		AccountName: username,
	})
	if err != nil {
		h.logger.Error().Err(err).Msg("generate totp")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Save secret (not yet enabled)
	_, err = h.db.Exec(r.Context(), `UPDATE users SET totp_secret = $1 WHERE id = $2`, key.Secret(), userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("save totp secret")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Generate QR code as base64 PNG
	img, err := key.Image(256, 256)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	qrBase64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	h.logger.Info().Str("username", username).Msg("totp setup initiated")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"secret": key.Secret(),
		"qr":     qrBase64,
	})
}

func (h *UsersHandler) TOTPEnable(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var secret *string
	h.db.QueryRow(r.Context(), `SELECT totp_secret FROM users WHERE id = $1`, userID).Scan(&secret)
	if secret == nil {
		http.Error(w, "run setup first", http.StatusBadRequest)
		return
	}

	if !totp.Validate(req.Code, *secret) {
		http.Error(w, "invalid code", http.StatusUnauthorized)
		return
	}

	h.db.Exec(r.Context(), `UPDATE users SET totp_enabled = TRUE WHERE id = $1`, userID)
	h.logger.Info().Str("user_id", userID).Msg("totp enabled")
	w.WriteHeader(http.StatusNoContent)
}

func (h *UsersHandler) TOTPDisable(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var secret *string
	var totpEnabled bool
	h.db.QueryRow(r.Context(), `SELECT totp_secret, totp_enabled FROM users WHERE id = $1`, userID).Scan(&secret, &totpEnabled)

	if totpEnabled && secret != nil {
		if !totp.Validate(req.Code, *secret) {
			http.Error(w, "invalid code", http.StatusUnauthorized)
			return
		}
	}

	h.db.Exec(r.Context(), `UPDATE users SET totp_enabled = FALSE, totp_secret = NULL WHERE id = $1`, userID)
	h.logger.Info().Str("user_id", userID).Msg("totp disabled")
	w.WriteHeader(http.StatusNoContent)
}
