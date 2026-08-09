package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	zeppSource = "zepp"
	// The obtained app token lives in its own oauth_tokens row, separate from the
	// login and password, so it can be provisioned or dropped independently.
	zeppSessionSource       = "zepp_session"
	zeppRequestTimeout      = 45 * time.Second
	zeppInitialBackfillDays = 90
	zeppIncrementalLookback = 3
	zeppEventPageLimit      = 1000
)

type zeppSession struct {
	appToken string
	userID   string
}

type ZeppConnector struct {
	db     *pgxpool.Pool
	client *http.Client
	logger zerolog.Logger

	mu       sync.Mutex
	sessions map[string]zeppSession
}

func NewZepp(db *pgxpool.Pool, logger zerolog.Logger) *ZeppConnector {
	return &ZeppConnector{
		db:       db,
		client:   &http.Client{Timeout: zeppRequestTimeout},
		logger:   logger.With().Str("connector", "zepp").Logger(),
		sessions: map[string]zeppSession{},
	}
}

func (z *ZeppConnector) Name() string { return zeppSource }

func (z *ZeppConnector) Sync(ctx context.Context, userID string) error {
	email, password, err := z.loadCredentials(ctx, userID)
	if err != nil {
		return err
	}

	session, err := z.session(ctx, userID, email, password)
	if err != nil {
		return err
	}

	from, to := z.syncWindow(ctx, userID)
	z.logger.Info().
		Str("user_id", userID).
		Str("from", from.Format("2006-01-02")).
		Str("to", to.Format("2006-01-02")).
		Msg("zepp sync window")

	// A stale cached session shows up as a refusal on the first call, so retry
	// once with a fresh login before giving up.
	days, err := z.fetchBandData(ctx, session, from, to)
	if err != nil && isZeppAuthError(err) {
		z.invalidate(ctx, userID)
		if session, err = z.session(ctx, userID, email, password); err != nil {
			return err
		}
		days, err = z.fetchBandData(ctx, session, from, to)
	}
	if err != nil {
		return fmt.Errorf("band data: %w", err)
	}

	metrics := 0
	sleeps := 0
	for _, day := range days {
		saved, slept, err := z.storeBandDay(ctx, userID, day)
		if err != nil {
			z.logger.Warn().Err(err).Str("day", day.DateTime).Msg("store zepp day failed")
			continue
		}
		metrics += saved
		if slept {
			sleeps++
		}
	}

	// Each of these is optional: a watch that never recorded stress or PAI simply
	// returns nothing, and one failing section must not lose the others.
	stress := z.ingestSection(ctx, userID, session, "all_day_stress", from, to, z.storeStress)
	pai := z.ingestSection(ctx, userID, session, "PaiHealthInfo", from, to, z.storePAI)
	oxygen := z.ingestSection(ctx, userID, session, "blood_oxygen", from, to, z.storeOxygen)

	z.logger.Info().
		Str("user_id", userID).
		Int("days", len(days)).
		Int("metrics", metrics).
		Int("sleep_sessions", sleeps).
		Int("stress", stress).
		Int("pai", pai).
		Int("spo2", oxygen).
		Msg("zepp sync finished")
	return nil
}

func (z *ZeppConnector) loadCredentials(ctx context.Context, userID string) (email, password string, err error) {
	// access_token holds the password and refresh_token the login, matching how
	// the other username/password integrations reuse the oauth_tokens row.
	err = z.db.QueryRow(ctx, `
		SELECT access_token, refresh_token
		FROM oauth_tokens
		WHERE source = $1 AND user_id = $2
	`, zeppSource, userID).Scan(&password, &email)
	if err != nil || strings.TrimSpace(email) == "" || strings.TrimSpace(password) == "" {
		return "", "", fmt.Errorf("no Zepp credentials — add your Zepp login and password in Settings")
	}
	return strings.TrimSpace(email), password, nil
}

// session resolves an app token, preferring anything already obtained. The Zepp
// login endpoint rate-limits extremely aggressively (a handful of attempts is
// enough for a 429 that lasts), so logging in is the last resort and its result
// is persisted: a pod restart must not cost another login.
func (z *ZeppConnector) session(ctx context.Context, userID, email, password string) (zeppSession, error) {
	z.mu.Lock()
	cached, ok := z.sessions[userID]
	z.mu.Unlock()
	if ok {
		return cached, nil
	}

	if stored, ok := z.loadSession(ctx, userID); ok {
		z.mu.Lock()
		z.sessions[userID] = stored
		z.mu.Unlock()
		return stored, nil
	}

	session, err := z.login(ctx, email, password)
	if err != nil {
		return zeppSession{}, err
	}
	z.saveSession(ctx, userID, session)

	z.mu.Lock()
	z.sessions[userID] = session
	z.mu.Unlock()
	return session, nil
}

// loadSession reads a previously obtained app token. It lives in its own
// oauth_tokens row so the credential row stays untouched and no migration is
// needed; a token can also be provisioned by hand from an unthrottled address.
func (z *ZeppConnector) loadSession(ctx context.Context, userID string) (zeppSession, bool) {
	var session zeppSession
	err := z.db.QueryRow(ctx, `
		SELECT access_token, refresh_token
		FROM oauth_tokens
		WHERE source = $1 AND user_id = $2
	`, zeppSessionSource, userID).Scan(&session.appToken, &session.userID)
	if err != nil || session.appToken == "" || session.userID == "" {
		return zeppSession{}, false
	}
	return session, true
}

