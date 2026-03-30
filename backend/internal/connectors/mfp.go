package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	mfpWebBase = "https://www.myfitnesspal.com"
	mfpAPIBase = "https://api.myfitnesspal.com"
)

type MFPConnector struct {
	username      string
	password      string
	sessionCookie string
	db            *pgxpool.Pool
	logger        zerolog.Logger

	client      *http.Client
	bearerToken string
	userID      string
	tokenExpiry time.Time
}

func NewMFP(username, password, sessionCookie, accessToken, userID string, db *pgxpool.Pool, logger zerolog.Logger) *MFPConnector {
	jar, _ := cookiejar.New(nil)
	c := &MFPConnector{
		username:      username,
		password:      password,
		sessionCookie: sessionCookie,
		db:            db,
		logger:        logger.With().Str("connector", "mfp").Logger(),
		client:        &http.Client{Jar: jar, Timeout: 30 * time.Second},
	}
	if accessToken != "" {
		c.bearerToken = accessToken
		c.userID = userID
		c.tokenExpiry = time.Now().Add(10 * 24 * time.Hour)
		c.logger.Info().Str("user_id", userID).Msg("using pre-configured access token")
	}
	return c
}

func (c *MFPConnector) Name() string { return "myfitnesspal" }

func (c *MFPConnector) Sync(ctx context.Context, userID string) error {
	c.logger.Info().Str("user_id", userID).Msg("starting sync")

	if err := c.ensureAuth(ctx, userID); err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	// Sync last 14 days
	today := time.Now().Truncate(24 * time.Hour)
	for i := 0; i < 14; i++ {
		date := today.AddDate(0, 0, -i)
		if err := c.syncDay(ctx, userID, date); err != nil {
			c.logger.Warn().Err(err).Str("date", date.Format("2006-01-02")).Msg("failed to sync day")
		}
	}

	c.logger.Info().Msg("sync complete")
	return nil
}

func (c *MFPConnector) ensureAuth(ctx context.Context, userID string) error {
	if c.bearerToken != "" && time.Now().Before(c.tokenExpiry) {
		return nil
	}

	// Try to load/refresh from DB first
	if err := c.authFromDB(ctx, userID); err == nil {
		return nil
	}

	if c.sessionCookie != "" {
		return c.authWithSessionCookie(ctx)
	}
	return c.authWithCredentials(ctx)
}

func (c *MFPConnector) authFromDB(ctx context.Context, userID string) error {
	var accessToken, refreshToken string
	var expiresAt time.Time
	var athleteID int64
	err := c.db.QueryRow(ctx, `
		SELECT access_token, refresh_token, expires_at, athlete_id
		FROM oauth_tokens WHERE source = 'myfitnesspal' AND user_id = $1
	`, userID).Scan(&accessToken, &refreshToken, &expiresAt, &athleteID)
	if err != nil {
		return err
	}

	if time.Now().Before(expiresAt.Add(-5 * time.Minute)) {
		c.bearerToken = accessToken
		c.userID = fmt.Sprintf("%d", athleteID)
		c.tokenExpiry = expiresAt
		c.logger.Info().Msg("loaded token from db")
		return nil
	}

	// Token expired — try refresh
	return c.refreshToken(ctx, userID, refreshToken)
}

