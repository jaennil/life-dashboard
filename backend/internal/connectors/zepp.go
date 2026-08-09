package connectors

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	zeppSource              = "zepp"
	zeppRequestTimeout      = 60 * time.Second
	zeppInitialBackfillDays = 90
	zeppIncrementalLookback = 3
	zeppEventPageLimit      = 1000
	zeppTimezone            = "Europe/Moscow"
	zeppCountry             = "RU"
)

type zeppSession struct {
	appToken string
	userID   string
}

type ZeppConnector struct {
	db     *pgxpool.Pool
	client *http.Client
	logger zerolog.Logger
}

func NewZepp(db *pgxpool.Pool, logger zerolog.Logger) *ZeppConnector {
	return &ZeppConnector{
		db:     db,
		client: &http.Client{Timeout: zeppRequestTimeout},
		logger: logger.With().Str("connector", "zepp").Logger(),
	}
}

func (z *ZeppConnector) Name() string { return zeppSource }

func (z *ZeppConnector) Sync(ctx context.Context, userID string) error {
	session, err := z.loadSession(ctx, userID)
	if err != nil {
		return err
	}

	from, to := z.syncWindow(ctx, userID)
	days, err := z.fetchBandData(ctx, session, from, to)
	if err != nil {
		if isZeppAuthError(err) {
			return fmt.Errorf("zepp rejected the app token — provision a fresh one in Settings: %w", err)
		}
		return fmt.Errorf("band data: %w", err)
	}

	metrics, sleeps, heartRates := 0, 0, 0
	for _, day := range days {
		saved, slept, hrCount, err := z.storeBandDay(ctx, userID, day)
		if err != nil {
			z.logger.Warn().Err(err).Str("day", day.DateTime).Msg("store zepp day failed")
			continue
		}
		metrics += saved
		heartRates += hrCount
		if slept {
			sleeps++
		}
	}

	// Each section is optional: a watch that never recorded stress or SpO2 simply
	// returns nothing, and one failing section must not lose the others.
	stress := z.ingestSection(ctx, userID, session, "all_day_stress", from, to, z.storeStress)
	pai := z.ingestSection(ctx, userID, session, "PaiHealthInfo", from, to, z.storePAI)
	oxygen := z.ingestSection(ctx, userID, session, "blood_oxygen", from, to, z.storeOxygen)

	z.logger.Info().
		Str("user_id", userID).
		Str("from", from.Format("2006-01-02")).
		Str("to", to.Format("2006-01-02")).
		Int("days", len(days)).
		Int("metrics", metrics).
		Int("heart_rate_samples", heartRates).
		Int("sleep_sessions", sleeps).
		Int("stress", stress).
		Int("pai", pai).
		Int("spo2", oxygen).
		Msg("zepp sync finished")
	return nil
}

// loadSession reads the provisioned app token. Zepp's password login is a
// separate v2 flow whose payload is AES-encrypted with a hardcoded key, and it
// rate-limits per account after a couple of attempts (429 {"code":12}). This
// connector therefore never logs in: the token is supplied once and used until
// Zepp rejects it.
func (z *ZeppConnector) loadSession(ctx context.Context, userID string) (zeppSession, error) {
	var session zeppSession
	err := z.db.QueryRow(ctx, `
		SELECT access_token, refresh_token
		FROM oauth_tokens
		WHERE source = $1 AND user_id = $2
	`, zeppSource, userID).Scan(&session.appToken, &session.userID)
	if err != nil {
		return zeppSession{}, fmt.Errorf("no Zepp app token — add the app token and Zepp user id in Settings")
	}
	session.appToken = strings.TrimSpace(session.appToken)
	session.userID = strings.TrimSpace(session.userID)
	if session.appToken == "" || session.userID == "" {
		return zeppSession{}, fmt.Errorf("incomplete Zepp credentials — both the app token and the Zepp user id are required")
	}
	return session, nil
}

func (z *ZeppConnector) syncWindow(ctx context.Context, userID string) (time.Time, time.Time) {
	now := time.Now()
	var lastSync *time.Time
	_ = z.db.QueryRow(ctx, `
		SELECT last_synced_at FROM sync_state WHERE source = $1 AND user_id = $2
	`, zeppSource, userID).Scan(&lastSync)

	if lastSync == nil || lastSync.IsZero() {
		return now.AddDate(0, 0, -zeppInitialBackfillDays), now
	}
	return lastSync.AddDate(0, 0, -zeppIncrementalLookback), now
}

func (z *ZeppConnector) fetchBandData(ctx context.Context, session zeppSession, from, to time.Time) ([]zeppBandDay, error) {
	body, err := z.get(ctx, session, zeppAPIHost+"/v1/data/band_data.json", url.Values{
		"query_type":  {"detail"},
		"device_type": {"android_phone"},
		"userid":      {session.userID},
		"from_date":   {from.Format("2006-01-02")},
		"to_date":     {to.Format("2006-01-02")},
	})
	if err != nil {
		return nil, err
	}

	var payload zeppBandDataResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode band data: %w", err)
	}
	return payload.Data, nil
}

func (z *ZeppConnector) fetchEvents(ctx context.Context, session zeppSession, eventType string, from, to time.Time) ([]json.RawMessage, error) {
	body, err := z.get(ctx, session, fmt.Sprintf("%s/users/%s/events", zeppAPIHost, session.userID), url.Values{
		"eventType": {eventType},
		"limit":     {fmt.Sprint(zeppEventPageLimit)},
		"from":      {fmt.Sprint(from.UnixMilli())},
		"to":        {fmt.Sprint(to.UnixMilli())},
		"timeZone":  {zeppTimezone},
	})
	if err != nil {
		return nil, err
	}

	var payload zeppEventsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode %s events: %w", eventType, err)
	}
	return payload.Items, nil
}

func (z *ZeppConnector) get(ctx context.Context, session zeppSession, endpoint string, params url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	for name, value := range zeppHeaders(session.appToken, newZeppRequestID(), zeppTimezone, zeppCountry) {
		req.Header.Set(name, value)
	}

	resp, err := z.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &zeppHTTPError{status: resp.StatusCode, body: string(body)}
	}
	return body, nil
}

func newZeppRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return strings.Repeat("0", 32)
	}
	return hex.EncodeToString(buf)
}

type zeppHTTPError struct {
	status int
	body   string
}

func (e *zeppHTTPError) Error() string {
	preview := e.body
	if len(preview) > 200 {
		preview = preview[:200]
	}
	return fmt.Sprintf("zepp http %d: %s", e.status, preview)
}

func isZeppAuthError(err error) bool {
	var httpErr *zeppHTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	return httpErr.status == http.StatusUnauthorized || httpErr.status == http.StatusForbidden
}
