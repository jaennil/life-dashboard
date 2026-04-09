package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
	googleCalURL   = "https://www.googleapis.com/calendar/v3/calendars/primary/events"
	googleScope    = "https://www.googleapis.com/auth/calendar.readonly"
)

type GoogleCalendarConnector struct {
	clientID     string
	clientSecret string
	redirectURI  string
	db           *pgxpool.Pool
	client       *http.Client
	logger       zerolog.Logger
}

func NewGoogleCalendar(clientID, clientSecret, redirectURI string, db *pgxpool.Pool, logger zerolog.Logger) *GoogleCalendarConnector {
	return &GoogleCalendarConnector{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		db:           db,
		client:       &http.Client{Timeout: 30 * time.Second},
		logger:       logger.With().Str("connector", "google_calendar").Logger(),
	}
}

func (g *GoogleCalendarConnector) Name() string { return "google_calendar" }

func (g *GoogleCalendarConnector) AuthURL(state string) string {
	params := url.Values{
		"client_id":     {g.clientID},
		"redirect_uri":  {g.redirectURI},
		"response_type": {"code"},
		"scope":         {googleScope},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
		"state":         {state},
	}
	return googleAuthURL + "?" + params.Encode()
}

func (g *GoogleCalendarConnector) ExchangeCode(ctx context.Context, userID, code string) error {
	form := url.Values{
		"code":          {code},
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
		"redirect_uri":  {g.redirectURI},
		"grant_type":    {"authorization_code"},
	}

	resp, err := g.client.PostForm(googleTokenURL, form)
	if err != nil {
		return fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token exchange failed %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	expiresAt := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	_, err = g.db.Exec(ctx, `
		INSERT INTO oauth_tokens (source, access_token, refresh_token, expires_at, updated_at, user_id)
		VALUES ('google_calendar', $1, $2, $3, NOW(), $4)
		ON CONFLICT (source, user_id) DO UPDATE SET
			access_token = $1, refresh_token = $2, expires_at = $3, updated_at = NOW()
	`, result.AccessToken, result.RefreshToken, expiresAt, userID)
	if err != nil {
		return err
	}
	_, err = g.db.Exec(ctx, `
		INSERT INTO sync_state (source, enabled, updated_at, user_id)
		VALUES ('google_calendar', true, NOW(), $1)
		ON CONFLICT (source, user_id) DO UPDATE SET enabled = true, updated_at = NOW()
	`, userID)
	if err != nil {
		return err
	}

	g.logger.Info().Str("user_id", userID).Msg("google calendar authorized")
	return err
}

func (g *GoogleCalendarConnector) Sync(ctx context.Context, userID string) error {
	token, err := g.getAccessToken(ctx, userID)
	if err != nil {
		return err
	}

	now := time.Now()
	timeMin := now.AddDate(0, -1, 0).Format(time.RFC3339)
	timeMax := now.AddDate(0, 3, 0).Format(time.RFC3339)

	params := url.Values{
		"timeMin":      {timeMin},
		"timeMax":      {timeMax},
		"singleEvents": {"true"},
		"orderBy":      {"startTime"},
		"maxResults":   {"250"},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", googleCalURL+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("calendar API %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Items []struct {
			ID          string `json:"id"`
			Summary     string `json:"summary"`
			Description string `json:"description"`
			Location    string `json:"location"`
			Start       struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"start"`
			End struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"end"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode events: %w", err)
	}

	synced := 0
	for _, ev := range result.Items {
		if ev.Summary == "" {
			continue
		}

		allDay := ev.Start.DateTime == ""
		var startTime, endTime time.Time
		if allDay {
			startTime, _ = time.Parse("2006-01-02", ev.Start.Date)
			endTime, _ = time.Parse("2006-01-02", ev.End.Date)
		} else {
			startTime, _ = time.Parse(time.RFC3339, ev.Start.DateTime)
			endTime, _ = time.Parse(time.RFC3339, ev.End.DateTime)
		}

		_, err := g.db.Exec(ctx, `
			INSERT INTO calendar_events (user_id, external_id, title, description, start_time, end_time, all_day, location)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (user_id, external_id) DO UPDATE SET
				title = $3, description = $4, start_time = $5, end_time = $6, all_day = $7, location = $8
		`, userID, ev.ID, ev.Summary, ev.Description, startTime, endTime, allDay, ev.Location)
		if err != nil {
			g.logger.Warn().Err(err).Str("event", ev.Summary).Msg("upsert event failed")
			continue
		}
		synced++
	}

	_, err = g.db.Exec(ctx, `
		INSERT INTO sync_state (source, last_synced_at, updated_at, user_id)
		VALUES ('google_calendar', NOW(), NOW(), $1)
		ON CONFLICT (source, user_id) DO UPDATE SET last_synced_at = NOW(), updated_at = NOW()
	`, userID)

	g.logger.Info().Int("synced", synced).Int("total", len(result.Items)).Msg("calendar sync complete")
	return err
}

func (g *GoogleCalendarConnector) getAccessToken(ctx context.Context, userID string) (string, error) {
	var accessToken, refreshToken string
	var expiresAt time.Time
	err := g.db.QueryRow(ctx, `
		SELECT access_token, refresh_token, expires_at FROM oauth_tokens
		WHERE source = 'google_calendar' AND user_id = $1
	`, userID).Scan(&accessToken, &refreshToken, &expiresAt)
	if err != nil {
		return "", fmt.Errorf("no token — authorize at /api/v1/auth/google")
	}

	if time.Now().After(expiresAt) {
		return g.refreshToken(ctx, userID, refreshToken)
	}
	return accessToken, nil
}

func (g *GoogleCalendarConnector) refreshToken(ctx context.Context, userID, refreshTok string) (string, error) {
	form := url.Values{
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
		"refresh_token": {refreshTok},
		"grant_type":    {"refresh_token"},
	}

	resp, err := g.client.PostForm(googleTokenURL, form)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("refresh failed %d", resp.StatusCode)
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	expiresAt := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	g.db.Exec(ctx, `
		UPDATE oauth_tokens SET access_token=$1, expires_at=$2, updated_at=NOW()
		WHERE source='google_calendar' AND user_id=$3
	`, result.AccessToken, expiresAt, userID)

	return result.AccessToken, nil
}
