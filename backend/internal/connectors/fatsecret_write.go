package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// FoodEntryDraft is one diary entry ready to be written.
//
// It carries provider identifiers rather than a food name on purpose: the search
// index available to this key holds no Russian foods, and food.get refuses the
// regional ids outright with "Invalid ID". The only pairs that work are the ones
// the account has already logged - which food_entry.create accepts happily, even
// though the same ids cannot be read back.
type FoodEntryDraft struct {
	FoodID        string
	ServingID     string
	NumberOfUnits float64
	// Meal is one of breakfast, lunch, dinner, other.
	Meal string
	Date time.Time
	// Name is what the diary shows. Empty leaves the provider's own naming.
	Name string
}

// fsValueString unwraps the {"value": "..."} envelope FatSecret wraps scalars in.
type fsValueString struct {
	Value string `json:"value"`
}

func (v *fsValueString) UnmarshalJSON(data []byte) error {
	var wrapped struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.Value != "" {
		v.Value = wrapped.Value
		return nil
	}
	// Tolerate a bare scalar in case the wrapping ever goes away.
	var bare string
	if err := json.Unmarshal(data, &bare); err == nil {
		v.Value = bare
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		v.Value = number.String()
		return nil
	}
	return fmt.Errorf("unsupported value envelope: %s", string(data))
}

type fsCreateEntryResponse struct {
	FoodEntryID fsValueString `json:"food_entry_id"`
}

type fsDeleteEntryResponse struct {
	Success fsValueString `json:"success"`
}

// CreateFoodEntry writes one entry to the diary and returns its provider id.
func (c *FatSecretConnector) CreateFoodEntry(ctx context.Context, userID string, draft FoodEntryDraft) (string, error) {
	if draft.FoodID == "" || draft.ServingID == "" {
		return "", fmt.Errorf("food entry needs both a food and a serving")
	}
	if draft.NumberOfUnits <= 0 {
		return "", fmt.Errorf("food entry needs a positive quantity")
	}

	params := map[string]string{
		"method":          "food_entry.create",
		"food_id":         draft.FoodID,
		"serving_id":      draft.ServingID,
		"number_of_units": strconv.FormatFloat(draft.NumberOfUnits, 'f', -1, 64),
		"meal":            draft.Meal,
		"date":            strconv.Itoa(daysSinceEpoch(draft.Date)),
	}
	if draft.Name != "" {
		params["food_entry_name"] = draft.Name
	}

	body, err := c.callAPI(ctx, userID, params)
	if err != nil {
		return "", err
	}

	var decoded fsCreateEntryResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("decode create entry: %w", err)
	}
	if decoded.FoodEntryID.Value == "" {
		// The entry may exist; without an id it cannot be recorded, and claiming
		// success would invite a duplicate on the next attempt.
		return "", fmt.Errorf("create entry returned no id: %s", truncateForLog(body))
	}
	return decoded.FoodEntryID.Value, nil
}

// DeleteFoodEntry removes an entry, which is what makes a wrong one recoverable
// without opening the app.
func (c *FatSecretConnector) DeleteFoodEntry(ctx context.Context, userID, entryID string) error {
	body, err := c.callAPI(ctx, userID, map[string]string{
		"method":        "food_entry.delete",
		"food_entry_id": entryID,
	})
	if err != nil {
		return err
	}

	var decoded fsDeleteEntryResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return fmt.Errorf("decode delete entry: %w", err)
	}
	if decoded.Success.Value != "1" {
		return fmt.Errorf("delete entry not confirmed: %s", truncateForLog(body))
	}
	return nil
}

// callAPI signs and sends one request, and treats the error document FatSecret
// returns with a 200 as the error it is.
func (c *FatSecretConnector) callAPI(ctx context.Context, userID string, extra map[string]string) ([]byte, error) {
	token, secret, err := c.getStoredTokens(ctx, userID)
	if err != nil {
		return nil, err
	}

	params := c.oauth1BaseParams(token)
	params.Set("format", "json")
	for key, value := range extra {
		params.Set(key, value)
	}
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
	if apiErr := fatSecretAPIError(body); apiErr != nil {
		return nil, apiErr
	}
	return body, nil
}