func (c *MFPConnector) refreshToken(ctx context.Context, userID string, refreshTok string) error {
	c.logger.Info().Str("user_id", userID).Msg("refreshing access token")

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshTok)
	form.Set("client_id", "mfp-main-js")

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		mfpAPIBase+"/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	c.setWebHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh status %d", resp.StatusCode)
	}

	var tokenData struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		UserID       string `json:"user_id"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenData); err != nil {
		return fmt.Errorf("refresh decode: %w", err)
	}
	if tokenData.AccessToken == "" {
		return fmt.Errorf("empty access token from refresh")
	}

	expiresAt := time.Now().Add(time.Duration(tokenData.ExpiresIn-300) * time.Second)
	c.bearerToken = tokenData.AccessToken
	if tokenData.UserID != "" {
		c.userID = tokenData.UserID
	}
	c.tokenExpiry = expiresAt

	// Persist new tokens to DB
	newRT := refreshTok
	if tokenData.RefreshToken != "" {
		newRT = tokenData.RefreshToken
	}
	c.db.Exec(ctx, `
		UPDATE oauth_tokens SET access_token=$1, refresh_token=$2, expires_at=$3, updated_at=NOW()
		WHERE source='myfitnesspal' AND user_id = $4
	`, tokenData.AccessToken, newRT, expiresAt, userID)
	c.logger.Info().Str("user_id", userID).Msg("token refreshed")
	return nil
}

func (c *MFPConnector) authWithSessionCookie(ctx context.Context) error {
	c.logger.Info().Msg("authenticating with session cookie")

	mfpURL, _ := url.Parse(mfpWebBase)
	c.client.Jar.SetCookies(mfpURL, []*http.Cookie{
		{Name: "_mfp_session", Value: c.sessionCookie, Domain: "www.myfitnesspal.com", Path: "/"},
	})

	return c.fetchBearerToken(ctx)
}

func (c *MFPConnector) authWithCredentials(ctx context.Context) error {
	c.logger.Info().Msg("authenticating with credentials")

	// Step 1: get CSRF token
	csrfReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, mfpWebBase+"/api/auth/csrf", nil)
	c.setWebHeaders(csrfReq)

	csrfResp, err := c.client.Do(csrfReq)
	if err != nil {
		return fmt.Errorf("csrf request: %w", err)
	}
	defer csrfResp.Body.Close()

	var csrfData struct {
		CsrfToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(csrfResp.Body).Decode(&csrfData); err != nil {
		return fmt.Errorf("csrf decode: %w", err)
	}
	c.logger.Debug().Str("csrf", csrfData.CsrfToken[:min(8, len(csrfData.CsrfToken))]).Msg("got csrf token")

	// Step 2: login with credentials
	formData := url.Values{}
	formData.Set("username", c.username)
	formData.Set("password", c.password)
	formData.Set("csrfToken", csrfData.CsrfToken)
	formData.Set("json", "true")
	formData.Set("callbackUrl", mfpWebBase)

	loginReq, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		mfpWebBase+"/api/auth/callback/credentials", strings.NewReader(formData.Encode()))
	c.setWebHeaders(loginReq)
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("Referer", mfpWebBase+"/login")

	loginResp, err := c.client.Do(loginReq)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusOK && loginResp.StatusCode != http.StatusFound {
		return fmt.Errorf("login failed with status %d", loginResp.StatusCode)
	}

	return c.fetchBearerToken(ctx)
}

func (c *MFPConnector) fetchBearerToken(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		mfpWebBase+"/user/auth_token?refresh=true", nil)
	c.setWebHeaders(req)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("auth_token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth_token status %d — check credentials or session cookie", resp.StatusCode)
	}

	var tokenData struct {
		AccessToken string `json:"access_token"`
		UserID      string `json:"user_id"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenData); err != nil {
		return fmt.Errorf("auth_token decode: %w", err)
	}
	if tokenData.AccessToken == "" {
		return fmt.Errorf("empty access token — authentication failed")
	}

	c.bearerToken = tokenData.AccessToken
	c.userID = tokenData.UserID
	c.tokenExpiry = time.Now().Add(time.Duration(tokenData.ExpiresIn-300) * time.Second)
	c.logger.Info().Str("user_id", c.userID).Msg("authenticated")
	return nil
}

