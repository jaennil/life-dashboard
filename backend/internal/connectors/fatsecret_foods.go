package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Meals the history endpoints accept. "all" is rejected with "Invalid Type: meal
// is invalid", so every meal is queried separately.
var fatSecretMeals = []string{"breakfast", "lunch", "dinner", "other"}

// fatSecretCatalogueMaxAge keeps the catalogue refresh off the regular sync path.
// The sync runs every fifteen minutes, the catalogue changes only when something
// new is eaten, and FatSecret rate-limits the account - so eight extra calls per
// sync would spend the budget on nothing.
const fatSecretCatalogueMaxAge = 24 * time.Hour

// fsFoodList decodes the "foods" envelope. FatSecret returns a bare object rather
// than a one-element array when a single food matches, so the field cannot be
// decoded as a slice directly.
type fsFoodList struct {
	Foods []fsCatalogueFood
}

func (l *fsFoodList) UnmarshalJSON(data []byte) error {
	var single struct {
		Food json.RawMessage `json:"food"`
	}
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	if len(single.Food) == 0 {
		return nil
	}

	if err := json.Unmarshal(single.Food, &l.Foods); err == nil {
		return nil
	}

	var one fsCatalogueFood
	if err := json.Unmarshal(single.Food, &one); err != nil {
		return fmt.Errorf("decode food list: %w", err)
	}
	l.Foods = []fsCatalogueFood{one}
	return nil
}

type fsCatalogueFood struct {
	FoodID    string `json:"food_id"`
	FoodName  string `json:"food_name"`
	BrandName string `json:"brand_name"`
	FoodType  string `json:"food_type"`
	FoodURL   string `json:"food_url"`
}

type fsFoodsResponse struct {
	Foods fsFoodList `json:"foods"`
}

// syncFoodCatalogue refreshes the account's own food list from its eating
// history. Both endpoints are queried because they answer different questions:
// most_eaten is what the account eats habitually, recently_eaten catches what it
// started eating this week.
func (c *FatSecretConnector) syncFoodCatalogue(ctx context.Context, userID, token, secret string) error {
	fresh, err := c.catalogueIsFresh(ctx, userID)
	if err != nil {
		return fmt.Errorf("check catalogue age: %w", err)
	}
	if fresh {
		c.logger.Debug().Msg("food catalogue is fresh, skipping")
		return nil
	}

	catalogue := map[string]*fsCatalogueEntry{}
	for _, method := range []string{"foods.get_most_eaten", "foods.get_recently_eaten"} {
		for _, meal := range fatSecretMeals {
			foods, err := c.fetchFoodHistory(ctx, token, secret, method, meal)
			if err != nil {
				// One dead combination must not cost the other seven.
				c.logger.Warn().Err(err).Str("method", method).Str("meal", meal).
					Msg("food history request failed")
				continue
			}
			collectFoodHistory(catalogue, foods, method, meal)
		}
	}

	saved := 0
	for _, entry := range catalogue {
		if err := c.upsertCatalogueFood(ctx, userID, entry); err != nil {
			return fmt.Errorf("upsert food %s: %w", entry.food.FoodID, err)
		}
		saved++
	}

	c.logger.Info().Int("foods", saved).Msg("food catalogue synced")
	return nil
}

// fsCatalogueEntry accumulates one food across the eight history responses.
type fsCatalogueEntry struct {
	food   fsCatalogueFood
	source string
	meals  map[string]bool
	// rank is the best position the food reached in a most-eaten list, 1-based.
	// Zero means it never appeared in one.
	rank int
}

// mealList returns the meals in a stable order.
func (e *fsCatalogueEntry) mealList() []string {
	meals := make([]string, 0, len(e.meals))
	for _, meal := range fatSecretMeals {
		if e.meals[meal] {
			meals = append(meals, meal)
		}
	}
	return meals
}

