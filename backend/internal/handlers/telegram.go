package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	authmw "life-dashboard/internal/middleware"
)

const (
	telegramAPIBase = "https://api.telegram.org"
	// Long polling: the request parks on Telegram's side until an update shows
	// up, which is why the client timeout has to outlive the poll itself.
	telegramPollTimeout    = 30 * time.Second
	telegramRequestTimeout = 45 * time.Second
	// A link code is typed into a chat within a minute or two of being issued.
	telegramLinkCodeTTL = 15 * time.Minute
	// Telegram rejects anything longer than 4096 characters, so reports are cut
	// below that with room for the part marker.
	telegramMaxMessageRunes = 3500
	telegramRetryDelay      = 5 * time.Second
)

type TelegramHandler struct {
	db      *pgxpool.Pool
	client  *telegramClient
	logger  zerolog.Logger
	botName string
}

type telegramClient struct {
	baseURL string
	token   string
	http    *http.Client
	logger  zerolog.Logger
}

type TelegramStatus struct {
	// Configured is false when the instance has no bot token at all, which is a
	// different problem from "this account has not linked a chat yet".
	Configured  bool       `json:"configured"`
	Linked      bool       `json:"linked"`
	BotUsername string     `json:"bot_username,omitempty"`
	ChatTitle   string     `json:"chat_title,omitempty"`
	LinkedAt    *time.Time `json:"linked_at,omitempty"`
}

type telegramLinkResponse struct {
	Code      string    `json:"code"`
	DeepLink  string    `json:"deep_link"`
	ExpiresAt time.Time `json:"expires_at"`
}

type telegramUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Text string `json:"text"`
		Chat struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
			Title    string `json:"title"`
			First    string `json:"first_name"`
		} `json:"chat"`
	} `json:"message"`
}

func NewTelegram(db *pgxpool.Pool, botToken, apiBase string, logger zerolog.Logger) *TelegramHandler {
	log := logger.With().Str("handler", "telegram").Logger()
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if base == "" {
		base = telegramAPIBase
	}
	return &TelegramHandler{
		db: db,
		client: &telegramClient{
			baseURL: base,
			token:   strings.TrimSpace(botToken),
			http:    &http.Client{Timeout: telegramRequestTimeout},
			logger:  log,
		},
		logger: log,
	}
}

func (h *TelegramHandler) enabled() bool { return h != nil && h.client.token != "" }

// GetStatus answers what Settings needs to draw the card.
func (h *TelegramHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)

	status := TelegramStatus{Configured: h.enabled()}
	if status.Configured {
		status.BotUsername = h.botUsername(ctx)
	}

	var chatTitle *string
	var linkedAt time.Time
	err := h.db.QueryRow(ctx, `
		SELECT username, linked_at FROM telegram_accounts WHERE user_id = $1
	`, userID).Scan(&chatTitle, &linkedAt)
	if err == nil {
		status.Linked = true
		status.LinkedAt = &linkedAt
		if chatTitle != nil {
			status.ChatTitle = *chatTitle
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		h.logger.Error().Err(err).Msg("load telegram account")
	}

	writeJSONStatus(w, http.StatusOK, status)
}

// CreateLink issues the one-time code the bot expects in /start.
func (h *TelegramHandler) CreateLink(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		http.Error(w, "telegram bot is not configured on the server", http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)

	code, err := newTelegramLinkCode()
	if err != nil {
		h.logger.Error().Err(err).Msg("generate telegram link code")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	expiresAt := time.Now().Add(telegramLinkCodeTTL)
	if _, err := h.db.Exec(ctx, `
		INSERT INTO telegram_link_codes (code, user_id, expires_at)
		VALUES ($1, $2, $3)
	`, code, userID, expiresAt); err != nil {
		h.logger.Error().Err(err).Msg("store telegram link code")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	botName := h.botUsername(ctx)
	if botName == "" {
		http.Error(w, "telegram bot is unreachable", http.StatusBadGateway)
		return
	}

	writeJSONStatus(w, http.StatusOK, telegramLinkResponse{
		Code:      code,
		DeepLink:  fmt.Sprintf("https://t.me/%s?start=%s", botName, code),
		ExpiresAt: expiresAt,
	})
}

// Unlink stops delivery and forgets the chat.
func (h *TelegramHandler) Unlink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)

	if _, err := h.db.Exec(ctx, `DELETE FROM telegram_accounts WHERE user_id = $1`, userID); err != nil {
		h.logger.Error().Err(err).Msg("unlink telegram account")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, http.StatusOK, TelegramStatus{Configured: h.enabled(), BotUsername: h.botUsername(ctx)})
}

