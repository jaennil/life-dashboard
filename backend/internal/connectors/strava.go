package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	stravaBaseURL   = "https://www.strava.com/api/v3"
	stravaTokenURL  = "https://www.strava.com/oauth/token"
	stravaAuthURL   = "https://www.strava.com/oauth/authorize"
	stravaPerPage   = 200
)

// ---- API types ----

type stravaTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	Athlete      struct {
		ID int64 `json:"id"`
	} `json:"athlete"`
}

type stravaActivity struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Description        string     `json:"description"`
	SportType          string     `json:"sport_type"`
	StartDate          time.Time  `json:"start_date"`
	MovingTime         int        `json:"moving_time"`
	ElapsedTime        int        `json:"elapsed_time"`
	Distance           float64    `json:"distance"`
	TotalElevationGain float64    `json:"total_elevation_gain"`
	AverageHeartrate   *float64   `json:"average_heartrate"`
	MaxHeartrate       *float64   `json:"max_heartrate"`
	AverageCadence     *float64   `json:"average_cadence"`
	AverageWatts       *float64   `json:"average_watts"`
	Calories           *float64   `json:"calories"`
	Map                struct {
		SummaryPolyline string `json:"summary_polyline"`
	} `json:"map"`
}

// ---- Connector ----

type StravaConnector struct {
	clientID     string
	clientSecret string
	redirectURI  string
	db           *pgxpool.Pool
	client       *http.Client
	logger       zerolog.Logger
}

func NewStrava(clientID, clientSecret, redirectURI string, db *pgxpool.Pool, logger zerolog.Logger) *StravaConnector {
	return &StravaConnector{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		db:           db,
		client:       &http.Client{Timeout: 30 * time.Second},
		logger:       logger.With().Str("connector", "strava").Logger(),
	}
}

func (s *StravaConnector) Name() string { return "strava" }

// AuthURL returns the Strava OAuth authorization URL
func (s *StravaConnector) AuthURL(state string) string {
	params := url.Values{
		"client_id":     {s.clientID},
		"redirect_uri":  {s.redirectURI},
		"response_type": {"code"},
		"scope":         {"activity:read_all"},
		"approval_prompt": {"auto"},
		"state":         {state},
	}
	return stravaAuthURL + "?" + params.Encode()
}

