package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	unleashclient "github.com/Unleash/unleash-client-go/v4"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	authmw "life-dashboard/internal/middleware"
	"life-dashboard/internal/observability"
)

type AIHandler struct {
	db      *pgxpool.Pool
	opts    AIOptions
	weather *WeatherHandler
	unleash *unleashclient.Client
	logger  zerolog.Logger
}

// AIOptions carries the upstream settings. They travel as a struct rather than
// as positional arguments because they are otherwise three interchangeable
// strings that are easy to pass in the wrong order.
type AIOptions struct {
	BaseURL string
	Model   string
	APIKey  string
	// ReasoningEffort is sent as reasoning_effort when set, and omitted when not.
	ReasoningEffort string
	// RequestTimeout bounds a single upstream call. Zero falls back to the
	// default below.
	RequestTimeout time.Duration
}

var aiDisplayLocation = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err == nil {
		return loc
	}
	return time.FixedZone("MSK", 3*60*60)
}()

func NewAI(db *pgxpool.Pool, opts AIOptions, weather *WeatherHandler, unleashClient *unleashclient.Client, logger zerolog.Logger) *AIHandler {
	opts.BaseURL = strings.TrimRight(opts.BaseURL, "/")
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = aiUpstreamDefaultTimeout
	}
	return &AIHandler{
		db:      db,
		opts:    opts,
		weather: weather,
		unleash: unleashClient,
		logger:  logger.With().Str("handler", "ai").Logger(),
	}
}

// Complete exposes a single non-streaming upstream call. The voice-workout
// webhook parses dictated phrases through it so that provider, model and
// reasoning effort stay configured in exactly one place.
func (h *AIHandler) Complete(ctx context.Context, operation string, messages []ChatMessage) (string, error) {
	return h.complete(ctx, operation, messages)
}

// CompleteWithModel is Complete with model-specific settings. An explicit model
// starts with no reasoning effort unless one is supplied: a fast extraction
// model must not accidentally inherit the main chat model's expensive setting.
// With no model override, an empty effort keeps all configured defaults.
func (h *AIHandler) CompleteWithModel(ctx context.Context, operation string, messages []ChatMessage, model, effort string) (string, error) {
	if model == "" && effort == "" {
		return h.complete(ctx, operation, messages)
	}

	override := *h
	if model != "" {
		override.opts.Model = model
		override.opts.ReasoningEffort = effort
	}
	if model == "" && effort != "" {
		override.opts.ReasoningEffort = effort
	}
	return override.complete(ctx, operation, messages)
}

// upstreamBody builds the chat-completions payload. reasoning_effort only
// appears when configured: providers that do not know the field answer 400 for
// the whole request instead of ignoring the extra key.
func (h *AIHandler) upstreamBody(messages []ChatMessage, stream bool) ([]byte, error) {
	payload := map[string]any{
		"model":    h.opts.Model,
		"messages": messages,
		"stream":   stream,
	}
	if h.opts.ReasoningEffort != "" {
		payload["reasoning_effort"] = h.opts.ReasoningEffort
	}
	return json.Marshal(payload)
}

// upstreamTimeout is the budget for one call, and doubles as the time allowed
// before the first byte: a thinking model stays silent while it reasons, so a
// separate, shorter header timeout would cut off exactly the calls we want.
func (h *AIHandler) upstreamTimeout() time.Duration {
	if h.opts.RequestTimeout > 0 {
		return h.opts.RequestTimeout
	}
	return aiUpstreamDefaultTimeout
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Message string        `json:"message"`
	History []ChatMessage `json:"history"`
}

type aiContextScope struct {
	finance      bool
	productivity bool
	activities   bool
	health       bool
	workouts     bool
	routines     bool
	habits       bool
	nutrition    bool
	journal      bool
	calendar     bool
	weather      bool
}

type aiJournalEntry struct {
	Date    time.Time
	Title   string
	Content string
	Tags    []string
	Mood    *int
	Source  string
}

type aiStreamEvent struct {
	Type        string `json:"type"`
	Content     string `json:"content,omitempty"`
	Period      string `json:"period,omitempty"`
	PeriodLabel string `json:"period_label,omitempty"`
	GeneratedAt string `json:"generated_at,omitempty"`
	Stage       string `json:"stage,omitempty"`
	Tool        string `json:"tool,omitempty"`
	Section     string `json:"section,omitempty"`
}

type aiProgressUpdate struct {
	Stage   string
	Message string
	Tool    aiToolName
	Section string
}

const (
	aiUpstreamDialTimeout     = 5 * time.Second
	aiUpstreamDefaultTimeout  = 10 * time.Minute
	aiUpstreamResponseLogSize = 512
	aiJournalDefaultLimit     = 300
	aiCheckupJournalTotalSize = 50000
	aiCheckupJournalMaxItems  = 45
)

var (
	errAIUnavailable = observability.ErrAIUnavailable
	errAIUpstream    = observability.ErrAIUpstream
	errAIBadResponse = observability.ErrAIBadResponse
)