// botUsername is read from the bot itself and cached: it is needed for the
// deep link, and asking Telegram once is better than another setting to keep in
// sync with whatever BotFather was told.
func (h *TelegramHandler) botUsername(ctx context.Context) string {
	if !h.enabled() {
		return ""
	}
	if h.botName != "" {
		return h.botName
	}

	var me struct {
		Username string `json:"username"`
	}
	if err := h.client.call(ctx, "getMe", nil, &me); err != nil {
		h.logger.Warn().Err(err).Msg("read telegram bot username")
		return ""
	}
	h.botName = me.Username
	return h.botName
}

// StartTelegramWorker consumes bot updates for as long as the process lives.
//
// Long polling rather than a webhook: the only thing the bot has to hear is a
// /start with a code, and polling needs no public callback URL, no TLS wiring
// and no secret to rotate.
func (h *TelegramHandler) StartTelegramWorker(ctx context.Context) {
	if !h.enabled() {
		h.logger.Info().Msg("telegram worker disabled: no bot token")
		return
	}

	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			if err := h.pollOnce(ctx); err != nil {
				if ctx.Err() != nil {
					return
				}
				h.logger.Warn().Err(err).Msg("telegram poll failed")
				select {
				case <-ctx.Done():
					return
				case <-time.After(telegramRetryDelay):
				}
			}
		}
	}()
}

func (h *TelegramHandler) pollOnce(ctx context.Context) error {
	offset, err := h.pollOffset(ctx)
	if err != nil {
		return err
	}

	var updates []telegramUpdate
	if err := h.client.call(ctx, "getUpdates", map[string]any{
		"offset":          offset + 1,
		"timeout":         int(telegramPollTimeout / time.Second),
		"allowed_updates": []string{"message"},
	}, &updates); err != nil {
		return err
	}

	for _, update := range updates {
		if update.Message != nil {
			h.handleMessage(ctx, update)
		}
		if err := h.storePollOffset(ctx, update.UpdateID); err != nil {
			return err
		}
	}
	return nil
}

func (h *TelegramHandler) handleMessage(ctx context.Context, update telegramUpdate) {
	text := strings.TrimSpace(update.Message.Text)
	chatID := update.Message.Chat.ID

	if !strings.HasPrefix(text, "/start") {
		h.reply(ctx, chatID, "Я присылаю checkup из Life Dashboard. Привяжи чат кнопкой в настройках дашборда.")
		return
	}

	code := strings.TrimSpace(strings.TrimPrefix(text, "/start"))
	if code == "" {
		h.reply(ctx, chatID, "Нужен код привязки. Открой настройки Life Dashboard и нажми \"Привязать Telegram\".")
		return
	}

	title := update.Message.Chat.Username
	if title == "" {
		title = strings.TrimSpace(update.Message.Chat.Title + " " + update.Message.Chat.First)
	}

	userID, err := h.consumeLinkCode(ctx, code, chatID, title)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.reply(ctx, chatID, "Код не подошёл или уже истёк. Сгенерируй новый в настройках дашборда.")
			return
		}
		h.logger.Error().Err(err).Msg("link telegram chat")
		h.reply(ctx, chatID, "Не получилось привязать чат, попробуй ещё раз чуть позже.")
		return
	}

	h.logger.Info().Str("user_id", userID).Int64("chat_id", chatID).Msg("telegram chat linked")
	h.reply(ctx, chatID, "Готово. Буду присылать сюда checkup по расписанию.")
}

