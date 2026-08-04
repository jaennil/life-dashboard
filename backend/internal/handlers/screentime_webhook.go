package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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
	During    string `json:"during"`
	// ScreenTime holds an apps-only list when Websites is also sent, and the
	// combined "Apps & Websites" list otherwise.
	ScreenTime string `json:"screen_time"`
	Apps       string `json:"apps"`
	Websites   string `json:"websites"`
	Combined   string `json:"combined"`
}

type screenTimeCaptureResponse struct {
	Status         string   `json:"status"`
	Day            string   `json:"day"`
	IsPartial      bool     `json:"is_partial"`
	AppsSaved      int      `json:"apps_saved"`
	WebsitesSaved  int      `json:"websites_saved"`
	AppSeconds     int      `json:"app_seconds"`
	WebsiteSeconds int      `json:"website_seconds"`
	UnparsedLines  []string `json:"unparsed_lines"`
	ReceivedBytes  int      `json:"received_bytes"`
	JSONValid      bool     `json:"json_valid"`
	Repaired       bool     `json:"repaired"`
	TopLevelKeys   []string `json:"top_level_keys"`
	RawPreview     string   `json:"raw_preview"`
	Truncated      bool     `json:"truncated"`
	RawEventID     string   `json:"raw_event_id"`
}

