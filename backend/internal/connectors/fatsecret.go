package connectors

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	fsRequestTokenURL          = "https://authentication.fatsecret.com/oauth/request_token"
	fsAuthorizeURL             = "https://authentication.fatsecret.com/oauth/authorize"
	fsAccessTokenURL           = "https://authentication.fatsecret.com/oauth/access_token"
	fsAPIBase                  = "https://platform.fatsecret.com/rest/server.api"
	nutritionSyncDays          = 90
	fatSecretScheduledSyncDays = 7
)

type requestTokenState struct {
	secret string
	userID string
}

type FatSecretConnector struct {
	consumerKey    string
	consumerSecret string
	redirectURI    string
	db             *pgxpool.Pool
	client         *http.Client
	logger         zerolog.Logger
	// temporary storage for request token secrets during OAuth flow
	requestSecrets map[string]requestTokenState
}

func NewFatSecret(consumerKey, consumerSecret, redirectURI string, db *pgxpool.Pool, logger zerolog.Logger) *FatSecretConnector {
	return &FatSecretConnector{
		consumerKey:    consumerKey,
		consumerSecret: consumerSecret,
		redirectURI:    redirectURI,
		db:             db,
		client:         &http.Client{Timeout: 30 * time.Second},
		logger:         logger.With().Str("connector", "fatsecret").Logger(),
		requestSecrets: make(map[string]requestTokenState),
	}
}

func (c *FatSecretConnector) Name() string { return "fatsecret" }

func oauth1Nonce() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// oauth1Sign builds HMAC-SHA1 signature for OAuth 1.0
func (c *FatSecretConnector) oauth1Sign(method, baseURL string, params url.Values, tokenSecret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(params))
	for _, k := range keys {
		pairs = append(pairs, url.QueryEscape(k)+"="+url.QueryEscape(params.Get(k)))
	}
	paramStr := strings.Join(pairs, "&")

	baseStr := strings.ToUpper(method) + "&" + url.QueryEscape(baseURL) + "&" + url.QueryEscape(paramStr)

	sigKey := url.QueryEscape(c.consumerSecret) + "&" + url.QueryEscape(tokenSecret)
	mac := hmac.New(sha1.New, []byte(sigKey))
	mac.Write([]byte(baseStr))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// oauth1BaseParams builds the common OAuth 1.0 parameters