func (c *MFPConnector) syncDay(ctx context.Context, userID string, date time.Time) error {
	dateStr := date.Format("2006-01-02")

	apiURL := fmt.Sprintf("%s/v2/diary?fields%%5B%%5D=all&entry_date=%s&types=food_entry,diary_meal",
		mfpAPIBase, dateStr)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	req.Header.Set("mfp-client-id", "mfp-main-js")
	req.Header.Set("mfp-user-id", c.userID)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("diary request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil // no diary entries for this day
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("diary status %d", resp.StatusCode)
	}

	var diaryResp struct {
		Items []mfpDiaryItem `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&diaryResp); err != nil {
		return fmt.Errorf("diary decode: %w", err)
	}

	return c.storeDiaryItems(ctx, userID, date, diaryResp.Items)
}

type mfpNutrients struct {
	Energy struct {
		Value float64 `json:"value"`
	} `json:"energy"`
	Protein      float64 `json:"protein"`
	Carbohydrates float64 `json:"carbohydrates"`
	Fat          float64 `json:"fat"`
	Fiber        float64 `json:"fiber"`
	Sugar        float64 `json:"sugar"`
	Sodium       float64 `json:"sodium"`
}

type mfpDiaryItem struct {
	Type       string       `json:"type"`
	MealName   string       `json:"meal_name"`
	DiaryMeal  string       `json:"diary_meal"`
	Food       *struct {
		Description string `json:"description"`
	} `json:"food"`
	ServingSize *struct {
		Value float64 `json:"value"`
		Unit  string  `json:"unit"`
	} `json:"serving_size"`
	Servings            float64      `json:"servings"`
	NutritionalContents mfpNutrients `json:"nutritional_contents"`
}

func (c *MFPConnector) storeDiaryItems(ctx context.Context, userID string, date time.Time, items []mfpDiaryItem) error {
	// Find diary_meal totals per meal and daily total
	var totalCal, totalProtein, totalCarbs, totalFat, totalFiber float64
	for _, item := range items {
		if item.Type == "diary_meal" && item.DiaryMeal == "" {
			// top-level daily total — some API versions return this
		}
		if item.Type == "food_entry" {
			n := item.NutritionalContents
			totalCal += n.Energy.Value
			totalProtein += n.Protein
			totalCarbs += n.Carbohydrates
			totalFat += n.Fat
			totalFiber += n.Fiber
		}
	}

	if totalCal == 0 && len(items) == 0 {
		return nil
	}

	// Upsert daily row
	var dailyID string
	err := c.db.QueryRow(ctx, `
		INSERT INTO nutrition_daily (user_id, date, calories_total, protein_g, carbs_g, fat_g, fiber_g, source)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'myfitnesspal')
		ON CONFLICT (user_id, date) DO UPDATE SET
			calories_total = EXCLUDED.calories_total,
			protein_g = EXCLUDED.protein_g,
			carbs_g = EXCLUDED.carbs_g,
			fat_g = EXCLUDED.fat_g,
			fiber_g = EXCLUDED.fiber_g,
			source = 'myfitnesspal'
		RETURNING id
	`, userID, date, totalCal, totalProtein, totalCarbs, totalFat, totalFiber).Scan(&dailyID)
	if err != nil {
		return fmt.Errorf("upsert daily: %w", err)
	}

	// Delete old items for this day and re-insert
	c.db.Exec(ctx, `DELETE FROM nutrition_items WHERE daily_id = $1`, dailyID)

	for _, item := range items {
		if item.Type != "food_entry" {
			continue
		}
		foodName := ""
		if item.Food != nil {
			foodName = item.Food.Description
		}
		servingDesc := ""
		if item.ServingSize != nil {
			servingDesc = fmt.Sprintf("%.0f %s", item.ServingSize.Value*item.Servings, item.ServingSize.Unit)
		}
		macros, _ := json.Marshal(map[string]float64{
			"protein": item.NutritionalContents.Protein,
			"carbs":   item.NutritionalContents.Carbohydrates,
			"fat":     item.NutritionalContents.Fat,
			"fiber":   item.NutritionalContents.Fiber,
			"sugar":   item.NutritionalContents.Sugar,
			"sodium":  item.NutritionalContents.Sodium,
		})

		mealName := item.MealName
		if mealName == "" {
			mealName = item.DiaryMeal
		}

		c.db.Exec(ctx, `
			INSERT INTO nutrition_items (daily_id, meal_type, food_name, serving_description, calories, macros)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, dailyID, strings.ToLower(mealName), foodName, servingDesc,
			item.NutritionalContents.Energy.Value, macros)
	}

	c.logger.Info().Str("date", date.Format("2006-01-02")).
		Float64("calories", totalCal).
		Int("items", len(items)).
		Msg("synced day")
	return nil
}

func (c *MFPConnector) setWebHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", mfpWebBase)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