// ExchangeCode exchanges an OAuth code for tokens and stores them
func (s *StravaConnector) ExchangeCode(ctx context.Context, code string) error {
	resp, err := s.fetchToken(ctx, url.Values{
		"client_id":     {s.clientID},
		"client_secret": {s.clientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	})
	if err != nil {
		return fmt.Errorf("exchange code: %w", err)
	}

	if err := s.saveToken(ctx, resp); err != nil {
		return fmt.Errorf("save token: %w", err)
	}

	s.logger.Info().Int64("athlete_id", resp.Athlete.ID).Msg("strava authorized")
	return nil
}

func (s *StravaConnector) Sync(ctx context.Context) error {
	token, err := s.loadToken(ctx)
	if err != nil {
		return fmt.Errorf("load token: %w", err)
	}

	// Refresh if expired
	if time.Now().After(token.ExpiresAt) {
		s.logger.Info().Msg("access token expired, refreshing")
		token, err = s.refreshToken(ctx, token.RefreshToken)
		if err != nil {
			return fmt.Errorf("refresh token: %w", err)
		}
	}

	lastSync, err := s.getLastSync(ctx)
	if err != nil {
		return fmt.Errorf("get last sync: %w", err)
	}

	total, err := s.fetchActivities(ctx, token.AccessToken, lastSync)
	if err != nil {
		return err
	}

	if err := s.updateLastSync(ctx); err != nil {
		return err
	}

	s.logger.Info().Int("total", total).Msg("sync complete")
	return nil
}

func (s *StravaConnector) fetchActivities(ctx context.Context, accessToken string, since time.Time) (int, error) {
	page := 1
	total := 0
	afterUnix := since.Unix()
	if since.IsZero() {
		afterUnix = 0
	}

	for {
		reqURL := fmt.Sprintf("%s/athlete/activities?after=%d&per_page=%d&page=%d",
			stravaBaseURL, afterUnix, stravaPerPage, page)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return total, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := s.client.Do(req)
		if err != nil {
			return total, err
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			return total, fmt.Errorf("strava rate limit exceeded")
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return total, fmt.Errorf("strava api returned status %d", resp.StatusCode)
		}

		var activities []stravaActivity
		if err := json.NewDecoder(resp.Body).Decode(&activities); err != nil {
			resp.Body.Close()
			return total, fmt.Errorf("decode activities: %w", err)
		}
		resp.Body.Close()

		if len(activities) == 0 {
			break
		}

		for i := range activities {
			if err := s.upsertActivity(ctx, &activities[i]); err != nil {
				return total, fmt.Errorf("upsert activity %d: %w", activities[i].ID, err)
			}
		}

		total += len(activities)
		s.logger.Debug().Int("page", page).Int("count", len(activities)).Int("total", total).Msg("page synced")

		if len(activities) < stravaPerPage {
			break
		}
		page++
	}

	return total, nil
}

func (s *StravaConnector) upsertActivity(ctx context.Context, a *stravaActivity) error {
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}

	externalID := fmt.Sprintf("%d", a.ID)

	_, err = s.db.Exec(ctx, `
		INSERT INTO raw_events (source, event_type, external_id, payload)
		VALUES ('strava', 'activity', $1, $2)
	`, externalID, raw)
	if err != nil {
		return fmt.Errorf("insert raw event: %w", err)
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO activities
			(source, external_id, type, started_at, duration_seconds,
			 distance_meters, elevation_gain_meters, avg_heart_rate, max_heart_rate,
			 avg_cadence, avg_power_watts, calories, name, description, raw_payload)
		VALUES
			('strava', $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (external_id) DO UPDATE SET
			type                  = EXCLUDED.type,
			started_at            = EXCLUDED.started_at,
			duration_seconds      = EXCLUDED.duration_seconds,
			distance_meters       = EXCLUDED.distance_meters,
			elevation_gain_meters = EXCLUDED.elevation_gain_meters,
			avg_heart_rate        = EXCLUDED.avg_heart_rate,
			max_heart_rate        = EXCLUDED.max_heart_rate,
			avg_cadence           = EXCLUDED.avg_cadence,
			avg_power_watts       = EXCLUDED.avg_power_watts,
			calories              = EXCLUDED.calories,
			name                  = EXCLUDED.name,
			description           = EXCLUDED.description,
			raw_payload           = EXCLUDED.raw_payload
	`,
		externalID, a.SportType, a.StartDate, a.MovingTime,
		a.Distance, a.TotalElevationGain,
		a.AverageHeartrate, a.MaxHeartrate,
		a.AverageCadence, a.AverageWatts,
		a.Calories, a.Name, a.Description, raw,
	)
	if err != nil {
		return fmt.Errorf("upsert activity: %w", err)
	}

	s.logger.Debug().Int64("id", a.ID).Str("type", a.SportType).Str("name", a.Name).Msg("activity upserted")
	return nil
}

// ---- Token helpers ----

type oauthToken struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

func (s *StravaConnector) fetchToken(ctx context.Context, params url.Values) (*stravaTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stravaTokenURL,
		strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed with status %d", resp.StatusCode)
	}

	var result stravaTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *StravaConnector) refreshToken(ctx context.Context, refreshToken string) (*oauthToken, error) {
	resp, err := s.fetchToken(ctx, url.Values{
		"client_id":     {s.clientID},
		"client_secret": {s.clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
	if err != nil {
		return nil, err
	}

	if err := s.saveToken(ctx, resp); err != nil {
		return nil, err
	}

	return &oauthToken{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    time.Unix(resp.ExpiresAt, 0),
	}, nil
}

func (s *StravaConnector) saveToken(ctx context.Context, t *stravaTokenResponse) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO oauth_tokens (source, access_token, refresh_token, expires_at, athlete_id, updated_at)
		VALUES ('strava', $1, $2, $3, $4, NOW())
		ON CONFLICT (source) DO UPDATE SET
			access_token  = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			expires_at    = EXCLUDED.expires_at,
			athlete_id    = EXCLUDED.athlete_id,
			updated_at    = NOW()
	`, t.AccessToken, t.RefreshToken, time.Unix(t.ExpiresAt, 0), t.Athlete.ID)
	return err
}

func (s *StravaConnector) loadToken(ctx context.Context) (*oauthToken, error) {
	var t oauthToken
	err := s.db.QueryRow(ctx, `
		SELECT access_token, refresh_token, expires_at FROM oauth_tokens WHERE source = 'strava'
	`).Scan(&t.AccessToken, &t.RefreshToken, &t.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("no strava token found — authorize first via GET /api/v1/auth/strava")
	}
	return &t, nil
}

func (s *StravaConnector) getLastSync(ctx context.Context) (time.Time, error) {
	var t time.Time
	err := s.db.QueryRow(ctx, `
		SELECT last_synced_at FROM sync_state WHERE source = 'strava'
	`).Scan(&t)
	if err != nil {
		return time.Time{}, nil
	}
	return t, nil
}

func (s *StravaConnector) updateLastSync(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO sync_state (source, last_synced_at, updated_at)
		VALUES ('strava', NOW(), NOW())
		ON CONFLICT (source) DO UPDATE SET
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at     = EXCLUDED.updated_at
	`)
	return err
}