func (c *FatSecretConnector) oauth1BaseParams(token string) url.Values {
	params := url.Values{}
	params.Set("oauth_consumer_key", c.consumerKey)
	params.Set("oauth_signature_method", "HMAC-SHA1")
	params.Set("oauth_timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	params.Set("oauth_nonce", oauth1Nonce())
	params.Set("oauth_version", "1.0")
	if token != "" {
		params.Set("oauth_token", token)
	}
	return params
}

// GetRequestToken fetches an OAuth 1.0 request token, returns (token, secret)
func (c *FatSecretConnector) GetRequestToken(ctx context.Context) (string, string, error) {
	params := c.oauth1BaseParams("")
	params.Set("oauth_callback", c.redirectURI)

	sig := c.oauth1Sign("POST", fsRequestTokenURL, params, "")
	params.Set("oauth_signature", sig)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, fsRequestTokenURL, strings.NewReader(params.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("request token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	c.logger.Debug().Int("status", resp.StatusCode).Str("body", string(body)).Msg("request token response")

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("request token status %d: %s", resp.StatusCode, string(body))
	}

	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return "", "", fmt.Errorf("parse request token response: %w", err)
	}

	token := vals.Get("oauth_token")
	secret := vals.Get("oauth_token_secret")
	if token == "" {
		return "", "", fmt.Errorf("empty request token in response: %s", string(body))
	}

	c.logger.Info().Str("token", token).Msg("got request token")
	return token, secret, nil
}

// AuthURL fetches a request token and returns the authorization URL + request token
func (c *FatSecretConnector) AuthURL(ctx context.Context, userID string) (string, error) {
	token, secret, err := c.GetRequestToken(ctx)
	if err != nil {
		return "", err
	}
	c.requestSecrets[token] = requestTokenState{secret: secret, userID: userID}
	authURL := fsAuthorizeURL + "?oauth_token=" + url.QueryEscape(token)
	return authURL, nil
}

// ExchangeToken exchanges request token + verifier for an access token
func (c *FatSecretConnector) ExchangeToken(ctx context.Context, requestToken, verifier string) error {
	state := c.requestSecrets[requestToken]
	delete(c.requestSecrets, requestToken)

	params := c.oauth1BaseParams(requestToken)
	params.Set("oauth_verifier", verifier)

	sig := c.oauth1Sign("GET", fsAccessTokenURL, params, state.secret)
	params.Set("oauth_signature", sig)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fsAccessTokenURL+"?"+params.Encode(), nil)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("access token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	c.logger.Debug().Int("status", resp.StatusCode).Str("body", string(body)).Msg("access token response")

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("access token status %d: %s", resp.StatusCode, string(body))
	}

	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return fmt.Errorf("parse access token response: %w", err)
	}

	accessToken := vals.Get("oauth_token")
	accessSecret := vals.Get("oauth_token_secret")
	if accessToken == "" {
		return fmt.Errorf("empty access token in response: %s", string(body))
	}

	// Store: access_token = oauth_token, refresh_token = oauth_token_secret
	// OAuth 1.0 tokens don't expire — use far future date
	_, err = c.db.Exec(ctx, `
		INSERT INTO oauth_tokens (source, user_id, access_token, refresh_token, expires_at, updated_at)
		VALUES ('fatsecret', $1, $2, $3, $4, NOW())
		ON CONFLICT (source, user_id) DO UPDATE SET
			access_token = $2, refresh_token = $3, expires_at = $4, updated_at = NOW()
	`, state.userID, accessToken, accessSecret, time.Now().AddDate(100, 0, 0))
	if err != nil {
		return fmt.Errorf("save tokens: %w", err)
	}
	if _, err := c.db.Exec(ctx, `
		INSERT INTO sync_state (source, enabled, updated_at, user_id)
		VALUES ('fatsecret', true, NOW(), $1)
		ON CONFLICT (source, user_id) DO UPDATE SET enabled = true, updated_at = NOW()
	`, state.userID); err != nil {
		return fmt.Errorf("mark fatsecret enabled: %w", err)
	}

	c.logger.Info().Str("user_id", state.userID).Msg("fatsecret oauth1 tokens saved")
	return nil
}

func (c *FatSecretConnector) getStoredTokens(ctx context.Context, userID string) (string, string, error) {
	var token, secret string
	err := c.db.QueryRow(ctx, `
		SELECT access_token, refresh_token FROM oauth_tokens WHERE source = 'fatsecret' AND user_id = $1
	`, userID).Scan(&token, &secret)
	if err != nil {
		return "", "", fmt.Errorf("no token in db — authorize first at /api/v1/auth/fatsecret")
	}
	return token, secret, nil
}