func (h *AIHandler) Chat(w http.ResponseWriter, r *http.Request) {
	if h.unleash != nil && !h.unleash.IsEnabled("ai-chat") {
		http.Error(w, "AI чат временно отключён", http.StatusForbidden)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, "empty message", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	history := h.buildConversationHistory(ctx, userID, req.History)

	if isAIStreamRequest(r) {
		flusher, ok := prepareAIStream(w)
		if !ok {
			http.Error(w, "stream unsupported", http.StatusInternalServerError)
			return
		}

		progress := func(update aiProgressUpdate) error {
			return writeAIStreamEvent(w, flusher, aiStreamEvent{
				Type:    "status",
				Content: update.Message,
				Stage:   update.Stage,
				Tool:    string(update.Tool),
				Section: update.Section,
			})
		}

		if err := progress(aiProgressUpdate{Stage: "planning", Message: "Понимаю вопрос и определяю, какие данные нужны"}); err != nil {
			h.logger.Warn().Err(err).Msg("write ai planning status")
			return
		}

		dataContext, sectionNames, err := h.buildChatContextWithProgress(ctx, userID, req.Message, history, progress)
		if err != nil {
			h.logger.Error().Err(err).Msg("build context")
			dataContext = "Данные пользователя временно недоступны."
			sectionNames = defaultAIContextScope().sectionNames()
			_ = progress(aiProgressUpdate{Stage: "loading", Message: "Часть данных недоступна, отвечу по тому, что удалось загрузить"})
		}

		systemPrompt := buildAISystemPromptWithSections(time.Now(), dataContext, sectionNames)

		messages := []ChatMessage{{Role: "system", Content: systemPrompt}}
		messages = append(messages, history...)
		messages = append(messages, ChatMessage{Role: "user", Content: req.Message})

		_ = progress(aiProgressUpdate{Stage: "generating", Message: "Формирую ответ"})

		content, err := h.completeStream(ctx, "chat", messages, func(delta string) error {
			return writeAIStreamEvent(w, flusher, aiStreamEvent{Type: "delta", Content: delta})
		})
		if err != nil {
			_ = writeAIStreamEvent(w, flusher, aiStreamEvent{Type: "error", Content: aiCompletionErrorMessage(err)})
			return
		}

		h.storeChatExchange(ctx, userID, req.Message, content)
		_ = writeAIStreamEvent(w, flusher, aiStreamEvent{Type: "done", Content: content})
		return
	}

	dataContext, sectionNames, err := h.buildChatContext(ctx, userID, req.Message, history)
	if err != nil {
		h.logger.Error().Err(err).Msg("build context")
		dataContext = "Данные пользователя временно недоступны."
		sectionNames = defaultAIContextScope().sectionNames()
	}

	systemPrompt := buildAISystemPromptWithSections(time.Now(), dataContext, sectionNames)

	messages := []ChatMessage{{Role: "system", Content: systemPrompt}}
	messages = append(messages, history...)
	messages = append(messages, ChatMessage{Role: "user", Content: req.Message})

	content, err := h.complete(ctx, "chat", messages)
	if err != nil {
		writeAICompletionError(w, err)
		return
	}

	respBody, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		h.logger.Error().Err(err).Msg("marshal ai response")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.storeChatExchange(ctx, userID, req.Message, content)

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(respBody); err != nil {
		h.logger.Error().Err(err).Msg("write ai response")
	}
}

func (h *AIHandler) buildConversationHistory(ctx context.Context, userID string, clientHistory []ChatMessage) []ChatMessage {
	clientHistory = sanitizeChatHistory(clientHistory, aiHistoryContextLimit)

	storedHistory, err := h.loadRecentChatMessages(ctx, userID, aiHistoryContextLimit)
	if err != nil {
		h.logger.Warn().Err(err).Str("user_id", userID).Msg("load ai chat history from db")
		return clientHistory
	}
	if len(storedHistory) == 0 {
		return clientHistory
	}

	history := mergeChatHistory(storedHistory, clientHistory, aiHistoryContextLimit)
	h.logger.Debug().
		Str("user_id", userID).
		Int("stored_history", len(storedHistory)).
		Int("client_history", len(clientHistory)).
		Int("merged_history", len(history)).
		Msg("ai chat history prepared")
	return history
}

func sanitizeChatHistory(history []ChatMessage, limit int) []ChatMessage {
	if limit <= 0 || len(history) == 0 {
		return nil
	}
	start := len(history) - limit
	if start < 0 {
		start = 0
	}

	messages := make([]ChatMessage, 0, len(history)-start)
	for _, msg := range history[start:] {
		if normalized := normalizeChatMessage(msg); normalized != nil {
			messages = append(messages, *normalized)
		}
	}
	return messages
}

func formatAIProgressToolList(calls []aiToolCall) string {
	if len(calls) == 0 {
		return "базовый набор данных"
	}

	seen := make(map[string]bool, len(calls))
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		label := aiToolProgressLabel(call.Name)
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		names = append(names, label)
	}
	if len(names) == 0 {
		return "нужные разделы"
	}
	return strings.Join(names, ", ")
}

func aiToolProgressLabel(name aiToolName) string {
	switch name {
	case aiToolFinanceOverview, aiToolRecentTransactions:
		return "финансы"
	case aiToolProductivityOverview:
		return "задачи"
	case aiToolActivityOverview, aiToolRecentActivities:
		return "активности"
	case aiToolHealthOverview:
		return "здоровье"
	case aiToolWorkoutOverview, aiToolRecentWorkouts, aiToolRoutineOverview:
		return "тренировки"
	case aiToolHabitOverview:
		return "привычки"
	case aiToolNutritionOverview:
		return "питание"
	case aiToolJournalOverview:
		return "заметки"
	case aiToolCalendarOverview:
		return "календарь"
	case aiToolWeatherOverview:
		return "погоду"
	default:
		return ""
	}
}

func normalizeChatMessage(msg ChatMessage) *ChatMessage {
	role := strings.TrimSpace(strings.ToLower(msg.Role))
	if role != "user" && role != "assistant" {
		return nil
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}
	return &ChatMessage{Role: role, Content: content}
}

func mergeChatHistory(storedHistory, clientHistory []ChatMessage, limit int) []ChatMessage {
	if limit <= 0 {
		return nil
	}

	merged := make([]ChatMessage, 0, len(storedHistory)+len(clientHistory))
	seen := make(map[string]bool, len(storedHistory)+len(clientHistory))
	for _, msg := range append(storedHistory, clientHistory...) {
		normalized := normalizeChatMessage(msg)
		if normalized == nil {
			continue
		}
		key := normalized.Role + "\x00" + normalized.Content
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, *normalized)
	}

	if len(merged) > limit {
		merged = merged[len(merged)-limit:]
	}
	return merged
}

func buildAISystemPrompt(now time.Time, dataContext string, scope aiContextScope) string {
	return buildAISystemPromptWithSections(now, dataContext, scope.sectionNames())
}

