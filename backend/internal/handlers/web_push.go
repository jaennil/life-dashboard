package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	authmw "life-dashboard/internal/middleware"
)

type WebPushOptions struct {
	PublicKey  string
	PrivateKey string
	Subscriber string
}

type webPushSender struct {
	db      *pgxpool.Pool
	options WebPushOptions
	logger  zerolog.Logger
}

type webPushSubscriptionRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func newWebPushSender(db *pgxpool.Pool, options WebPushOptions, logger zerolog.Logger) *webPushSender {
	return &webPushSender{db: db, options: options, logger: logger.With().Str("component", "web_push").Logger()}
}

func (h *VoiceWorkoutHandler) GetPushConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSONStatus(w, http.StatusOK, map[string]any{
		"enabled":    h.push.enabled(),
		"public_key": h.push.options.PublicKey,
	})
}

func (h *VoiceWorkoutHandler) SavePushSubscription(w http.ResponseWriter, r *http.Request) {
	if !h.push.enabled() {
		http.Error(w, "web push is not configured", http.StatusServiceUnavailable)
		return
	}
	var subscription webPushSubscriptionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&subscription); err != nil {
		http.Error(w, "invalid subscription", http.StatusBadRequest)
		return
	}
	subscription.Endpoint = strings.TrimSpace(subscription.Endpoint)
	if subscription.Endpoint == "" || subscription.Keys.P256dh == "" || subscription.Keys.Auth == "" {
		http.Error(w, "incomplete subscription", http.StatusBadRequest)
		return
	}
	userID := r.Context().Value(authmw.UserIDKey).(string)
	_, err := h.db.Exec(r.Context(), `
		INSERT INTO web_push_subscriptions (user_id, endpoint, p256dh, auth)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, endpoint) DO UPDATE
		SET p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth, updated_at = NOW()
	`, userID, subscription.Endpoint, subscription.Keys.P256dh, subscription.Keys.Auth)
	if err != nil {
		http.Error(w, "cannot save subscription", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *VoiceWorkoutHandler) DeletePushSubscription(w http.ResponseWriter, r *http.Request) {
	var subscription struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&subscription); err != nil {
		http.Error(w, "invalid subscription", http.StatusBadRequest)
		return
	}
	userID := r.Context().Value(authmw.UserIDKey).(string)
	if _, err := h.db.Exec(r.Context(), `
		DELETE FROM web_push_subscriptions WHERE user_id = $1 AND endpoint = $2
	`, userID, subscription.Endpoint); err != nil {
		http.Error(w, "cannot delete subscription", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (p *webPushSender) enabled() bool {
	return p != nil && p.options.PublicKey != "" && p.options.PrivateKey != ""
}

func (p *webPushSender) sendInputResult(ctx context.Context, userID, jobID, display string, success bool) {
	if !p.enabled() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	title := "Запись готова"
	if !success {
		title = "Не удалось обработать запись"
	}
	payload, err := json.Marshal(map[string]string{
		"title": title, "body": display, "url": "/input", "tag": "input-job-" + jobID,
	})
	if err != nil {
		return
	}

	rows, err := p.db.Query(ctx, `
		SELECT endpoint, p256dh, auth FROM web_push_subscriptions WHERE user_id = $1
	`, userID)
	if err != nil {
		p.logger.Warn().Err(err).Msg("load subscriptions")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var subscription webpush.Subscription
		if err := rows.Scan(&subscription.Endpoint, &subscription.Keys.P256dh, &subscription.Keys.Auth); err != nil {
			p.logger.Warn().Err(err).Msg("scan subscription")
			continue
		}
		response, err := webpush.SendNotificationWithContext(ctx, payload, &subscription, &webpush.Options{
			Subscriber: p.options.Subscriber, VAPIDPublicKey: p.options.PublicKey,
			VAPIDPrivateKey: p.options.PrivateKey, TTL: 86400,
		})
		if err != nil {
			p.logger.Warn().Err(err).Msg("send notification")
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode == http.StatusGone || response.StatusCode == http.StatusNotFound {
			if _, err := p.db.Exec(ctx, `DELETE FROM web_push_subscriptions WHERE endpoint = $1`, subscription.Endpoint); err != nil {
				p.logger.Warn().Err(err).Msg("delete expired subscription")
			}
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			p.logger.Warn().Int("status", response.StatusCode).Msg("push service rejected notification")
			continue
		}
		p.logger.Info().Int("status", response.StatusCode).Str("job_id", jobID).Msg("notification sent")
	}
}
