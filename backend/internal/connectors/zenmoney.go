package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const zenmoneyDiffURL = "https://api.zenmoney.ru/v8/diff/"

// ---- API types ----

type zenmoneyDiffRequest struct {
	CurrentClientTimestamp int64 `json:"currentClientTimestamp"`
	LastServerTimestamp    int64 `json:"lastServerTimestamp"`
}

type zenmoneyDiffResponse struct {
	ServerTimestamp int64                  `json:"serverTimestamp"`
	Instrument      []zenmoneyInstrument   `json:"instrument"`
	Account         []zenmoneyAccount      `json:"account"`
	Transaction     []zenmoneyTransaction  `json:"transaction"`
	Tag             []zenmoneyTag          `json:"tag"`
}

type zenmoneyTag struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Parent   *string `json:"parent"`
	Deleted  bool    `json:"deleted"`
}

type zenmoneyInstrument struct {
	ID         int    `json:"id"`
	ShortTitle string `json:"shortTitle"`
}

type zenmoneyAccount struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Type     string  `json:"type"`
	Currency int     `json:"currency"`
	Balance  float64 `json:"balance"`
	Archived bool    `json:"archived"`
	Deleted  bool    `json:"deleted"`
}

type zenmoneyTransaction struct {
	ID              string   `json:"id"`
	Date            string   `json:"date"` // YYYY-MM-DD
	Income          float64  `json:"income"`
	Outcome         float64  `json:"outcome"`
	IncomeAccount   string   `json:"incomeAccount"`
	OutcomeAccount  string   `json:"outcomeAccount"`
	IncomeCurrency  int      `json:"incomeCurrency"`
	OutcomeCurrency int      `json:"outcomeCurrency"`
	Comment         string   `json:"comment"`
	Payee           *string  `json:"payee"`
	Tag             []string `json:"tag"`
	Deleted         bool     `json:"deleted"`
}

// ---- Connector ----

type ZenmoneyConnector struct {
	token  string
	db     *pgxpool.Pool
	client *http.Client
	logger zerolog.Logger
}

func NewZenmoney(token string, db *pgxpool.Pool, logger zerolog.Logger) *ZenmoneyConnector {
	return &ZenmoneyConnector{
		token:  token,
		db:     db,
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger.With().Str("connector", "zenmoney").Logger(),
	}
}

func (z *ZenmoneyConnector) Name() string { return "zenmoney" }

func (z *ZenmoneyConnector) Sync(ctx context.Context, userID string) error {
	lastTS, err := z.getLastServerTimestamp(ctx, userID)
	if err != nil {
		return fmt.Errorf("get last server timestamp: %w", err)
	}

	z.logger.Info().Int64("last_server_timestamp", lastTS).Msg("fetching diff")

	resp, err := z.fetchDiff(ctx, lastTS)
	if err != nil {
		return fmt.Errorf("fetch diff: %w", err)
	}

	currencies := make(map[int]string, len(resp.Instrument))
	for _, inst := range resp.Instrument {
		currencies[inst.ID] = inst.ShortTitle
	}

	// Upsert tags and build id→title map
	tagNames := make(map[string]string)
	for i := range resp.Tag {
		t := &resp.Tag[i]
		if t.Deleted {
			z.db.Exec(ctx, `DELETE FROM zenmoney_tags WHERE id = $1 AND user_id = $2`, t.ID, userID)
			continue
		}
		z.db.Exec(ctx, `
			INSERT INTO zenmoney_tags (id, title, parent_id, updated_at, user_id)
			VALUES ($1, $2, $3, NOW(), $4)
			ON CONFLICT (id) DO UPDATE SET title = EXCLUDED.title, parent_id = EXCLUDED.parent_id, updated_at = NOW()
		`, t.ID, t.Title, t.Parent, userID)
		tagNames[t.ID] = t.Title
	}
	// Fill tagNames from DB for tags not in current diff
	if len(tagNames) == 0 {
		rows, err := z.db.Query(ctx, `SELECT id, title FROM zenmoney_tags WHERE user_id = $1`, userID)
		if err == nil {
			for rows.Next() {
				var id, title string
				if rows.Scan(&id, &title) == nil {
					tagNames[id] = title
				}
			}
			rows.Close()
		}
	}
	z.logger.Info().Int("count", len(resp.Tag)).Msg("tags synced")

	for i := range resp.Account {
		if err := z.upsertAccount(ctx, userID, &resp.Account[i], currencies); err != nil {
			return fmt.Errorf("upsert account %s: %w", resp.Account[i].ID, err)
		}
	}
	z.logger.Info().Int("count", len(resp.Account)).Msg("accounts synced")

	synced, deleted := 0, 0
	for i := range resp.Transaction {
		tx := &resp.Transaction[i]
		if tx.Deleted {
			if err := z.deleteTransaction(ctx, userID, tx.ID); err != nil {
				return fmt.Errorf("delete transaction %s: %w", tx.ID, err)
			}
			deleted++
		} else {
			if err := z.upsertTransaction(ctx, userID, tx, currencies, tagNames); err != nil {
				return fmt.Errorf("upsert transaction %s: %w", tx.ID, err)
			}
			synced++
		}
	}
	z.logger.Info().Int("synced", synced).Int("deleted", deleted).Msg("transactions synced")

	// Backfill category for existing transactions that have tags but no category
	z.db.Exec(ctx, `
		UPDATE transactions t
		SET category = zt.title
		FROM zenmoney_tags zt
		WHERE t.category IS NULL
		  AND t.tags IS NOT NULL
		  AND zt.id = t.tags[1]
		  AND t.user_id = $1
	`, userID)

	return z.updateLastServerTimestamp(ctx, userID, resp.ServerTimestamp)
}