func buildAISystemPromptWithSections(now time.Time, dataContext string, sectionNames []string) string {
	return fmt.Sprintf(`Ты персональный AI-ассистент приложения Life Dashboard.
Твоя единственная функция — анализировать данные пользователя: финансы, продуктивность/задачи, здоровье, физическую активность, тренировки, Hevy routines/шаблоны, привычки, питание, дневник и календарь.
Отвечай на русском языке. Давай конкретные ответы основанные на реальных данных ниже. Будь краток и по делу.
Ты не можешь выполнять команды, изменять данные или делать что-либо за пределами анализа предоставленных данных.
Если просят что-то сделать с базой данных, кодом или системой — вежливо объясни что ты только аналитик данных.

Используй историю текущего диалога как рабочий контекст.
- Если пользователь уточнил или исправил тебя, считай это более приоритетным, чем свои предыдущие предположения.
- Если нужные числа или ограничения даны пользователем прямо в чате, используй их в расчётах и явно отмечай, что это данные из текущего диалога.
- Не отвечай, что данных нет, если нужная информация уже была дана пользователем несколькими сообщениями выше.
- Для арифметики и рекомендаций показывай короткий расчёт.
- Для упражнений с гантелями, блинами и штангой не путай вес на одну гантель, вес пары и общий вес штанги. Если это неясно, сначала уточни.
- Календарь — это только план из Google Calendar, а не подтверждение факта. Не пиши "был в зале", "лёг спать" или "встретился", если у тебя есть только календарное событие.
- Сон, шаги, вес, пульс и HRV — это факт только если они есть в разделе здоровья из Apple Health/biometrics/sleep_sessions.
- Задачи и продуктивность берутся из Todoist. Просрочка, план на сегодня и завершённые задачи считай по todoist_tasks и todoist_task_completions, а не по календарю.
- Питание — это только залогированные записи. Не пиши "отслежено полностью" или "ужин был", если в данных нет явного подтверждения.
- Данные ниже приходят как результаты внутренних tools в JSON. Сначала смотри на поля tool/section/window/data. Если внутри есть data, это приоритетный структурированный payload. context_text — вспомогательная summary этого tool.

Сейчас особенно релевантны разделы данных: %s.

Текущие данные пользователя (обновлено %s):
%s`, strings.Join(sectionNames, ", "), formatAITimestampLocal(now, "02.01.2006 15:04"), dataContext)
}

func (h *AIHandler) complete(ctx context.Context, operation string, messages []ChatMessage) (_ string, err error) {
	start := time.Now()
	defer func() {
		observability.ObserveAIUpstream(operation, observability.AIStatusFromError(err), time.Since(start))
	}()

	body, err := h.upstreamBody(messages, false)
	if err != nil {
		h.logger.Error().Err(err).Msg("marshal ai request")
		return "", err
	}

	upstreamCtx, cancel := context.WithTimeout(ctx, h.upstreamTimeout())
	defer cancel()

	apiReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost,
		h.opts.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	apiReq.Header.Set("Content-Type", "application/json")
	if h.opts.APIKey != "" {
		apiReq.Header.Set("Authorization", "Bearer "+h.opts.APIKey)
	}

	client := &http.Client{
		Timeout: h.upstreamTimeout(),
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   aiUpstreamDialTimeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   aiUpstreamDialTimeout,
			ResponseHeaderTimeout: h.upstreamTimeout(),
		},
	}

	resp, err := client.Do(apiReq)
	if err != nil {
		h.logger.Error().Err(err).Msg("ai api request")
		return "", errAIUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, aiUpstreamResponseLogSize))
		h.logger.Error().
			Int("status", resp.StatusCode).
			Str("body", strings.TrimSpace(string(body))).
			Msg("ai api error")
		return "", errAIUpstream
	}

	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		h.logger.Error().Err(err).Msg("read ai response")
		return "", errAIUnavailable
	}

	var completion struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rawResp, &completion); err != nil {
		h.logger.Error().Err(err).Str("body", truncateAIText(string(rawResp), aiUpstreamResponseLogSize)).Msg("decode ai response")
		return "", errAIBadResponse
	}
	if len(completion.Choices) == 0 {
		h.logger.Error().Msg("ai response has no choices")
		return "", errAIBadResponse
	}

	content := normalizeAIContent(completion.Choices[0].Message.Content)
	if strings.TrimSpace(content) == "" {
		h.logger.Error().Msg("ai response content is empty")
		return "", errAIBadResponse
	}

	return content, nil
}

