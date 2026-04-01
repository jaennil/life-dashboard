package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
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
	ID                    int64      `json:"id"`
	Name                  string     `json:"name"`
	Description           string     `json:"description"`
	Type                  string     `json:"type"`
	SportType             string     `json:"sport_type"`
	StartDate             time.Time  `json:"start_date"`
	MovingTime            int        `json:"moving_time"`
	ElapsedTime           int        `json:"elapsed_time"`
	Distance              float64    `json:"distance"`
	TotalElevationGain    float64    `json:"total_elevation_gain"`
	AverageHeartrate      *float64   `json:"average_heartrate"`
	MaxHeartrate          *float64   `json:"max_heartrate"`
	AverageCadence        *float64   `json:"average_cadence"`
	AverageWatts          *float64   `json:"average_watts"`
	WeightedAverageWatts  *int       `json:"weighted_average_watts"`
	MaxWatts              *int       `json:"max_watts"`
	Calories              *float64   `json:"calories"`
	AverageSpeed          *float64   `json:"average_speed"`
	MaxSpeed              *float64   `json:"max_speed"`
	AverageTemp           *float64   `json:"average_temp"`
	Kilojoules            *float64   `json:"kilojoules"`
	ElevHigh              *float64   `json:"elev_high"`
	ElevLow               *float64   `json:"elev_low"`
	SufferScore           *int       `json:"suffer_score"`
	PRCount               *int       `json:"pr_count"`
	WorkoutType           *int       `json:"workout_type"`
	DeviceName            string     `json:"device_name"`
	GearID                string     `json:"gear_id"`
	StartLatlng           []float64  `json:"start_latlng"`
	Map                   struct {
		SummaryPolyline string `json:"summary_polyline"`
	} `json:"map"`
	Gear *struct {
		Name string `json:"name"`
	} `json:"gear"`
	SplitsMetric []stravaSplit `json:"splits_metric"`
	Laps         []stravaLap   `json:"laps"`
}

type stravaSplit struct {
	Split               int     `json:"split"`
	Distance            float64 `json:"distance"`
	ElapsedTime         int     `json:"elapsed_time"`
	MovingTime          int     `json:"moving_time"`
	ElevationDifference float64 `json:"elevation_difference"`
	AverageSpeed        float64 `json:"average_speed"`
	PaceZone            int     `json:"pace_zone"`
}

