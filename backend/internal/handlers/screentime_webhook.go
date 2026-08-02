package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const screenTimeSource = "ios_screentime"

// rawPreviewLimit caps how much of the body we echo back so the response stays
// readable inside the Shortcuts result sheet.
const rawPreviewLimit = 4000

type ScreenTimeWebhookHandler struct {
	db     *pgxpool.Pool
	logger zerolog.Logger
}

func NewScreenTimeWebhook(db *pgxpool.Pool, logger zerolog.Logger) *ScreenTimeWebhookHandler {
	return &ScreenTimeWebhookHandler{db: db, logger: logger.With().Str("handler", "screentime_webhook").Logger()}
}

type screenTimeEnvelope struct {
	APIKey    string `json:"api_key"`
	Source    string `json:"source"`
	EventType string `json:"event_type"`
	Day       string `json:"day"`
}

type screenTimeCaptureResponse struct {
	Status        string   `json:"status"`
	ReceivedBytes int      `json:"received_bytes"`
	JSONValid     bool     `json:"json_valid"`
	Repaired      bool     `json:"repaired"`
	TopLevelKeys  []string `json:"top_level_keys"`
	RawPreview    string   `json:"raw_preview"`
	Truncated     bool     `json:"truncated"`
	RawEventID    string   `json:"raw_event_id"`
}

// POST /api/v1/webhook/screentime — capture Screen Time payloads from the iOS 26
// Shortcuts action "Get App & Website Activity".
//
// The output of that action has no documented serialization, so this endpoint
// deliberately stores the body verbatim in raw_events before interpreting
// anything, and echoes it back so the shape can be inspected from the phone.
// Parsing into typed tables comes once the real format is known.
func (h *ScreenTimeWebhookHandler) ReceiveData(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Shortcuts emits literal newlines inside JSON string values. Escape them
	// instead of deleting them: the Screen Time payload is newline-separated,
	// so stripping would destroy the line boundaries we need.
	var envelope screenTimeEnvelope
	parsed := json.RawMessage(nil)
	jsonValid := false
	repaired := false
	if json.Unmarshal(body, &envelope) == nil && json.Valid(body) {
		jsonValid = true
		parsed = json.RawMessage(body)
	} else if fixed := escapeLiteralNewlines(body); json.Valid(fixed) {
		if json.Unmarshal(fixed, &envelope) == nil {
			jsonValid = true
			repaired = true
			parsed = json.RawMessage(fixed)
		}
	}

	apiKey := healthAPIKeyFromRequest(r, envelope.APIKey)
	if apiKey == "" {
		http.Error(w, "api_key required", http.StatusUnauthorized)
		return
	}

	var userID string
	if err := h.db.QueryRow(r.Context(),
		`SELECT user_id FROM api_keys WHERE key = $1`, apiKey).Scan(&userID); err != nil {
		http.Error(w, "invalid api_key", http.StatusUnauthorized)
		return
	}

	source := normalizeHealthSource(firstNonEmpty(envelope.Source, screenTimeSource))
	eventType := firstNonEmpty(envelope.EventType, "capture")
	if len(eventType) > 100 {
		eventType = eventType[:100]
	}

	stored, _ := json.Marshal(map[string]any{
		"raw":          string(body),
		"parsed":       parsed,
		"content_type": r.Header.Get("Content-Type"),
		"json_valid":   jsonValid,
		"repaired":     repaired,
	})

	var rawEventID string
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO raw_events (source, event_type, external_id, payload, user_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, source, eventType, time.Now().UTC().Format(time.RFC3339Nano), stored, userID).Scan(&rawEventID)
	if err != nil {
		h.logger.Error().Err(err).Msg("store screen time raw event failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if _, err := h.db.Exec(r.Context(), `
		INSERT INTO sync_state (source, last_synced_at, updated_at, enabled, user_id)
		VALUES ($1, NOW(), NOW(), TRUE, $2)
		ON CONFLICT (source, user_id) DO UPDATE SET
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at = NOW(),
			enabled = TRUE
	`, screenTimeSource, userID); err != nil {
		h.logger.Warn().Err(err).Msg("update screen time sync state failed")
	}

	preview := string(body)
	truncated := false
	if len(preview) > rawPreviewLimit {
		preview = preview[:rawPreviewLimit]
		truncated = true
	}

	h.logger.Info().
		Str("user_id", userID).
		Str("event_type", eventType).
		Int("bytes", len(body)).
		Bool("json_valid", jsonValid).
		Bool("repaired", repaired).
		Msg("screen time payload captured")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(screenTimeCaptureResponse{
		Status:        "ok",
		ReceivedBytes: len(body),
		JSONValid:     jsonValid,
		Repaired:      repaired,
		TopLevelKeys:  topLevelKeys(parsed),
		RawPreview:    preview,
		Truncated:     truncated,
		RawEventID:    rawEventID,
	})
}

// escapeLiteralNewlines rewrites raw control characters that appear inside JSON
// string literals into their escaped form, leaving structural whitespace alone.
func escapeLiteralNewlines(body []byte) []byte {
	out := make([]byte, 0, len(body)+16)
	inString := false
	escaped := false
	for _, b := range body {
		if escaped {
			out = append(out, b)
			escaped = false
			continue
		}
		switch {
		case b == '\\' && inString:
			out = append(out, b)
			escaped = true
		case b == '"':
			inString = !inString
			out = append(out, b)
		case inString && b == '\n':
			out = append(out, '\\', 'n')
		case inString && b == '\r':
			out = append(out, '\\', 'r')
		case inString && b == '\t':
			out = append(out, '\\', 't')
		default:
			out = append(out, b)
		}
	}
	return out
}

func topLevelKeys(payload json.RawMessage) []string {
	if len(payload) == 0 {
		return []string{}
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(payload, &obj); err != nil {
		return []string{}
	}
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