// consumeLinkCode binds the chat inside one transaction: a code that linked a
// chat must not be usable a second time.
func (h *TelegramHandler) consumeLinkCode(ctx context.Context, code string, chatID int64, title string) (string, error) {
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var userID string
	if err := tx.QueryRow(ctx, `
		UPDATE telegram_link_codes
		SET used_at = NOW()
		WHERE code = $1 AND used_at IS NULL AND expires_at > NOW()
		RETURNING user_id
	`, code).Scan(&userID); err != nil {
		return "", err
	}

	// One chat belongs to one account: re-linking moves it rather than leaving
	// two accounts pushing reports into the same conversation.
	if _, err := tx.Exec(ctx, `DELETE FROM telegram_accounts WHERE chat_id = $1`, chatID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO telegram_accounts (user_id, chat_id, username, linked_at)
		VALUES ($1, $2, NULLIF($3, ''), NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			chat_id = EXCLUDED.chat_id,
			username = EXCLUDED.username,
			linked_at = NOW()
	`, userID, chatID, strings.TrimSpace(title)); err != nil {
		return "", err
	}

	return userID, tx.Commit(ctx)
}

func (h *TelegramHandler) pollOffset(ctx context.Context) (int64, error) {
	var offset int64
	err := h.db.QueryRow(ctx, `SELECT last_update_id FROM telegram_poll_state WHERE singleton`).Scan(&offset)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return offset, err
}

func (h *TelegramHandler) storePollOffset(ctx context.Context, updateID int64) error {
	_, err := h.db.Exec(ctx, `
		UPDATE telegram_poll_state
		SET last_update_id = GREATEST(last_update_id, $1), updated_at = NOW()
		WHERE singleton
	`, updateID)
	return err
}

func (h *TelegramHandler) reply(ctx context.Context, chatID int64, text string) {
	if err := h.client.sendMessage(ctx, chatID, text); err != nil {
		h.logger.Warn().Err(err).Int64("chat_id", chatID).Msg("reply in telegram")
	}
}

// SendReport delivers a finished report to the account's chat. Not linked is
// not an error: most accounts simply have no chat bound.
func (h *TelegramHandler) SendReport(ctx context.Context, userID, title, body string) (bool, error) {
	if !h.enabled() {
		return false, nil
	}

	var chatID int64
	err := h.db.QueryRow(ctx, `SELECT chat_id FROM telegram_accounts WHERE user_id = $1`, userID).Scan(&chatID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	chunks := splitTelegramMessage(strings.TrimSpace(title+"\n\n"+body), telegramMaxMessageRunes)
	for _, chunk := range chunks {
		if err := h.client.sendMessage(ctx, chatID, chunk); err != nil {
			return false, err
		}
	}
	return true, nil
}

// splitTelegramMessage cuts a long report on a paragraph, then on a line, then
// wherever the limit falls, so a report arrives as a few readable messages
// rather than as one the API rejects.
func splitTelegramMessage(text string, limit int) []string {
	trimmed := strings.TrimSpace(text)
	rest := []rune(trimmed)
	if limit <= 0 || len(rest) <= limit {
		return []string{trimmed}
	}

	chunks := make([]string, 0, len(rest)/limit+1)
	for len(rest) > limit {
		window := rest[:limit]
		cut := lastRuneIndex(window, "\n\n")
		if cut <= 0 {
			cut = lastRuneIndex(window, "\n")
		}
		if cut <= 0 {
			cut = limit
		}
		if chunk := strings.TrimSpace(string(rest[:cut])); chunk != "" {
			chunks = append(chunks, chunk)
		}
		rest = []rune(strings.TrimLeft(string(rest[cut:]), " \n"))
	}
	if tail := strings.TrimSpace(string(rest)); tail != "" {
		chunks = append(chunks, tail)
	}
	return chunks
}

func lastRuneIndex(runes []rune, separator string) int {
	sep := []rune(separator)
	for i := len(runes) - len(sep); i >= 0; i-- {
		if string(runes[i:i+len(sep)]) == separator {
			return i
		}
	}
	return -1
}

func newTelegramLinkCode() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (c *telegramClient) sendMessage(ctx context.Context, chatID int64, text string) error {
	// No parse_mode on purpose: reports are Markdown-ish prose, and Telegram
	// rejects the whole message over a stray underscore or bracket.
	return c.call(ctx, "sendMessage", map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}, nil)
}

func (c *telegramClient) call(ctx context.Context, method string, payload map[string]any, out any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	endpoint := fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var envelope struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode telegram %s response: %w", method, err)
	}
	if !envelope.OK {
		// The token never travels into a log line: the description alone says
		// what went wrong.
		return fmt.Errorf("telegram %s failed: %s", method, strings.TrimSpace(envelope.Description))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Result, out)
}