func (h *AIHandler) completeStream(ctx context.Context, operation string, messages []ChatMessage, onDelta func(string) error) (_ string, err error) {
	start := time.Now()
	defer func() {
		observability.ObserveAIUpstream(operation, observability.AIStatusFromError(err), time.Since(start))
	}()

	body, err := h.upstreamBody(messages, true)
	if err != nil {
		h.logger.Error().Err(err).Msg("marshal ai stream request")
		return "", err
	}

	upstreamCtx, cancel := context.WithTimeout(ctx, h.upstreamTimeout())
	defer cancel()

	apiReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost,
		h.opts.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	apiReq.Header.Set("Content-Type", "application/json")
	if h.opts.APIKey != "" {
		apiReq.Header.Set("Authorization", "Bearer "+h.opts.APIKey)
	}

	client := &http.Client{
		Timeout: h.upstreamTimeout(),
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   aiUpstreamDialTimeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   aiUpstreamDialTimeout,
			ResponseHeaderTimeout: h.upstreamTimeout(),
		},
	}

	resp, err := client.Do(apiReq)
	if err != nil {
		h.logger.Error().Err(err).Msg("ai stream request")
		return "", errAIUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, aiUpstreamResponseLogSize))
		h.logger.Error().
			Int("status", resp.StatusCode).
			Str("body", strings.TrimSpace(string(body))).
			Msg("ai stream api error")
		return "", errAIUpstream
	}

	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		rawResp, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			h.logger.Error().Err(readErr).Msg("read non-stream ai response")
			return "", errAIUnavailable
		}

		content, parseErr := parseAICompletion(rawResp)
		if parseErr != nil {
			h.logger.Error().Err(parseErr).Str("body", truncateAIText(string(rawResp), aiUpstreamResponseLogSize)).Msg("decode non-stream ai response")
			return "", errAIBadResponse
		}
		if onDelta != nil {
			if writeErr := onDelta(content); writeErr != nil {
				return "", writeErr
			}
		}
		return content, nil
	}

	reader := bufio.NewReader(resp.Body)
	var content strings.Builder
	dataLines := make([]string, 0, 4)

	processEvent := func(lines []string) error {
		if len(lines) == 0 {
			return nil
		}

		delta, done, parseErr := parseAIStreamData(strings.Join(lines, "\n"))
		if parseErr != nil {
			h.logger.Error().Err(parseErr).Msg("decode ai stream event")
			return errAIBadResponse
		}
		if done {
			return nil
		}
		if delta == "" {
			return nil
		}

		content.WriteString(delta)
		if onDelta != nil {
			if err := onDelta(delta); err != nil {
				return err
			}
		}
		return nil
	}

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			h.logger.Error().Err(readErr).Msg("read ai stream line")
			return "", errAIUnavailable
		}

		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			if err := processEvent(dataLines); err != nil {
				return "", err
			}
			dataLines = dataLines[:0]
		} else if strings.HasPrefix(trimmed, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		}

		if errors.Is(readErr, io.EOF) {
			if err := processEvent(dataLines); err != nil {
				return "", err
			}
			break
		}
	}

	if strings.TrimSpace(content.String()) == "" {
		h.logger.Error().Msg("ai stream content is empty")
		return "", errAIBadResponse
	}

	return content.String(), nil
}

func writeAICompletionError(w http.ResponseWriter, err error) {
	http.Error(w, aiCompletionErrorMessage(err), aiCompletionStatusCode(err))
}

func aiCompletionErrorMessage(err error) string {
	switch {
	case errors.Is(err, errAIUnavailable):
		return "AI сервис временно недоступен. Попробуй позже."
	case errors.Is(err, errAIUpstream):
		return "AI сервис вернул ошибку. Попробуй позже."
	case errors.Is(err, errAIBadResponse):
		return "AI сервис не вернул корректный ответ. Попробуй позже."
	default:
		return "internal error"
	}
}