type stravaLap struct {
	LapIndex           int       `json:"lap_index"`
	Name               string    `json:"name"`
	Distance           float64   `json:"distance"`
	ElapsedTime        int       `json:"elapsed_time"`
	MovingTime         int       `json:"moving_time"`
	StartDate          time.Time `json:"start_date"`
	TotalElevationGain float64   `json:"total_elevation_gain"`
	AverageSpeed       float64   `json:"average_speed"`
	MaxSpeed           float64   `json:"max_speed"`
	AverageCadence     *float64  `json:"average_cadence"`
	AverageWatts       *float64  `json:"average_watts"`
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
func (s *StravaConnector) ExchangeCode(ctx context.Context, userID string, code string) error {
	resp, err := s.fetchToken(ctx, url.Values{
		"client_id":     {s.clientID},
		"client_secret": {s.clientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	})
	if err != nil {
		return fmt.Errorf("exchange code: %w", err)
	}

	if err := s.saveToken(ctx, userID, resp); err != nil {
		return fmt.Errorf("save token: %w", err)
	}

	s.logger.Info().Int64("athlete_id", resp.Athlete.ID).Msg("strava authorized")
	return nil
}

func (s *StravaConnector) Sync(ctx context.Context, userID string) error {
	token, err := s.loadToken(ctx, userID)
	if err != nil {
		return fmt.Errorf("load token: %w", err)
	}

	// Refresh if expired
	if time.Now().After(token.ExpiresAt) {
		s.logger.Info().Msg("access token expired, refreshing")
		token, err = s.refreshToken(ctx, userID, token.RefreshToken)
		if err != nil {
			return fmt.Errorf("refresh token: %w", err)
		}
	}

	lastSync, err := s.getLastSync(ctx, userID)
	if err != nil {
		return fmt.Errorf("get last sync: %w", err)
	}

	total, err := s.fetchActivities(ctx, userID, token.AccessToken, lastSync)
	if err != nil {
		return err
	}

	if err := s.updateLastSync(ctx, userID); err != nil {
		return err
	}

	s.logger.Info().Int("total", total).Msg("sync complete")
	return nil
}

func (s *StravaConnector) fetchActivities(ctx context.Context, userID string, accessToken string, since time.Time) (int, error) {
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
			if err := s.upsertActivity(ctx, userID, &activities[i]); err != nil {
				return total, fmt.Errorf("upsert activity %d: %w", activities[i].ID, err)
			}
			if activities[i].SportType == "WeightTraining" {
				if err := s.syncWorkoutDetails(ctx, userID, accessToken, &activities[i]); err != nil {
					s.logger.Warn().Err(err).Int64("id", activities[i].ID).Msg("failed to sync workout details")
				}
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

func (s *StravaConnector) upsertActivity(ctx context.Context, userID string, a *stravaActivity) error {
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}

	externalID := fmt.Sprintf("%d", a.ID)

	_, err = s.db.Exec(ctx, `
		INSERT INTO raw_events (source, event_type, external_id, payload, user_id)
		VALUES ('strava', 'activity', $1, $2, $3)
	`, externalID, raw, userID)
	if err != nil {
		return fmt.Errorf("insert raw event: %w", err)
	}

	var gearName *string
	if a.Gear != nil && a.Gear.Name != "" {
		gearName = &a.Gear.Name
	}
	var startLat, startLng *float64
	if len(a.StartLatlng) == 2 {
		startLat = &a.StartLatlng[0]
		startLng = &a.StartLatlng[1]
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO activities
			(source, external_id, type, sport_type, started_at, duration_seconds, elapsed_time,
			 distance_meters, elevation_gain_meters, avg_heart_rate, max_heart_rate,
			 avg_cadence, avg_power_watts, weighted_average_watts, max_watts,
			 calories, average_speed, max_speed, average_temp, kilojoules,
			 elev_high, elev_low, suffer_score, pr_count, workout_type,
			 device_name, gear_name, start_lat, start_lng, map_summary_polyline,
			 name, description, raw_payload, user_id)
		VALUES
			('strava', $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			 $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33)
		ON CONFLICT (user_id, external_id) DO UPDATE SET
			type = EXCLUDED.type, sport_type = EXCLUDED.sport_type,
			started_at = EXCLUDED.started_at, duration_seconds = EXCLUDED.duration_seconds,
			elapsed_time = EXCLUDED.elapsed_time, distance_meters = EXCLUDED.distance_meters,
			elevation_gain_meters = EXCLUDED.elevation_gain_meters,
			avg_heart_rate = EXCLUDED.avg_heart_rate, max_heart_rate = EXCLUDED.max_heart_rate,
			avg_cadence = EXCLUDED.avg_cadence, avg_power_watts = EXCLUDED.avg_power_watts,
			weighted_average_watts = EXCLUDED.weighted_average_watts, max_watts = EXCLUDED.max_watts,
			calories = EXCLUDED.calories, average_speed = EXCLUDED.average_speed, max_speed = EXCLUDED.max_speed,
			average_temp = EXCLUDED.average_temp, kilojoules = EXCLUDED.kilojoules,
			elev_high = EXCLUDED.elev_high, elev_low = EXCLUDED.elev_low,
			suffer_score = EXCLUDED.suffer_score, pr_count = EXCLUDED.pr_count,
			workout_type = EXCLUDED.workout_type, device_name = EXCLUDED.device_name,
			gear_name = EXCLUDED.gear_name, start_lat = EXCLUDED.start_lat, start_lng = EXCLUDED.start_lng,
			map_summary_polyline = EXCLUDED.map_summary_polyline,
			name = EXCLUDED.name, description = EXCLUDED.description, raw_payload = EXCLUDED.raw_payload
	`,
		externalID, a.Type, a.SportType, a.StartDate, a.MovingTime, a.ElapsedTime,
		a.Distance, a.TotalElevationGain,
		a.AverageHeartrate, a.MaxHeartrate,
		a.AverageCadence, a.AverageWatts, a.WeightedAverageWatts, a.MaxWatts,
		a.Calories, a.AverageSpeed, a.MaxSpeed, a.AverageTemp, a.Kilojoules,
		a.ElevHigh, a.ElevLow, a.SufferScore, a.PRCount, a.WorkoutType,
		a.DeviceName, gearName, startLat, startLng, a.Map.SummaryPolyline,
		a.Name, a.Description, raw, userID,
	)
	if err != nil {
		return fmt.Errorf("upsert activity: %w", err)
	}

	s.logger.Debug().Int64("id", a.ID).Str("type", a.SportType).Str("name", a.Name).Msg("activity upserted")

	// Get activity DB id for splits/laps
	var activityDBID string
	s.db.QueryRow(ctx, `SELECT id FROM activities WHERE external_id = $1 AND user_id = $2`, externalID, userID).Scan(&activityDBID)

	// Save splits
	if len(a.SplitsMetric) > 0 && activityDBID != "" {
		s.db.Exec(ctx, `DELETE FROM activity_splits WHERE activity_id = $1`, activityDBID)
		for _, sp := range a.SplitsMetric {
			s.db.Exec(ctx, `
				INSERT INTO activity_splits (activity_id, split, distance, elapsed_time, moving_time, elevation_difference, average_speed, pace_zone)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`, activityDBID, sp.Split, sp.Distance, sp.ElapsedTime, sp.MovingTime, sp.ElevationDifference, sp.AverageSpeed, sp.PaceZone)
		}
	}

	// Save laps
	if len(a.Laps) > 0 && activityDBID != "" {
		s.db.Exec(ctx, `DELETE FROM activity_laps WHERE activity_id = $1`, activityDBID)
		for _, lp := range a.Laps {
			s.db.Exec(ctx, `
				INSERT INTO activity_laps (activity_id, lap_index, name, distance, elapsed_time, moving_time, start_date, total_elevation_gain, average_speed, max_speed, average_cadence, average_watts)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			`, activityDBID, lp.LapIndex, lp.Name, lp.Distance, lp.ElapsedTime, lp.MovingTime, lp.StartDate, lp.TotalElevationGain, lp.AverageSpeed, lp.MaxSpeed, lp.AverageCadence, lp.AverageWatts)
		}
	}

	return nil
}

// ---- Workout parsing (Hevy via Strava description) ----

type parsedSet struct {
	Index    int
	SetType  string
	WeightKg float64
	Reps     int
}

type parsedExercise struct {
	Name     string
	Category string
	Sets     []parsedSet
}

var setLineRe = regexp.MustCompile(`^Set\s+(\d+):\s+([\d.]+)\s+kg\s+x\s+(\d+)(?:\s+\[([^\]]+)\])?`)

func parseHevyDescription(desc string) []parsedExercise {
	var exercises []parsedExercise
	var current *parsedExercise

	for _, rawLine := range strings.Split(desc, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "Logged with") || strings.HasPrefix(line, `"`) {
			continue
		}

		if m := setLineRe.FindStringSubmatch(line); m != nil {
			if current == nil {
				continue
			}
			idx, _ := strconv.Atoi(m[1])
			weight, _ := strconv.ParseFloat(m[2], 64)
			reps, _ := strconv.Atoi(m[3])
			setType := "normal"
			if m[4] != "" {
				setType = strings.ToLower(m[4])
			}
			current.Sets = append(current.Sets, parsedSet{
				Index:    idx,
				SetType:  setType,
				WeightKg: weight,
				Reps:     reps,
			})
			continue
		}

		// It's an exercise name, possibly with category in parentheses
		exercises = append(exercises, parsedExercise{})
		current = &exercises[len(exercises)-1]
		if open := strings.LastIndex(line, "("); open != -1 && strings.HasSuffix(line, ")") {
			current.Name = strings.TrimSpace(line[:open])
			current.Category = line[open+1 : len(line)-1]
		} else {
			current.Name = line
		}
	}

	return exercises
}

func (s *StravaConnector) syncWorkoutDetails(ctx context.Context, userID string, accessToken string, a *stravaActivity) error {
	externalID := fmt.Sprintf("%d", a.ID)

	// Skip if workout already stored for this activity
	var exists bool
	s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workouts WHERE external_id = $1 AND user_id = $2)`, externalID, userID).Scan(&exists)
	if exists {
		return nil
	}

	// Fetch full activity detail from Strava
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/activities/%d", stravaBaseURL, a.ID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("strava rate limit exceeded")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("strava detail api returned status %d", resp.StatusCode)
	}

	var detail stravaActivity
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return fmt.Errorf("decode activity detail: %w", err)
	}

	if detail.Description == "" {
		return nil
	}

	exercises := parseHevyDescription(detail.Description)
	if len(exercises) == 0 {
		return nil
	}

	// Store workout
	endedAt := a.StartDate.Add(time.Duration(a.ElapsedTime) * time.Second)
	var workoutID string
	err = s.db.QueryRow(ctx, `
		INSERT INTO workouts (source, external_id, started_at, ended_at, title, notes, user_id)
		VALUES ('strava', $1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, external_id) DO UPDATE SET
			started_at = EXCLUDED.started_at,
			ended_at   = EXCLUDED.ended_at,
			title      = EXCLUDED.title
		RETURNING id
	`, externalID, a.StartDate, endedAt, a.Name, detail.Description, userID).Scan(&workoutID)
	if err != nil {
		return fmt.Errorf("upsert workout: %w", err)
	}

	// Delete existing sets and re-insert (clean slate on re-sync)
	if _, err := s.db.Exec(ctx, `DELETE FROM workout_sets WHERE workout_id = $1`, workoutID); err != nil {
		return fmt.Errorf("delete workout sets: %w", err)
	}

	for _, ex := range exercises {
		for _, set := range ex.Sets {
			_, err := s.db.Exec(ctx, `
				INSERT INTO workout_sets
					(workout_id, exercise_name, exercise_category, set_index, set_type, weight_kg, reps)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
			`, workoutID, ex.Name, ex.Category, set.Index, set.SetType, set.WeightKg, set.Reps)
			if err != nil {
				return fmt.Errorf("insert workout set: %w", err)
			}
		}
	}

	s.logger.Info().Str("name", a.Name).Int("exercises", len(exercises)).Msg("workout details synced")
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

func (s *StravaConnector) refreshToken(ctx context.Context, userID string, refreshToken string) (*oauthToken, error) {
	resp, err := s.fetchToken(ctx, url.Values{
		"client_id":     {s.clientID},
		"client_secret": {s.clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
	if err != nil {
		return nil, err
	}

	if err := s.saveToken(ctx, userID, resp); err != nil {
		return nil, err
	}

	return &oauthToken{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    time.Unix(resp.ExpiresAt, 0),
	}, nil
}

func (s *StravaConnector) saveToken(ctx context.Context, userID string, t *stravaTokenResponse) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO oauth_tokens (source, access_token, refresh_token, expires_at, athlete_id, updated_at, user_id)
		VALUES ('strava', $1, $2, $3, $4, NOW(), $5)
		ON CONFLICT (source, user_id) DO UPDATE SET
			access_token  = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			expires_at    = EXCLUDED.expires_at,
			athlete_id    = EXCLUDED.athlete_id,
			updated_at    = NOW()
	`, t.AccessToken, t.RefreshToken, time.Unix(t.ExpiresAt, 0), t.Athlete.ID, userID)
	return err
}

func (s *StravaConnector) loadToken(ctx context.Context, userID string) (*oauthToken, error) {
	var t oauthToken
	err := s.db.QueryRow(ctx, `
		SELECT access_token, refresh_token, expires_at FROM oauth_tokens WHERE source = 'strava' AND user_id = $1
	`, userID).Scan(&t.AccessToken, &t.RefreshToken, &t.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("no strava token found — authorize first via GET /api/v1/auth/strava")
	}
	return &t, nil
}

func (s *StravaConnector) getLastSync(ctx context.Context, userID string) (time.Time, error) {
	var t time.Time
	err := s.db.QueryRow(ctx, `
		SELECT last_synced_at FROM sync_state WHERE source = 'strava' AND user_id = $1
	`, userID).Scan(&t)
	if err != nil {
		return time.Time{}, nil
	}
	return t, nil
}

func (s *StravaConnector) updateLastSync(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO sync_state (source, last_synced_at, updated_at, user_id)
		VALUES ('strava', NOW(), NOW(), $1)
		ON CONFLICT (source, user_id) DO UPDATE SET
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at     = EXCLUDED.updated_at
	`, userID)
	return err
}