func (c *FatSecretConnector) Sync(ctx context.Context, userID string) error {
	trigger := GetSyncTrigger(ctx)
	syncDays := fatSecretSyncDays(trigger)

	c.logger.Info().
		Str("user_id", userID).
		Str("trigger", string(trigger)).
		Int("sync_days", syncDays).
		Msg("starting sync")

	token, secret, err := c.getStoredTokens(ctx, userID)
	if err != nil {
		return err
	}

	if err := c.syncProfile(ctx, userID, token, secret); err != nil {
		c.logger.Warn().Err(err).Msg("failed to sync profile")
	}

	today := time.Now().Truncate(24 * time.Hour)
	recentFailures := make([]string, 0, fatSecretCriticalSyncDays)
	recentSuccesses := 0
	stoppedOnRateLimit := false
	for i := 0; i < syncDays; i++ {
		date := today.AddDate(0, 0, -i)
		if err := c.syncDay(ctx, userID, token, secret, date); err != nil {
			c.logger.Warn().Err(err).Str("date", date.Format("2006-01-02")).Msg("failed to sync day")
			if i < fatSecretCriticalSyncDays {
				recentFailures = append(recentFailures, date.Format("2006-01-02"))
			}
			if isFatSecretRateLimitError(err) {
				stoppedOnRateLimit = true
				break
			}
			continue
		}
		if i < fatSecretCriticalSyncDays {
			recentSuccesses++
		}
	}

	if err := fatSecretRecentSyncError(recentFailures, recentSuccesses); err != nil {
		return err
	}
	if stoppedOnRateLimit {
		c.logger.Warn().
			Str("user_id", userID).
			Str("trigger", string(trigger)).
			Msg("fatsecret sync stopped early after rate limit; recent days already refreshed")
	}

	_, err = c.db.Exec(ctx, `
		INSERT INTO sync_state (source, last_synced_at, updated_at, user_id)
		VALUES ('fatsecret', NOW(), NOW(), $1)
		ON CONFLICT (source, user_id) DO UPDATE SET last_synced_at = NOW(), updated_at = NOW()
	`, userID)

	c.logger.Info().Msg("sync complete")
	return err
}

func fatSecretSyncDays(trigger SyncTrigger) int {
	if trigger == SyncTriggerScheduled {
		return fatSecretScheduledSyncDays
	}
	return nutritionSyncDays
}

// daysSinceEpoch returns FatSecret's date format: days since Jan 1, 1970
func daysSinceEpoch(t time.Time) int {
	epoch := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	return int(t.UTC().Sub(epoch).Hours() / 24)
}

type fsFoodEntry struct {
	FoodEntryID            string `json:"food_entry_id"`
	FoodEntryName          string `json:"food_entry_name"`
	MealID                 string `json:"meal_id"`
	Meal                   string `json:"meal"`
	NumberOfUnits          string `json:"number_of_units"`
	MeasurementDescription string `json:"measurement_description"`
	Calories               string `json:"calories"`
	Carbohydrate           string `json:"carbohydrate"`
	Protein                string `json:"protein"`
	Fat                    string `json:"fat"`
	SaturatedFat           string `json:"saturated_fat"`
	PolyunsaturatedFat     string `json:"polyunsaturated_fat"`
	MonounsaturatedFat     string `json:"monounsaturated_fat"`
	Cholesterol            string `json:"cholesterol"`
	Sodium                 string `json:"sodium"`
	Potassium              string `json:"potassium"`
	Fiber                  string `json:"fiber"`
	Sugar                  string `json:"sugar"`
	VitaminA               string `json:"vitamin_a"`
	VitaminC               string `json:"vitamin_c"`
	Calcium                string `json:"calcium"`
	Iron                   string `json:"iron"`
}

type fsProfile struct {
	LastWeightKg       string `json:"last_weight_kg"`
	LastWeightDateInt  string `json:"last_weight_date_int"`
	LastWeightComment  string `json:"last_weight_comment"`
	GoalWeightKg       string `json:"goal_weight_kg"`
	HeightCm           string `json:"height_cm"`
	WeightMeasure      string `json:"weight_measure"`
	HeightMeasure      string `json:"height_measure"`
	PreferredFoods     string `json:"preferred_foods"`
	ExerciseLevel      string `json:"exercise_level"`
	FoodDiaryPrivacy   string `json:"food_diary_privacy"`
	WeightDiaryPrivacy string `json:"weight_diary_privacy"`
	CalendarSharingURL string `json:"calendar_sharing_url"`
}

var fsMealNames = map[string]string{
	"0": "breakfast",
	"1": "lunch",
	"2": "dinner",
	"3": "snacks",
}

const fatSecretCriticalSyncDays = 3

func normalizeFatSecretMealType(meal, mealID string) string {
	switch strings.TrimSpace(strings.ToLower(meal)) {
	case "breakfast":
		return "breakfast"
	case "lunch":
		return "lunch"
	case "dinner":
		return "dinner"
	case "snack", "snacks", "morning snack", "afternoon snack", "evening snack":
		return "snacks"
	case "other":
		return "other"
	}

	if normalized := fsMealNames[strings.TrimSpace(mealID)]; normalized != "" {
		return normalized
	}

	return "other"
}

func isFatSecretRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "too many actions")
}

func fatSecretRecentSyncError(recentFailures []string, recentSuccesses int) error {
	if len(recentFailures) > 0 {
		return fmt.Errorf("fatsecret recent days sync failed for %s", strings.Join(recentFailures, ", "))
	}
	if recentSuccesses == 0 {
		return fmt.Errorf("fatsecret recent days sync did not refresh")
	}
	return nil
}

func (c *FatSecretConnector) syncProfile(ctx context.Context, userID, token, secret string) error {
	params := c.oauth1BaseParams(token)
	params.Set("method", "profile.get")
	params.Set("format", "json")

	sig := c.oauth1Sign("GET", fsAPIBase, params, secret)
	params.Set("oauth_signature", sig)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fsAPIBase+"?"+params.Encode(), nil)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("profile request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("profile status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Profile json.RawMessage `json:"profile"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("decode profile: %w", err)
	}
	if result.Error != nil {
		return fmt.Errorf("profile api error: %s", result.Error.Message)
	}
	if len(result.Profile) == 0 || string(result.Profile) == "null" {
		return nil
	}

	var profile fsProfile
	if err := json.Unmarshal(result.Profile, &profile); err != nil {
		return fmt.Errorf("decode profile body: %w", err)
	}

	return c.storeProfile(ctx, userID, profile, result.Profile)
}

func (c *FatSecretConnector) syncDay(ctx context.Context, userID, token, secret string, date time.Time) error {
	dateDay := daysSinceEpoch(date)

	params := c.oauth1BaseParams(token)
	params.Set("method", "food_entries.get")
	params.Set("date", fmt.Sprintf("%d", dateDay))
	params.Set("format", "json")

	sig := c.oauth1Sign("GET", fsAPIBase, params, secret)
	params.Set("oauth_signature", sig)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fsAPIBase+"?"+params.Encode(), nil)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("api status %d", resp.StatusCode)
	}

	var result struct {
		FoodEntries struct {
			FoodEntry json.RawMessage `json:"food_entry"`
		} `json:"food_entries"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if result.Error != nil {
		if strings.Contains(result.Error.Message, "no food entries") {
			return nil
		}
		return fmt.Errorf("api error: %s", result.Error.Message)
	}

	// food_entry can be single object or array
	var entries []fsFoodEntry
	if len(result.FoodEntries.FoodEntry) > 0 {
		if result.FoodEntries.FoodEntry[0] == '[' {
			json.Unmarshal(result.FoodEntries.FoodEntry, &entries)
		} else {
			var single fsFoodEntry
			if json.Unmarshal(result.FoodEntries.FoodEntry, &single) == nil {
				entries = []fsFoodEntry{single}
			}
		}
	}

	if len(entries) == 0 {
		return nil
	}

	return c.storeEntries(ctx, userID, date, entries)
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

func parseOptionalFloat(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &value
}

func dateFromDaysSinceEpochPtr(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	days, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	date := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, days)
	return &date
}