func aiCompletionStatusCode(err error) int {
	switch {
	case errors.Is(err, errAIUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, errAIUpstream), errors.Is(err, errAIBadResponse):
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

func prepareAIStream(w http.ResponseWriter) (http.Flusher, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	return flusher, true
}

func writeAIStreamEvent(w io.Writer, flusher http.Flusher, event aiStreamEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(payload, '\n')); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func isAIStreamRequest(r *http.Request) bool {
	return r.URL.Query().Get("stream") == "1"
}

func parseAICompletion(rawResp []byte) (string, error) {
	var completion struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rawResp, &completion); err != nil {
		return "", err
	}
	if len(completion.Choices) == 0 {
		return "", errAIBadResponse
	}

	content := normalizeAIContent(completion.Choices[0].Message.Content)
	if strings.TrimSpace(content) == "" {
		return "", errAIBadResponse
	}
	return content, nil
}

func parseAIStreamData(data string) (delta string, done bool, err error) {
	if strings.TrimSpace(data) == "" {
		return "", false, nil
	}
	if strings.TrimSpace(data) == "[DONE]" {
		return "", true, nil
	}

	var chunk struct {
		Choices []struct {
			Delta struct {
				Content any `json:"content"`
			} `json:"delta"`
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return "", false, err
	}
	if len(chunk.Choices) == 0 {
		return "", false, nil
	}

	delta = normalizeAIContent(chunk.Choices[0].Delta.Content)
	if strings.TrimSpace(delta) != "" {
		return delta, false, nil
	}
	return normalizeAIContent(chunk.Choices[0].Message.Content), false, nil
}

func normalizeAIContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := obj["text"].(string); ok && text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func (h *AIHandler) buildContext(ctx context.Context, userID string, scope aiContextScope) (string, error) {
	var sb strings.Builder
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))

	// === ФИНАНСЫ ===
	if scope.finance {
		sb.WriteString("=== ФИНАНСЫ ===\n")

		rows, err := h.db.Query(ctx, `
			SELECT title, currency, balance, type, in_balance
			FROM accounts
			WHERE balance != 0 AND user_id = $1 AND COALESCE(archived, FALSE) = FALSE
			ORDER BY in_balance DESC, balance DESC LIMIT 10
		`, userID)
		if err == nil {
			sb.WriteString("Счета:\n")
			for rows.Next() {
				var title, currency, accType string
				var balance float64
				var inBalance bool
				if err := rows.Scan(&title, &currency, &balance, &accType, &inBalance); err == nil {
					visibility := ""
					if !inBalance {
						visibility = " [вне баланса]"
					}
					sb.WriteString(fmt.Sprintf("  - %s (%s%s): %.0f %s\n", title, accType, visibility, balance, currency))
				}
			}
			rows.Close()
		}

		var totalBalance, monthSpending, monthIncome float64
		h.db.QueryRow(ctx, `SELECT COALESCE(SUM(balance),0) FROM accounts WHERE currency='RUB' AND in_balance = TRUE AND COALESCE(archived, FALSE) = FALSE AND user_id = $1`, userID).Scan(&totalBalance)
		h.db.QueryRow(ctx, `
			SELECT
				COALESCE(SUM(CASE WHEN t.amount < 0 THEN ABS(t.amount) ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN t.amount > 0 THEN t.amount ELSE 0 END), 0)
			FROM transactions t
			LEFT JOIN accounts a ON a.id = t.account_id
			WHERE t.currency='RUB'
				AND t.occurred_at >= $1
				AND t.is_transfer=false
				AND t.user_id = $2
				AND COALESCE(a.in_balance, TRUE) = TRUE
		`, monthStart, userID).Scan(&monthSpending, &monthIncome)

		sb.WriteString(fmt.Sprintf("Общий баланс (RUB): %.0f ₽\n", totalBalance))
		sb.WriteString(fmt.Sprintf("Расходы за текущий месяц: %.0f ₽\n", monthSpending))
		sb.WriteString(fmt.Sprintf("Доходы за текущий месяц: %.0f ₽\n", monthIncome))

		txRows, err := h.db.Query(ctx, `
			SELECT t.occurred_at, t.amount, t.currency, COALESCE(t.payee, t.comment, '') as label
			FROM transactions t
			LEFT JOIN accounts a ON a.id = t.account_id
			WHERE t.is_transfer=false
				AND t.user_id = $1
				AND COALESCE(a.in_balance, TRUE) = TRUE
			ORDER BY t.occurred_at DESC LIMIT 30
		`, userID)
		if err == nil {
			sb.WriteString("Последние транзакции:\n")
			for txRows.Next() {
				var t time.Time
				var amount float64
				var currency, label string
				if err := txRows.Scan(&t, &amount, &currency, &label); err == nil {
					sign := ""
					if amount > 0 {
						sign = "+"
					}
					sb.WriteString(fmt.Sprintf("  %s %s%.0f %s %s\n", t.Format("02.01"), sign, amount, currency, label))
				}
			}
			txRows.Close()
		}
	}

	// === АКТИВНОСТИ ===
	if scope.activities {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("=== АКТИВНОСТИ ===\n")
		var weekActivities int
		var weekDistanceKm float64
		h.db.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE started_at >= $1 AND user_id = $2`, weekStart, userID).Scan(&weekActivities)
		h.db.QueryRow(ctx, `SELECT COALESCE(SUM(distance_meters)/1000.0,0) FROM activities WHERE started_at >= $1 AND user_id = $2`, weekStart, userID).Scan(&weekDistanceKm)
		sb.WriteString(fmt.Sprintf("За эту неделю: %d активностей, %.1f км\n", weekActivities, weekDistanceKm))

		actRows, err := h.db.Query(ctx, `
			SELECT started_at, type, COALESCE(distance_meters/1000.0,0), COALESCE(duration_seconds/60,0), name
			FROM activities WHERE user_id = $1 ORDER BY started_at DESC LIMIT 10
		`, userID)
		if err == nil {
			for actRows.Next() {
				var t time.Time
				var actType, name string
				var distKm, durationMin float64
				if err := actRows.Scan(&t, &actType, &distKm, &durationMin, &name); err == nil {
					sb.WriteString(fmt.Sprintf("  %s %s: %s %.1fкм %.0fмин\n", t.Format("02.01"), actType, name, distKm, durationMin))
				}
			}
			actRows.Close()
		}
	}

	if scope.productivity {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		if err := h.appendProductivityContextInRange(ctx, &sb, userID, now.AddDate(0, 0, -14), now, "=== ПРОДУКТИВНОСТЬ ===", 12); err != nil {
			return "", err
		}
	}

	if scope.health {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		h.appendHealthContextInRange(ctx, &sb, userID, now.AddDate(0, 0, -30), now, "=== ЗДОРОВЬЕ ===")
	}

	// === ТРЕНИРОВКИ ===
	if scope.workouts {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("=== ТРЕНИРОВКИ ===\n")
		var weekWorkouts int
		h.db.QueryRow(ctx, `SELECT COUNT(*) FROM workouts WHERE started_at >= $1 AND user_id = $2`, weekStart, userID).Scan(&weekWorkouts)
		sb.WriteString(fmt.Sprintf("За эту неделю: %d тренировок\n", weekWorkouts))

		workoutContext, err := h.buildRecentWorkoutContext(ctx, userID)
		if err == nil {
			sb.WriteString("Ниже приведены последние тренировки с реальными упражнениями и подходами, если они сохранены в базе:\n")
			sb.WriteString(workoutContext)
		}
	}

	if scope.routines {
		routineContext, err := h.buildRoutineContext(ctx, userID, 4)
		if err == nil && strings.TrimSpace(routineContext) != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString("=== HEVY ROUTINES ===\n")
			sb.WriteString("Ниже приведены шаблоны/routines из Hevy. Это плановые упражнения и веса, а не подтверждение факта выполнения.\n")
			sb.WriteString(routineContext)
		}
	}

	if scope.habits {
		habitContext, err := h.buildHabitContext(ctx, userID, 30)
		if err == nil && strings.TrimSpace(habitContext) != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(habitContext)
		}
	}

	// === ПИТАНИЕ ===
	if scope.nutrition {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("=== ПИТАНИЕ ===\n")
		sb.WriteString("Это лог питания и воды. Отсутствие ужина, перекуса или воды в логах не доказывает, что их не было.\n")
		if targets, err := loadNutritionTargets(ctx, h.db, userID); err == nil {
			sb.WriteString(renderNutritionTargetsForAI(targets))
		}
		nutritionRows, err := h.db.Query(ctx, `
			SELECT date, COALESCE(calories_total, 0), COALESCE(protein_g, 0), COALESCE(carbs_g, 0), COALESCE(fat_g, 0), COALESCE(fiber_g, 0), COALESCE(water_ml, 0)
			FROM nutrition_daily
			WHERE user_id = $1
			ORDER BY date DESC LIMIT 14
		`, userID)
		if err == nil {
			for nutritionRows.Next() {
				var date time.Time
				var cal, protein, carbs, fat, fiber, waterML float64
				if err := nutritionRows.Scan(&date, &cal, &protein, &carbs, &fat, &fiber, &waterML); err == nil {
					sb.WriteString(fmt.Sprintf("  %s: %.0f ккал | Б:%.0fг Ж:%.0fг У:%.0fг Клетч:%.0fг",
						date.Format("02.01"), cal, protein, fat, carbs, fiber))
					if waterML > 0 {
						sb.WriteString(fmt.Sprintf(" | Вода:%.0fмл", waterML))
					}
					sb.WriteString("\n")
				}
			}
			nutritionRows.Close()
		}

		// Детали по приёмам пищи за последние 2 дня
		mealRows, err := h.db.Query(ctx, `
			SELECT nd.date, ni.meal_type, ni.food_name, ni.serving_description, ni.calories
			FROM nutrition_items ni
			JOIN nutrition_daily nd ON nd.id = ni.daily_id
			WHERE nd.date >= $1 AND nd.user_id = $2
			ORDER BY nd.date DESC, ni.meal_type, ni.calories DESC
		`, now.AddDate(0, 0, -2), userID)
		if err == nil {
			var curDay string
			var curMeal string
			for mealRows.Next() {
				var date time.Time
				var mealType, foodName, serving string
				var calories float64
				if err := mealRows.Scan(&date, &mealType, &foodName, &serving, &calories); err != nil {
					continue
				}
				day := date.Format("02.01")
				if day != curDay {
					sb.WriteString(fmt.Sprintf("  Детали %s:\n", day))
					curDay = day
					curMeal = ""
				}
				if mealType != curMeal {
					sb.WriteString(fmt.Sprintf("    [%s]\n", mealType))
					curMeal = mealType
				}
				sb.WriteString(fmt.Sprintf("      - %s (%s): %.0f ккал\n", foodName, serving, calories))
			}
			mealRows.Close()
		}
	}

	// === ДНЕВНИК ===
	if scope.journal {
		recentEntries, err := h.loadAIJournalEntries(ctx, userID, 30, 20)
		if err == nil {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString("=== ДНЕВНИК / NOTION ===\n")
			sb.WriteString("Это заметки и записи из Notion. Если свежих записей нет, ниже показываются последние доступные.\n")

			if len(recentEntries) > 0 {
				sb.WriteString("Свежие записи за последние 30 дней:\n")
				writeAIJournalEntries(&sb, recentEntries)
			} else {
				sb.WriteString("За последние 30 дней новых записей нет.\n")
				latestEntries, latestErr := h.loadAILatestJournalEntries(ctx, userID, 10)
				if latestErr == nil && len(latestEntries) > 0 {
					sb.WriteString("Последние доступные записи:\n")
					writeAIJournalEntries(&sb, latestEntries)
				} else {
					sb.WriteString("В Notion-заметках записей не найдено.\n")
				}
			}
		}
	}

	// === КАЛЕНДАРЬ ===
	if scope.calendar {
		calRows, err := h.db.Query(ctx, `
			SELECT title, start_time, end_time, all_day, COALESCE(location, '')
			FROM calendar_events
			WHERE user_id = $1 AND start_time >= NOW() - INTERVAL '30 days' AND start_time <= NOW() + INTERVAL '30 days'
			ORDER BY start_time LIMIT 50
		`, userID)
		if err == nil {
			var calEvents []string
			for calRows.Next() {
				var title, location string
				var startTime, endTime time.Time
				var allDay bool
				if calRows.Scan(&title, &startTime, &endTime, &allDay, &location) == nil {
					calEvents = append(calEvents, formatAICalendarEvent(startTime, endTime, allDay, title, location))
				}
			}
			calRows.Close()
			if len(calEvents) > 0 {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString("=== КАЛЕНДАРЬ (30 дней назад — 30 дней вперёд; это план, не факт) ===\n")
				for _, e := range calEvents {
					sb.WriteString(e + "\n")
				}
			}
		}
	}

	// === ПОГОДА ===
	if scope.weather && h.weather != nil {
		if wd, err := h.weather.Fetch(0, 0, ""); err == nil {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString("=== ПОГОДА ===\n")
			sb.WriteString(fmt.Sprintf("Город: %s\n", wd.City))
			sb.WriteString(fmt.Sprintf("Сейчас: %.1f°C, ощущается %.1f°C, %s\n", wd.Temp, wd.FeelsLike, wd.Description))
			sb.WriteString(fmt.Sprintf("Влажность: %d%%, ветер %.1f км/ч\n", wd.Humidity, wd.WindSpeed))
			if len(wd.Daily) > 0 {
				sb.WriteString("Прогноз:\n")
				for _, d := range wd.Daily {
					sb.WriteString(fmt.Sprintf("  %s: %s, макс %.0f°C, мин %.0f°C\n",
						d.Date, wmoDescription(d.WeatherCode), d.TempMax, d.TempMin))
				}
			}
		}
	}

	return sb.String(), nil
}

func defaultAIContextScope() aiContextScope {
	return aiContextScope{
		finance:      true,
		productivity: true,
		activities:   true,
		health:       true,
		workouts:     true,
		routines:     true,
		habits:       true,
		nutrition:    true,
		journal:      true,
		calendar:     true,
		weather:      true,
	}
}

func (s aiContextScope) empty() bool {
	return !s.finance && !s.productivity && !s.activities && !s.health && !s.workouts && !s.routines && !s.habits && !s.nutrition && !s.journal && !s.calendar && !s.weather
}

func (s aiContextScope) sectionNames() []string {
	names := make([]string, 0, 11)
	if s.finance {
		names = append(names, "финансы")
	}
	if s.productivity {
		names = append(names, "продуктивность")
	}
	if s.activities {
		names = append(names, "активности")
	}
	if s.health {
		names = append(names, "здоровье")
	}
	if s.workouts {
		names = append(names, "тренировки")
	}
	if s.routines {
		names = append(names, "hevy routines")
	}
	if s.habits {
		names = append(names, "привычки")
	}
	if s.nutrition {
		names = append(names, "питание")
	}
	if s.journal {
		names = append(names, "дневник")
	}
	if s.calendar {
		names = append(names, "календарь")
	}
	if s.weather {
		names = append(names, "погода")
	}
	if len(names) == 0 {
		return defaultAIContextScope().sectionNames()
	}
	return names
}

func selectAIContextScope(message string, history []ChatMessage) aiContextScope {
	scope := aiContextScope{}
	text := strings.ToLower(message)
	recentHistory := recentHistoryText(history, 6)
	combined := strings.TrimSpace(strings.Join([]string{text, recentHistory}, "\n"))

	financeKeywords := []string{"финанс", "деньг", "расход", "доход", "баланс", "трат", "бюджет", "транзак", "счет", "счёт", "руб"}
	productivityKeywords := []string{"задач", "todo", "task", "todoist", "продуктив", "дедлайн", "срок", "просроч", "сделать сегодня", "перегруз", "нагрузка по задачам", "completed today", "overdue", "висят", "висит", "план на день", "план задач"}
	activityKeywords := []string{"актив", "бег", "пробеж", "килом", "км", "ходьб", "вел", "плав", "дистанц", "шаг", "strava", "run", "ride"}
	healthKeywords := []string{"здоров", "сон", "спал", "сплю", "пульс", "сердц", "hrv", "вес", "взвеш", "шаг", "apple health", "health", "zepp", "amazfit", "кислород", "spo2", "vo2"}
	workoutKeywords := []string{"тренир", "упражнен", "жим", "тяга", "присед", "гантел", "штанг", "блин", "гриф", "подход", "повтор", "hevy", "workout", "pull", "push", "legs", "зал", "вес"}
	routineKeywords := []string{"routine", "routines", "рутин", "шаблон", "сплит", "программ", "план трениров", "template"}
	habitKeywords := []string{"привыч", "habit", "habitify", "todoist", "зуб", "умы", "лиц", "уход", "skincare", "cleanser", "чеклист", "дейли", "daily"}
	nutritionKeywords := []string{"питан", "калор", "кбжу", "бжу", "еда", "ккал", "углев", "белк", "жир", "fatsecret", "myfitnesspal", "mfp", "вода", "воды", "hydration", "hydrated"}
	journalKeywords := []string{"дневник", "journal", "ноушн", "notion", "настроен", "рефлекс", "запис"}
	calendarKeywords := []string{"календар", "встреч", "событи", "созвон", "митинг", "расписан", "план"}
	weatherKeywords := []string{"погод", "температ", "дожд", "ветер", "на улице"}
	generalKeywords := []string{"сводк", "обзор", "проанализ", "анализ", "итог", "общ", "что происходит", "что нового", "удивил"}

	if containsAny(combined, financeKeywords...) {
		scope.finance = true
	}
	if containsAny(combined, productivityKeywords...) {
		scope.productivity = true
	}
	if containsAny(combined, activityKeywords...) {
		scope.activities = true
	}
	if containsAny(combined, healthKeywords...) {
		scope.health = true
	}
	if containsAny(combined, workoutKeywords...) {
		scope.workouts = true
	}
	if containsAny(combined, routineKeywords...) {
		scope.routines = true
	}
	if containsAny(combined, habitKeywords...) {
		scope.habits = true
	}
	if containsAny(combined, nutritionKeywords...) {
		scope.nutrition = true
	}
	if containsAny(combined, journalKeywords...) {
		scope.journal = true
	}
	if containsAny(combined, calendarKeywords...) {
		scope.calendar = true
	}
	if containsAny(combined, weatherKeywords...) {
		scope.weather = true
	}

	if strings.Contains(combined, "фитнес") || strings.Contains(combined, "нагруз") {
		scope.activities = true
		scope.workouts = true
	}

	if scope.empty() && containsAny(combined, generalKeywords...) {
		return defaultAIContextScope()
	}
	if scope.empty() {
		return defaultAIContextScope()
	}
	return scope
}

func recentHistoryText(history []ChatMessage, limit int) string {
	if len(history) == 0 || limit <= 0 {
		return ""
	}
	start := len(history) - limit
	if start < 0 {
		start = 0
	}
	parts := make([]string, 0, len(history)-start)
	for _, msg := range history[start:] {
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		parts = append(parts, strings.ToLower(msg.Content))
	}
	return strings.Join(parts, "\n")
}

func containsAny(text string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func (h *AIHandler) buildRecentWorkoutContext(ctx context.Context, userID string) (string, error) {
	return h.buildRecentWorkoutContextLimit(ctx, userID, 10)
}

func (h *AIHandler) buildRecentWorkoutContextLimit(ctx context.Context, userID string, limit int) (string, error) {
	data, err := h.buildRecentWorkoutsData(ctx, userID, limit)
	if err != nil {
		return "", err
	}
	return renderRecentWorkoutsText("", data), nil
}

func formatAIWorkoutContext(workout Workout) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\nТренировка %s: %s\n", formatAITimestampLocal(workout.StartedAt, "02.01.2006 15:04"), workout.Title))
	if workout.Notes != "" {
		sb.WriteString(fmt.Sprintf("  Заметки: %s\n", truncateAIText(workout.Notes, 240)))
	}
	if len(workout.Exercises) == 0 {
		sb.WriteString("  Деталей по упражнениям и подходам нет.\n")
		return sb.String()
	}

	for _, ex := range workout.Exercises {
		exerciseHeader := ex.Name
		if ex.Category != "" {
			exerciseHeader += " (" + ex.Category + ")"
		}
		if ex.Index > 0 {
			exerciseHeader += fmt.Sprintf(" [блок %d]", ex.Index)
		}
		sb.WriteString(fmt.Sprintf("  %s:\n", exerciseHeader))
		if ex.Notes != "" {
			sb.WriteString(fmt.Sprintf("    Заметки: %s\n", truncateAIText(ex.Notes, 180)))
		}

		for _, set := range ex.Sets {
			parts := make([]string, 0, 4)
			if set.WeightKg != nil || set.Reps != nil {
				weight := "-"
				reps := "-"
				if set.WeightKg != nil {
					weight = formatAIFloat(*set.WeightKg)
				}
				if set.Reps != nil {
					reps = strconv.Itoa(*set.Reps)
				}
				parts = append(parts, fmt.Sprintf("%s кг x %s", weight, reps))
			}
			if set.DistanceMeters != nil {
				parts = append(parts, fmt.Sprintf("%s м", formatAIFloat(*set.DistanceMeters)))
			}
			if set.DurationSeconds != nil {
				parts = append(parts, fmt.Sprintf("%d сек", *set.DurationSeconds))
			}
			if set.RPE != nil {
				parts = append(parts, fmt.Sprintf("RPE %s", formatAIFloat(*set.RPE)))
			}
			if len(parts) == 0 {
				parts = append(parts, "без числовых метрик")
			}

			sb.WriteString(fmt.Sprintf("    Подход %d: %s", set.SetIndex, strings.Join(parts, ", ")))
			if set.SetType != "" && set.SetType != "normal" {
				sb.WriteString(fmt.Sprintf(" [%s]", set.SetType))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func formatAIJournalEntry(entry aiJournalEntry) string {
	return formatAIJournalEntryWithLimit(entry, aiJournalDefaultLimit)
}

func formatAIJournalEntryWithLimit(entry aiJournalEntry, contentLimit int) string {
	line := fmt.Sprintf("  %s: %s", entry.Date.Format("02.01.2006"), strings.TrimSpace(entry.Title))
	if strings.TrimSpace(entry.Title) == "" {
		line = fmt.Sprintf("  %s: (без названия)", entry.Date.Format("02.01.2006"))
	}
	if entry.Mood != nil {
		line += fmt.Sprintf(" (настроение: %d/10)", *entry.Mood)
	}
	if len(entry.Tags) > 0 {
		line += " [" + strings.Join(entry.Tags, ", ") + "]"
	}
	content := strings.TrimSpace(entry.Content)
	if contentLimit > 0 && len(content) > contentLimit {
		content = content[:contentLimit] + "..."
	}
	if content != "" {
		line += "\n    " + strings.ReplaceAll(content, "\n", "\n    ")
	}
	return line
}

func writeAIJournalEntries(sb *strings.Builder, entries []aiJournalEntry) {
	writeAIJournalEntriesWithLimit(sb, entries, aiJournalDefaultLimit)
}

func writeAIJournalEntriesWithLimit(sb *strings.Builder, entries []aiJournalEntry, contentLimit int) {
	for _, entry := range entries {
		sb.WriteString(formatAIJournalEntryWithLimit(entry, contentLimit))
		sb.WriteString("\n")
	}
}

func writeAIJournalEntriesWithinBudget(sb *strings.Builder, entries []aiJournalEntry, contentLimit int, totalLimit int, maxEntries int) (written int, omitted int) {
	if maxEntries > 0 && len(entries) > maxEntries {
		omitted += len(entries) - maxEntries
		entries = entries[:maxEntries]
	}

	total := 0
	for i, entry := range entries {
		formatted := formatAIJournalEntryWithLimit(entry, contentLimit) + "\n"
		if totalLimit > 0 && total+len(formatted) > totalLimit {
			omitted += len(entries) - i
			break
		}
		sb.WriteString(formatted)
		total += len(formatted)
		written++
	}

	return written, omitted
}

func (h *AIHandler) loadAIJournalEntries(ctx context.Context, userID string, days, limit int) ([]aiJournalEntry, error) {
	since := aiNow().AddDate(0, 0, -days)
	rows, err := h.db.Query(ctx, `
		SELECT date, title, content, tags, mood, COALESCE(source, '')
		FROM journal_entries
		WHERE user_id = $1
			AND date >= $2
		ORDER BY date DESC, updated_at DESC
		LIMIT $3
	`, userID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []aiJournalEntry
	for rows.Next() {
		var entry aiJournalEntry
		if rows.Scan(&entry.Date, &entry.Title, &entry.Content, &entry.Tags, &entry.Mood, &entry.Source) == nil {
			entries = append(entries, entry)
		}
	}
	return entries, rows.Err()
}

func (h *AIHandler) loadAILatestJournalEntries(ctx context.Context, userID string, limit int) ([]aiJournalEntry, error) {
	rows, err := h.db.Query(ctx, `
		SELECT date, title, content, tags, mood, COALESCE(source, '')
		FROM journal_entries
		WHERE user_id = $1
		ORDER BY date DESC, updated_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []aiJournalEntry
	for rows.Next() {
		var entry aiJournalEntry
		if rows.Scan(&entry.Date, &entry.Title, &entry.Content, &entry.Tags, &entry.Mood, &entry.Source) == nil {
			entries = append(entries, entry)
		}
	}
	return entries, rows.Err()
}

func truncateAIText(value string, maxLen int) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + "..."
}

func formatAIFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", value), "0"), ".")
}

func aiTimeLocation() *time.Location {
	return aiDisplayLocation
}

func aiTime(t time.Time) time.Time {
	return t.In(aiTimeLocation())
}

func aiNow() time.Time {
	return aiTime(time.Now())
}

func formatAITimestampLocal(t time.Time, layout string) string {
	return aiTime(t).Format(layout)
}

func formatAICalendarEvent(startTime, endTime time.Time, allDay bool, title, location string) string {
	locationPart := ""
	if location != "" {
		locationPart = " @ " + location
	}
	if allDay {
		return fmt.Sprintf("  %s: %s (весь день)%s", formatAITimestampLocal(startTime, "02.01"), title, locationPart)
	}
	return fmt.Sprintf("  %s %s-%s: %s%s",
		formatAITimestampLocal(startTime, "02.01"),
		formatAITimestampLocal(startTime, "15:04"),
		formatAITimestampLocal(endTime, "15:04"),
		title,
		locationPart,
	)
}

func (h *AIHandler) buildRoutineContext(ctx context.Context, userID string, limit int) (string, error) {
	data, err := h.buildRoutineOverviewData(ctx, userID, limit)
	if err != nil {
		return "", err
	}
	return renderRoutineOverviewText("", data), nil
}
