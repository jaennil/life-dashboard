package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
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

// isFatSecretTransportFailure reports whether the request died on the way rather
// than being refused by the API. Only those are worth repeating: a refusal is a
// decision and will be repeated identically.
func isFatSecretTransportFailure(err error) bool {
	var failure *fatSecretFailure
	if errors.As(err, &failure) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded)
}

// findFoodEntry looks for a draft that may already be in the diary, matching on
// the day, the food, the serving and the quantity - the four things that make an
// entry the same entry.
func (c *FatSecretConnector) findFoodEntry(ctx context.Context, userID string, draft FoodEntryDraft) (string, error) {
	body, err := c.callAPI(ctx, userID, map[string]string{
		"method": "food_entries.get",
		"date":   strconv.Itoa(daysSinceEpoch(draft.Date)),
	})
	if err != nil {
		return "", err
	}

	var decoded struct {
		FoodEntries struct {
			FoodEntry json.RawMessage `json:"food_entry"`
		} `json:"food_entries"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("decode day entries: %w", err)
	}
	if len(decoded.FoodEntries.FoodEntry) == 0 {
		return "", nil
	}

	// One entry arrives as an object, several as an array.
	var entries []struct {
		FoodEntryID   fsValueString `json:"food_entry_id"`
		FoodID        fsValueString `json:"food_id"`
		ServingID     fsValueString `json:"serving_id"`
		NumberOfUnits fsValueString `json:"number_of_units"`
	}
	if err := json.Unmarshal(decoded.FoodEntries.FoodEntry, &entries); err != nil {
		var single struct {
			FoodEntryID   fsValueString `json:"food_entry_id"`
			FoodID        fsValueString `json:"food_id"`
			ServingID     fsValueString `json:"serving_id"`
			NumberOfUnits fsValueString `json:"number_of_units"`
		}
		if err := json.Unmarshal(decoded.FoodEntries.FoodEntry, &single); err != nil {
			return "", fmt.Errorf("decode day entries: %w", err)
		}
		entries = append(entries, single)
	}

	for _, entry := range entries {
		if entry.FoodID.Value != draft.FoodID || entry.ServingID.Value != draft.ServingID {
			continue
		}
		units, err := strconv.ParseFloat(entry.NumberOfUnits.Value, 64)
		if err != nil {
			continue
		}
		// The provider rounds the quantity it echoes back, so the comparison is
		// deliberately loose.
		if math.Abs(units-draft.NumberOfUnits) < 0.05 {
			return entry.FoodEntryID.Value, nil
		}
	}
	return "", nil
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
		// A write that never got an answer is not a write that never happened.
		// This network drops about one connection in twenty, and the timeout
		// arrives "while awaiting headers" - the entry may be in the diary
		// already. So the day is re-read, and the write is repeated only if the
		// entry is genuinely absent.
		if !isFatSecretTransportFailure(err) {
			return "", err
		}
		existing, lookupErr := c.findFoodEntry(ctx, userID, draft)
		if lookupErr != nil {
			return "", fmt.Errorf("%w (and the diary could not be re-read: %v)", err, lookupErr)
		}
		if existing != "" {
			c.logger.Info().Str("entry_id", existing).Msg("food entry had been written before the timeout")
			return existing, nil
		}
		body, err = c.callAPI(ctx, userID, params)
		if err != nil {
			return "", err
		}
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
// fatSecretFailure is a request that died on the way rather than being refused.
//
// It is its own type for two reasons. The URL has to go: every FatSecret URL is
// signed in the query string, so it carries the consumer key, the access token
// and the signature, and url.Error prints the URL it failed on - one timeout on
// food_entry.create put all three into the job result, the notification on the
// phone and the log store. And the fact that it was the transport has to stay,
// because that is what makes the write worth checking and repeating; stripping
// the URL by wrapping the inner error alone threw that away.
type fatSecretFailure struct {
	method string
	err    error
}

func (e *fatSecretFailure) Error() string {
	return "fatsecret " + e.method + " request failed: " + e.err.Error()
}

func (e *fatSecretFailure) Unwrap() error { return e.err }

func fatSecretTransportError(method string, err error) error {
	inner := err
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		inner = urlErr.Err
	}
	return &fatSecretFailure{method: method, err: inner}
}

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
		return nil, fatSecretTransportError(extra["method"], err)
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