func (c *FatSecretConnector) storeProfile(ctx context.Context, userID string, profile fsProfile, raw json.RawMessage) error {
	_, err := c.db.Exec(ctx, `
		INSERT INTO nutrition_targets (
			user_id, source, current_weight_kg, current_weight_date, current_weight_comment,
			target_weight_kg, height_cm, weight_measure, height_measure, raw_payload,
			synced_at, updated_at
		)
		VALUES ($1, 'fatsecret', $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
		ON CONFLICT (user_id, source) DO UPDATE SET
			current_weight_kg = EXCLUDED.current_weight_kg,
			current_weight_date = EXCLUDED.current_weight_date,
			current_weight_comment = EXCLUDED.current_weight_comment,
			target_weight_kg = EXCLUDED.target_weight_kg,
			height_cm = EXCLUDED.height_cm,
			weight_measure = EXCLUDED.weight_measure,
			height_measure = EXCLUDED.height_measure,
			raw_payload = EXCLUDED.raw_payload,
			synced_at = NOW(),
			updated_at = NOW()
	`, userID,
		parseOptionalFloat(profile.LastWeightKg),
		dateFromDaysSinceEpochPtr(profile.LastWeightDateInt),
		nullIfEmpty(profile.LastWeightComment),
		parseOptionalFloat(profile.GoalWeightKg),
		parseOptionalFloat(profile.HeightCm),
		nullIfEmpty(profile.WeightMeasure),
		nullIfEmpty(profile.HeightMeasure),
		raw,
	)
	if err != nil {
		return fmt.Errorf("upsert profile targets: %w", err)
	}

	c.logger.Info().
		Str("user_id", userID).
		Str("current_weight_kg", profile.LastWeightKg).
		Str("goal_weight_kg", profile.GoalWeightKg).
		Msg("synced profile")
	return nil
}

func (c *FatSecretConnector) storeEntries(ctx context.Context, userID string, date time.Time, entries []fsFoodEntry) error {
	var totalCal, totalProtein, totalCarbs, totalFat, totalFiber float64
	for _, e := range entries {
		totalCal += parseFloat(e.Calories)
		totalProtein += parseFloat(e.Protein)
		totalCarbs += parseFloat(e.Carbohydrate)
		totalFat += parseFloat(e.Fat)
		totalFiber += parseFloat(e.Fiber)
	}

	var dailyID string
	err := c.db.QueryRow(ctx, `
		INSERT INTO nutrition_daily (user_id, date, calories_total, protein_g, carbs_g, fat_g, fiber_g, source)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'fatsecret')
		ON CONFLICT (user_id, date) DO UPDATE SET
			calories_total = EXCLUDED.calories_total,
			protein_g = EXCLUDED.protein_g,
			carbs_g = EXCLUDED.carbs_g,
			fat_g = EXCLUDED.fat_g,
			fiber_g = EXCLUDED.fiber_g,
			water_ml = nutrition_daily.water_ml,
			source = 'fatsecret'
		RETURNING id
	`, userID, date, totalCal, totalProtein, totalCarbs, totalFat, totalFiber).Scan(&dailyID)
	if err != nil {
		return fmt.Errorf("upsert daily: %w", err)
	}

	c.db.Exec(ctx, `DELETE FROM nutrition_items WHERE daily_id = $1`, dailyID)

	for _, e := range entries {
		mealName := normalizeFatSecretMealType(e.Meal, e.MealID)
		serving := fmt.Sprintf("%.0f %s", parseFloat(e.NumberOfUnits), e.MeasurementDescription)
		macros, _ := json.Marshal(map[string]float64{
			"protein":             parseFloat(e.Protein),
			"carbs":               parseFloat(e.Carbohydrate),
			"fat":                 parseFloat(e.Fat),
			"saturated_fat":       parseFloat(e.SaturatedFat),
			"polyunsaturated_fat": parseFloat(e.PolyunsaturatedFat),
			"monounsaturated_fat": parseFloat(e.MonounsaturatedFat),
			"cholesterol":         parseFloat(e.Cholesterol),
			"sodium":              parseFloat(e.Sodium),
			"potassium":           parseFloat(e.Potassium),
			"fiber":               parseFloat(e.Fiber),
			"sugar":               parseFloat(e.Sugar),
			"vitamin_a":           parseFloat(e.VitaminA),
			"vitamin_c":           parseFloat(e.VitaminC),
			"calcium":             parseFloat(e.Calcium),
			"iron":                parseFloat(e.Iron),
		})
		c.db.Exec(ctx, `
			INSERT INTO nutrition_items (daily_id, meal_type, food_name, serving_description, calories, macros)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, dailyID, mealName, e.FoodEntryName, serving, parseFloat(e.Calories), macros)
	}

	c.logger.Info().Str("date", date.Format("2006-01-02")).
		Float64("calories", totalCal).Int("items", len(entries)).Msg("synced day")
	return nil
}