// collectFoodHistory folds one history response into the catalogue being built.
//
// The response order carries the signal: foods.get_most_eaten returns the most
// habitual first, so position is frequency. The best position across meals is
// kept, because a food that ranks first at breakfast is a habit even if it never
// appears at dinner.
func collectFoodHistory(catalogue map[string]*fsCatalogueEntry, foods []fsCatalogueFood, method, meal string) {
	source := historySource(method)
	for position, food := range foods {
		if food.FoodID == "" {
			continue
		}

		entry, seen := catalogue[food.FoodID]
		if !seen {
			entry = &fsCatalogueEntry{food: food, source: source, meals: map[string]bool{}}
			catalogue[food.FoodID] = entry
		}
		// most_eaten wins over recently_eaten as the recorded source.
		if source == "most_eaten" {
			entry.source = "most_eaten"
		}
		entry.meals[meal] = true

		// Only the most-eaten lists are ranked by frequency; position in a recent
		// list means recency, which is a different question.
		if source == "most_eaten" {
			rank := position + 1
			if entry.rank == 0 || rank < entry.rank {
				entry.rank = rank
			}
		}
	}
}

func historySource(method string) string {
	if strings.Contains(method, "most_eaten") {
		return "most_eaten"
	}
	return "recently_eaten"
}

// catalogueIsFresh reports whether the catalogue was refreshed recently enough to
// skip. An empty catalogue is never fresh.
func (c *FatSecretConnector) catalogueIsFresh(ctx context.Context, userID string) (bool, error) {
	var fresh bool
	err := c.db.QueryRow(ctx, `
		SELECT COALESCE(MAX(updated_at) > NOW() - $2::interval, false)
		FROM fatsecret_foods WHERE user_id = $1
	`, userID, fatSecretCatalogueMaxAge.String()).Scan(&fresh)
	return fresh, err
}

func (c *FatSecretConnector) fetchFoodHistory(ctx context.Context, token, secret, method, meal string) ([]fsCatalogueFood, error) {
	params := c.oauth1BaseParams(token)
	params.Set("method", method)
	params.Set("meal", meal)
	params.Set("format", "json")

	params.Set("oauth_signature", c.oauth1Sign(http.MethodGet, fsAPIBase, params, secret))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fsAPIBase+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, truncateForLog(body))
	}

	// FatSecret answers 200 with an error document, so the body has to be checked
	// rather than the status code.
	if apiErr := fatSecretAPIError(body); apiErr != nil {
		return nil, apiErr
	}

	var decoded fsFoodsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode %s: %w", method, err)
	}
	return decoded.Foods.Foods, nil
}

func (c *FatSecretConnector) upsertCatalogueFood(ctx context.Context, userID string, entry *fsCatalogueEntry) error {
	raw, err := json.Marshal(entry.food)
	if err != nil {
		return err
	}

	_, err = c.db.Exec(ctx, `
		INSERT INTO fatsecret_foods (
			user_id, food_id, food_name, brand_name, food_type, food_url,
			source, meals, most_eaten_rank, last_seen_at, raw_payload, updated_at
		)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), $7, $8,
		        NULLIF($9, 0), NOW(), $10::jsonb, NOW())
		ON CONFLICT (user_id, food_id) DO UPDATE SET
			food_name    = EXCLUDED.food_name,
			brand_name   = EXCLUDED.brand_name,
			food_type    = EXCLUDED.food_type,
			food_url     = EXCLUDED.food_url,
			source       = EXCLUDED.source,
			meals        = EXCLUDED.meals,
			-- A food that slipped out of the top twenty keeps its last known rank
			-- rather than losing the signal entirely. The rank goes slightly stale;
			-- dropping it would leave nothing to break a tie with.
			most_eaten_rank = COALESCE(EXCLUDED.most_eaten_rank, fatsecret_foods.most_eaten_rank),
			last_seen_at = NOW(),
			raw_payload  = EXCLUDED.raw_payload,
			updated_at   = NOW()
	`, userID, entry.food.FoodID, entry.food.FoodName, entry.food.BrandName,
		entry.food.FoodType, entry.food.FoodURL, entry.source, entry.mealList(),
		entry.rank, raw)
	return err
}

// fatSecretAPIError extracts the error document FatSecret returns with a 200.
func fatSecretAPIError(body []byte) error {
	var envelope struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Error == nil {
		return nil
	}
	return fmt.Errorf("fatsecret error %d: %s", envelope.Error.Code, envelope.Error.Message)
}

func truncateForLog(body []byte) string {
	const limit = 200
	text := strings.TrimSpace(string(body))
	if len(text) > limit {
		return text[:limit]
	}
	return text
}