func (z *ZeppConnector) saveSession(ctx context.Context, userID string, session zeppSession) {
	if _, err := z.db.Exec(ctx, `
		INSERT INTO oauth_tokens (source, access_token, refresh_token, expires_at, updated_at, user_id)
		VALUES ($1, $2, $3, NOW() + INTERVAL '100 years', NOW(), $4)
		ON CONFLICT (source, user_id) DO UPDATE SET
			access_token = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			updated_at = NOW()
	`, zeppSessionSource, session.appToken, session.userID, userID); err != nil {
		z.logger.Warn().Err(err).Msg("persist zepp session failed")
	}
}

// invalidate drops the token everywhere so the next attempt logs in again. Only
// called when Zepp actually refuses the token, never on transport errors.
func (z *ZeppConnector) invalidate(ctx context.Context, userID string) {
	z.mu.Lock()
	delete(z.sessions, userID)
	z.mu.Unlock()

	if _, err := z.db.Exec(ctx, `
		DELETE FROM oauth_tokens WHERE source = $1 AND user_id = $2
	`, zeppSessionSource, userID); err != nil {
		z.logger.Warn().Err(err).Msg("drop stale zepp session failed")
	}
}

// login performs the two-stage Huami handshake: an access token comes back in the
// Location header of a deliberately unfollowed redirect, then that token is
// exchanged for the long-lived app token.
func (z *ZeppConnector) login(ctx context.Context, email, password string) (zeppSession, error) {
	form := url.Values{
		"state":        {"REDIRECTION"},
		"client_id":    {"HuaMi"},
		"redirect_uri": {"https://s3-us-west-2.amazonws.com/hm-registration/successsignin.html"},
		"token":        {"access"},
		"password":     {password},
	}
	authURL := fmt.Sprintf(zeppAuthURLTemplate, url.PathEscape(email))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, strings.NewReader(form.Encode()))
	if err != nil {
		return zeppSession{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	noRedirect := &http.Client{
		Timeout: zeppRequestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		return zeppSession{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return zeppSession{}, fmt.Errorf("zepp login rate limited (429), try again later")
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return zeppSession{}, fmt.Errorf("zepp login returned no redirect (status %d)", resp.StatusCode)
	}
	redirect, err := url.Parse(location)
	if err != nil {
		return zeppSession{}, fmt.Errorf("parse zepp redirect: %w", err)
	}
	params := redirect.Query()
	accessToken := params.Get("access")
	countryCode := params.Get("country_code")
	if accessToken == "" {
		return zeppSession{}, fmt.Errorf("zepp login failed: %s", firstNonEmptyString(params.Get("error"), "no access token in redirect"))
	}

	loginForm := url.Values{
		"app_name":           {"com.xiaomi.hm.health"},
		"dn":                 {"account.huami.com,api-user.huami.com,api-mifit.huami.com"},
		"device_id":          {"02:00:00:00:00:00"},
		"device_model":       {"android_phone"},
		"app_version":        {"4.0.9"},
		"allow_registration": {"false"},
		"third_name":         {"huami"},
		"grant_type":         {"access_token"},
		"country_code":       {countryCode},
		"code":               {accessToken},
	}
	loginReq, err := http.NewRequestWithContext(ctx, http.MethodPost, zeppLoginURL, strings.NewReader(loginForm.Encode()))
	if err != nil {
		return zeppSession{}, err
	}
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	loginResp, err := z.client.Do(loginReq)
	if err != nil {
		return zeppSession{}, err
	}
	defer loginResp.Body.Close()

	var payload struct {
		TokenInfo struct {
			AppToken string `json:"app_token"`
			UserID   string `json:"user_id"`
		} `json:"token_info"`
		ErrorCode json.Number `json:"error_code"`
	}
	if err := json.NewDecoder(loginResp.Body).Decode(&payload); err != nil {
		return zeppSession{}, fmt.Errorf("decode zepp login: %w", err)
	}
	if payload.TokenInfo.AppToken == "" || payload.TokenInfo.UserID == "" {
		return zeppSession{}, fmt.Errorf("zepp login returned no app token (error_code %s)", payload.ErrorCode.String())
	}

	z.logger.Info().Msg("zepp login succeeded")
	return zeppSession{appToken: payload.TokenInfo.AppToken, userID: payload.TokenInfo.UserID}, nil
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
	params := url.Values{
		"query_type":  {"detail"},
		"device_type": {"android_phone"},
		"userid":      {session.userID},
		"from_date":   {from.Format("2006-01-02")},
		"to_date":     {to.Format("2006-01-02")},
	}

	body, err := z.get(ctx, session, zeppBandDataURL, params)
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
	params := url.Values{
		"eventType": {eventType},
		"limit":     {fmt.Sprint(zeppEventPageLimit)},
		"from":      {fmt.Sprint(from.UnixMilli())},
		"to":        {fmt.Sprint(to.UnixMilli())},
		"timeZone":  {"Europe/Moscow"},
	}

	endpoint := fmt.Sprintf(zeppEventsURLTemplate, session.userID)
	if eventType == "PaiHealthInfo" {
		// PAI lives on a different regional host than the other event types.
		endpoint = fmt.Sprintf(zeppPAIEventsURLTemplate, session.userID)
	}

	body, err := z.get(ctx, session, endpoint, params)
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
	req.Header.Set("apptoken", session.appToken)

	resp, err := z.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &zeppHTTPError{status: resp.StatusCode, body: string(body)}
	}
	return body, nil
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

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