// ---- HTTP ----

func (z *ZenmoneyConnector) fetchDiff(ctx context.Context, lastServerTimestamp int64) (*zenmoneyDiffResponse, error) {
	body, err := json.Marshal(zenmoneyDiffRequest{
		CurrentClientTimestamp: time.Now().Unix(),
		LastServerTimestamp:    lastServerTimestamp,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, zenmoneyDiffURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+z.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := z.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("zenmoney api returned status %d", resp.StatusCode)
	}

	var result zenmoneyDiffResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// ---- DB ----

func (z *ZenmoneyConnector) upsertAccount(ctx context.Context, userID string, acc *zenmoneyAccount, currencies map[int]string) error {
	if acc.Deleted {
		_, err := z.db.Exec(ctx, `DELETE FROM accounts WHERE external_id = $1 AND user_id = $2`, acc.ID, userID)
		return err
	}

	currency := currencies[acc.Currency]
	if currency == "" {
		currency = "RUB"
	}

	_, err := z.db.Exec(ctx, `
		INSERT INTO accounts (external_id, title, type, currency, balance, last_updated, user_id)
		VALUES ($1, $2, $3, $4, $5, NOW(), $6)
		ON CONFLICT (user_id, external_id) DO UPDATE SET
			title        = EXCLUDED.title,
			type         = EXCLUDED.type,
			currency     = EXCLUDED.currency,
			balance      = EXCLUDED.balance,
			last_updated = EXCLUDED.last_updated
	`, acc.ID, acc.Title, acc.Type, currency, acc.Balance, userID)
	if err != nil {
		return err
	}

	z.logger.Debug().Str("id", acc.ID).Str("title", acc.Title).Float64("balance", acc.Balance).Msg("account upserted")
	return nil
}

func (z *ZenmoneyConnector) upsertTransaction(ctx context.Context, userID string, tx *zenmoneyTransaction, currencies map[int]string, tagNames map[string]string) error {
	isTransfer := tx.Income > 0 && tx.Outcome > 0

	var accountExternalID string
	var amount float64
	var currencyID int
	if tx.Income > 0 {
		accountExternalID = tx.IncomeAccount
		amount = tx.Income
		currencyID = tx.IncomeCurrency
	} else {
		accountExternalID = tx.OutcomeAccount
		amount = -tx.Outcome
		currencyID = tx.OutcomeCurrency
	}

	currency := currencies[currencyID]
	if currency == "" {
		currency = "RUB"
	}

	var accountID *string
	if accountExternalID != "" {
		var id string
		if err := z.db.QueryRow(ctx, `SELECT id FROM accounts WHERE external_id = $1 AND user_id = $2`, accountExternalID, userID).Scan(&id); err == nil {
			accountID = &id
		}
	}

	date, err := time.Parse("2006-01-02", tx.Date)
	if err != nil {
		date = time.Now()
	}

	var category *string
	if len(tx.Tag) > 0 {
		if name, ok := tagNames[tx.Tag[0]]; ok {
			category = &name
		}
	}

	_, err = z.db.Exec(ctx, `
		INSERT INTO transactions (external_id, account_id, occurred_at, amount, currency, payee, comment, tags, category, is_transfer, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (user_id, external_id) DO UPDATE SET
			account_id  = EXCLUDED.account_id,
			occurred_at = EXCLUDED.occurred_at,
			amount      = EXCLUDED.amount,
			currency    = EXCLUDED.currency,
			payee       = EXCLUDED.payee,
			comment     = EXCLUDED.comment,
			tags        = EXCLUDED.tags,
			category    = EXCLUDED.category,
			is_transfer = EXCLUDED.is_transfer
	`, tx.ID, accountID, date, amount, currency, tx.Payee, tx.Comment, tx.Tag, category, isTransfer, userID)

	if err != nil {
		return err
	}

	z.logger.Debug().Str("id", tx.ID).Str("date", tx.Date).Float64("amount", amount).Msg("transaction upserted")
	return nil
}

func (z *ZenmoneyConnector) deleteTransaction(ctx context.Context, userID string, externalID string) error {
	_, err := z.db.Exec(ctx, `DELETE FROM transactions WHERE external_id = $1 AND user_id = $2`, externalID, userID)
	if err != nil {
		return err
	}
	z.logger.Debug().Str("external_id", externalID).Msg("transaction deleted")
	return nil
}

func (z *ZenmoneyConnector) getLastServerTimestamp(ctx context.Context, userID string) (int64, error) {
	var t time.Time
	err := z.db.QueryRow(ctx, `SELECT last_synced_at FROM sync_state WHERE source = 'zenmoney' AND user_id = $1`, userID).Scan(&t)
	if err != nil {
		return 0, nil // first sync
	}
	return t.Unix(), nil
}

func (z *ZenmoneyConnector) updateLastServerTimestamp(ctx context.Context, userID string, serverTimestamp int64) error {
	t := time.Unix(serverTimestamp, 0)
	_, err := z.db.Exec(ctx, `
		INSERT INTO sync_state (source, last_synced_at, updated_at, user_id)
		VALUES ('zenmoney', $1, NOW(), $2)
		ON CONFLICT (source, user_id) DO UPDATE SET
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at     = EXCLUDED.updated_at
	`, t, userID)
	return err
}