// POST /api/v1/webhook/screentime — ingest Screen Time from the iOS 26 Shortcuts
// action "Get App & Website Activity".
//
// The body is stored verbatim in raw_events before anything is interpreted, so a
// payload can always be re-parsed later without asking the phone for it again.
func (h *ScreenTimeWebhookHandler) ReceiveData(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Shortcuts can emit literal newlines inside JSON string values. Escape them
	// instead of deleting them: the Screen Time payload is newline-separated, so
	// stripping would destroy the line boundaries the parser needs.
	var envelope screenTimeEnvelope
	parsed := json.RawMessage(nil)
	jsonValid := false
	repaired := false
	if json.Valid(body) && json.Unmarshal(body, &envelope) == nil {
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

	items, unparsed := collectScreenTimeItems(envelope)

	day, isPartial, err := resolveScreenTimeDay(envelope)
	if err != nil {
		h.logger.Warn().
			Err(err).
			Str("user_id", userID).
			Str("event_type", eventType).
			Str("raw_event_id", rawEventID).
			Msg("screen time payload has an unusable day field")
		resp := screenTimeCaptureResponse{Status: "bad_day_field", ReceivedBytes: len(body), JSONValid: jsonValid, Repaired: repaired, RawEventID: rawEventID, UnparsedLines: unparsed, TopLevelKeys: topLevelKeys(parsed)}
		resp.RawPreview, resp.Truncated = previewBody(body)
		writeJSON(w, resp)
		return
	}

	resp := screenTimeCaptureResponse{
		Status:        "ok",
		Day:           day.Format("2006-01-02"),
		IsPartial:     isPartial,
		UnparsedLines: unparsed,
		ReceivedBytes: len(body),
		JSONValid:     jsonValid,
		Repaired:      repaired,
		TopLevelKeys:  topLevelKeys(parsed),
		RawEventID:    rawEventID,
	}
	resp.RawPreview, resp.Truncated = previewBody(body)

	// An empty item list means the Shortcuts variable did not get substituted.
	// Never let that wipe a day that was already ingested correctly.
	if len(items) == 0 {
		resp.Status = "no_items_parsed"
		h.logger.Warn().
			Str("user_id", userID).
			Str("day", resp.Day).
			Str("event_type", eventType).
			Int("bytes", len(body)).
			Int("unparsed", len(unparsed)).
			Msg("screen time payload had no parsable items")
		writeJSON(w, resp)
		return
	}

	// Foreground time does not overlap, so a single day cannot exceed 24h. A
	// larger total means the Shortcut asked for an aggregate window (thisWeek,
	// thisMonth, inBetween) instead of one day, which would otherwise be stored
	// as a single absurd day. Refuse it rather than corrupt the series.
	summary := summarizeScreenTimeItems(items)
	resp.AppSeconds = summary.appSeconds
	resp.WebsiteSeconds = summary.websiteSeconds
	if summary.appSeconds > screenTimeMaxSeconds || summary.websiteSeconds > screenTimeMaxSeconds {
		resp.Status = "day_total_exceeds_24h"
		h.logger.Warn().
			Str("user_id", userID).
			Str("day", resp.Day).
			Str("event_type", eventType).
			Int("app_seconds", summary.appSeconds).
			Int("website_seconds", summary.websiteSeconds).
			Msg("screen time payload looks like a multi-day aggregate, refusing")
		writeJSON(w, resp)
		return
	}

	if existing := h.storedScreenTimeDay(r.Context(), userID, day); isDegradedReread(summary, existing) {
		resp.Status = "stale_reread_ignored"
		h.logger.Warn().
			Str("user_id", userID).
			Str("day", resp.Day).
			Str("event_type", eventType).
			Int("incoming_items", summary.itemCount()).
			Int("stored_items", existing.itemCount).
			Int("incoming_app_seconds", summary.appSeconds).
			Int("stored_app_seconds", existing.appSeconds).
			Msg("screen time re-read is poorer than the stored day, keeping the stored one")
		writeJSON(w, resp)
		return
	}

	saved, err := h.persistScreenTime(r.Context(), userID, day, isPartial, summary, items, len(unparsed), stored)
	if err != nil {
		h.logger.Error().Err(err).Str("day", resp.Day).Msg("persist screen time failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp.AppsSaved = saved.appCount
	resp.WebsitesSaved = saved.websiteCount
	resp.AppSeconds = saved.appSeconds
	resp.WebsiteSeconds = saved.websiteSeconds

	h.logger.Info().
		Str("user_id", userID).
		Str("day", resp.Day).
		Str("event_type", eventType).
		Bool("is_partial", isPartial).
		Int("apps", saved.appCount).
		Int("websites", saved.websiteCount).
		Int("app_seconds", saved.appSeconds).
		Int("website_seconds", saved.websiteSeconds).
		Int("unparsed", len(unparsed)).
		Bool("clamped", saved.clamped).
		Bool("repaired", repaired).
		Msg("screen time ingested")

	writeJSON(w, resp)
}

// collectScreenTimeItems reads every usage field the payload may carry. When a
// dedicated websites field is present the screen_time field is authoritative
// apps-only data; otherwise it is the combined list and kinds get inferred.
func collectScreenTimeItems(envelope screenTimeEnvelope) ([]screenTimeItem, []string) {
	hasWebsiteField := strings.TrimSpace(envelope.Websites) != ""

	screenTimeKind := ""
	if hasWebsiteField || strings.TrimSpace(envelope.Apps) != "" {
		screenTimeKind = screenTimeKindApp
	}

	fields := []struct {
		blob string
		kind string
	}{
		{envelope.Apps, screenTimeKindApp},
		{envelope.Websites, screenTimeKindWebsite},
		{envelope.ScreenTime, screenTimeKind},
		{envelope.Combined, ""},
	}

	items := []screenTimeItem{}
	unparsed := []string{}
	for _, field := range fields {
		if strings.TrimSpace(field.blob) == "" {
			continue
		}
		result := parseScreenTimeBlob(field.blob, field.kind)
		items = append(items, result.Items...)
		unparsed = append(unparsed, result.Unparsed...)
	}
	return items, unparsed
}

// resolveScreenTimeDay trusts an explicit day field over anything derived on the
// server, because the Shortcut resolves "yesterday" in the phone's timezone.
//
// An unparsable day field is an error rather than a reason to fall back: during a
// backfill it would quietly file one day's numbers under yesterday and overwrite
// good data.
func resolveScreenTimeDay(envelope screenTimeEnvelope) (time.Time, bool, error) {
	now := time.Now().In(aiDisplayLocation)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, aiDisplayLocation)

	if explicit := strings.TrimSpace(envelope.Day); explicit != "" {
		parsedDay, err := parseDate(explicit)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("unparsable day %q: %w", explicit, err)
		}
		day := time.Date(parsedDay.Year(), parsedDay.Month(), parsedDay.Day(), 0, 0, 0, 0, aiDisplayLocation)
		return day, day.Equal(today), nil
	}

	if strings.EqualFold(strings.TrimSpace(envelope.During), "today") {
		return today, true, nil
	}
	return today.AddDate(0, 0, -1), false, nil
}

type screenTimeSaveResult struct {
	appCount       int
	websiteCount   int
	appSeconds     int
	websiteSeconds int
	clamped        bool
}

func (r screenTimeSaveResult) itemCount() int {
	return r.appCount + r.websiteCount
}

type screenTimeStoredDay struct {
	exists     bool
	itemCount  int
	appSeconds int
}

// storedScreenTimeDay reads what is already recorded for a day so a degraded
// re-read can be recognized before it overwrites anything.
func (h *ScreenTimeWebhookHandler) storedScreenTimeDay(ctx context.Context, userID string, day time.Time) screenTimeStoredDay {
	var stored screenTimeStoredDay
	err := h.db.QueryRow(ctx, `
		SELECT app_count + website_count, app_seconds
		FROM screen_time_daily
		WHERE user_id = $1 AND source = $2 AND day = $3::date
	`, userID, screenTimeSource, day).Scan(&stored.itemCount, &stored.appSeconds)
	if err != nil {
		return screenTimeStoredDay{}
	}
	stored.exists = true
	return stored
}

// isDegradedReread reports whether an incoming payload is strictly poorer than
// what is already stored for the day.
//
// Screen Time only keeps about 30 days on the device and trims the oldest day as
// the window slides, so re-reading a day near that edge legitimately returns less
// than it did before. Replacing the stored day with that answer loses history the
// device can no longer produce again, and this database is the only long-term
// archive of it. A payload with both fewer items and less app time is therefore
// treated as an eroded re-read, not a correction.
func isDegradedReread(incoming screenTimeSaveResult, stored screenTimeStoredDay) bool {
	if !stored.exists {
		return false
	}
	return incoming.itemCount() < stored.itemCount && incoming.appSeconds < stored.appSeconds
}

func summarizeScreenTimeItems(items []screenTimeItem) screenTimeSaveResult {
	var summary screenTimeSaveResult
	for _, item := range items {
		if item.Kind == screenTimeKindWebsite {
			summary.websiteCount++
			summary.websiteSeconds += item.Seconds
		} else {
			summary.appCount++
			summary.appSeconds += item.Seconds
		}
		summary.clamped = summary.clamped || item.Clamped
	}
	return summary
}

func (h *ScreenTimeWebhookHandler) persistScreenTime(
	ctx context.Context,
	userID string,
	day time.Time,
	isPartial bool,
	saved screenTimeSaveResult,
	items []screenTimeItem,
	unparsedLines int,
	rawPayload []byte,
) (screenTimeSaveResult, error) {
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return saved, err
	}
	defer tx.Rollback(ctx)

	// Each payload is a full snapshot of the day, and Apple retroactively drops
	// history for uninstalled apps, so replace the day rather than upserting into
	// it and leaving stale rows behind.
	if _, err := tx.Exec(ctx, `
		DELETE FROM screen_time_app_usage
		WHERE user_id = $1 AND source = $2 AND day = $3::date
	`, userID, screenTimeSource, day); err != nil {
		return saved, err
	}

	rows := make([][]any, 0, len(items))
	for _, item := range items {
		rows = append(rows, []any{
			userID, screenTimeSource, day, item.Kind,
			item.ItemKey, item.DisplayName, item.Seconds, item.KindInferred,
		})
	}
	if _, err := tx.CopyFrom(ctx,
		pgx.Identifier{"screen_time_app_usage"},
		[]string{"user_id", "source", "day", "kind", "item_key", "display_name", "seconds", "kind_inferred"},
		pgx.CopyFromRows(rows),
	); err != nil {
		return saved, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO screen_time_daily (
			user_id, source, day, app_seconds, website_seconds,
			app_count, website_count, unparsed_lines, clamped, is_partial, raw_payload
		)
		VALUES ($1, $2, $3::date, $4, $5, $6, $7, $8, $9, $10, $11::jsonb)
		ON CONFLICT (user_id, source, day) DO UPDATE SET
			app_seconds = EXCLUDED.app_seconds,
			website_seconds = EXCLUDED.website_seconds,
			app_count = EXCLUDED.app_count,
			website_count = EXCLUDED.website_count,
			unparsed_lines = EXCLUDED.unparsed_lines,
			clamped = EXCLUDED.clamped,
			is_partial = EXCLUDED.is_partial,
			raw_payload = EXCLUDED.raw_payload,
			updated_at = NOW()
	`, userID, screenTimeSource, day, saved.appSeconds, saved.websiteSeconds,
		saved.appCount, saved.websiteCount, unparsedLines, saved.clamped, isPartial, rawPayload); err != nil {
		return saved, err
	}

	return saved, tx.Commit(ctx)
}

func previewBody(body []byte) (string, bool) {
	preview := string(body)
	if len(preview) > rawPreviewLimit {
		return preview[:rawPreviewLimit], true
	}
	return preview, false
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
