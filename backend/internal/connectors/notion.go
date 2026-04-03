package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const notionAPIBase = "https://api.notion.com/v1"
const notionAPIVersion = "2022-06-28"

// ---- Notion API types ----

type notionQueryResponse struct {
	Results    []notionPage `json:"results"`
	HasMore    bool         `json:"has_more"`
	NextCursor string       `json:"next_cursor"`
}

type notionPage struct {
	ID             string                        `json:"id"`
	CreatedTime    time.Time                     `json:"created_time"`
	LastEditedTime time.Time                     `json:"last_edited_time"`
	Properties     map[string]notionPropertyValue `json:"properties"`
}

type notionPropertyValue struct {
	Type        string             `json:"type"`
	Title       []notionRichText   `json:"title,omitempty"`
	RichText    []notionRichText   `json:"rich_text,omitempty"`
	Date        *notionDateProp    `json:"date,omitempty"`
	Number      *float64           `json:"number,omitempty"`
	MultiSelect []notionSelectItem `json:"multi_select,omitempty"`
	Select      *notionSelectItem  `json:"select,omitempty"`
}

type notionRichText struct {
	PlainText string `json:"plain_text"`
}

type notionDateProp struct {
	Start string `json:"start"`
	End   string `json:"end,omitempty"`
}

type notionSelectItem struct {
	Name string `json:"name"`
}

type notionBlocksResponse struct {
	Results    []notionBlock `json:"results"`
	HasMore    bool          `json:"has_more"`
	NextCursor string        `json:"next_cursor"`
}

type notionBlock struct {
	Type      string          `json:"type"`
	Paragraph *notionTextBlock `json:"paragraph,omitempty"`
	Heading1  *notionTextBlock `json:"heading_1,omitempty"`
	Heading2  *notionTextBlock `json:"heading_2,omitempty"`
	Heading3  *notionTextBlock `json:"heading_3,omitempty"`
	BulletedListItem *notionTextBlock `json:"bulleted_list_item,omitempty"`
	NumberedListItem *notionTextBlock `json:"numbered_list_item,omitempty"`
	Toggle    *notionTextBlock `json:"toggle,omitempty"`
	Quote     *notionTextBlock `json:"quote,omitempty"`
	Callout   *notionTextBlock `json:"callout,omitempty"`
	ToDo      *notionToDo      `json:"to_do,omitempty"`
}

type notionTextBlock struct {
	RichText []notionRichText `json:"rich_text"`
}

type notionToDo struct {
	RichText []notionRichText `json:"rich_text"`
	Checked  bool             `json:"checked"`
}

// ---- Connector ----

type NotionConnector struct {
	db     *pgxpool.Pool
	client *http.Client
	logger zerolog.Logger
}

func NewNotion(db *pgxpool.Pool, logger zerolog.Logger) *NotionConnector {
	return &NotionConnector{
		db:     db,
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger.With().Str("connector", "notion").Logger(),
	}
}

func (n *NotionConnector) Name() string { return "notion" }

func (n *NotionConnector) loadCredentials(ctx context.Context, userID string) (apiToken string, databaseID string, err error) {
	err = n.db.QueryRow(ctx,
		`SELECT access_token, COALESCE(athlete_id, '') FROM oauth_tokens WHERE source = 'notion' AND user_id = $1`,
		userID,
	).Scan(&apiToken, &databaseID)
	if err != nil {
		return "", "", fmt.Errorf("no Notion credentials — add your Notion token and database ID in Settings")
	}
	if databaseID == "" {
		return "", "", fmt.Errorf("no Notion database ID configured")
	}
	return apiToken, databaseID, nil
}

func (n *NotionConnector) Sync(ctx context.Context, userID string) error {
	apiToken, databaseID, err := n.loadCredentials(ctx, userID)
	if err != nil {
		return err
	}

	n.logger.Info().Str("database_id", databaseID).Msg("starting notion sync")

	pages, err := n.queryDatabase(ctx, apiToken, databaseID)
	if err != nil {
		return fmt.Errorf("query notion database: %w", err)
	}

	n.logger.Info().Int("pages", len(pages)).Msg("fetched pages from notion")

	synced := 0
	for _, page := range pages {
		content, err := n.getPageContent(ctx, apiToken, page.ID)
		if err != nil {
			n.logger.Warn().Err(err).Str("page_id", page.ID).Msg("failed to fetch page content, skipping")
			continue
		}

		entry := n.pageToEntry(page, content)
		if err := n.upsertEntry(ctx, userID, entry); err != nil {
			n.logger.Error().Err(err).Str("page_id", page.ID).Msg("failed to upsert journal entry")
			continue
		}
		synced++
	}

	if err := n.updateLastSync(ctx, userID); err != nil {
		return fmt.Errorf("update last sync: %w", err)
	}

	n.logger.Info().Int("synced", synced).Int("total", len(pages)).Msg("notion sync complete")
	return nil
}

func (n *NotionConnector) queryDatabase(ctx context.Context, token, databaseID string) ([]notionPage, error) {
	var allPages []notionPage
	var cursor string

	for {
		body := `{"page_size": 100`
		if cursor != "" {
			body += fmt.Sprintf(`, "start_cursor": "%s"`, cursor)
		}
		body += `}`

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			fmt.Sprintf("%s/databases/%s/query", notionAPIBase, databaseID),
			strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		n.setHeaders(req, token)

		resp, err := n.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("notion API returned %d: %s", resp.StatusCode, string(respBody))
		}

		var result notionQueryResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}

		allPages = append(allPages, result.Results...)
		n.logger.Debug().Int("batch", len(result.Results)).Int("total", len(allPages)).Bool("has_more", result.HasMore).Msg("fetched page batch")

		if !result.HasMore {
			break
		}
		cursor = result.NextCursor
	}

	return allPages, nil
}

func (n *NotionConnector) getPageContent(ctx context.Context, token, pageID string) (string, error) {
	var parts []string
	var cursor string

	for {
		url := fmt.Sprintf("%s/blocks/%s/children?page_size=100", notionAPIBase, pageID)
		if cursor != "" {
			url += "&start_cursor=" + cursor
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		n.setHeaders(req, token)

		resp, err := n.client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return "", fmt.Errorf("notion blocks API returned %d: %s", resp.StatusCode, string(respBody))
		}

		var result notionBlocksResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", fmt.Errorf("decode blocks: %w", err)
		}

		for _, block := range result.Results {
			text := n.extractBlockText(block)
			if text != "" {
				parts = append(parts, text)
			}
		}

		if !result.HasMore {
			break
		}
		cursor = result.NextCursor
	}

	return strings.Join(parts, "\n"), nil
}

func (n *NotionConnector) extractBlockText(block notionBlock) string {
	var textBlock *notionTextBlock

	switch block.Type {
	case "paragraph":
		textBlock = block.Paragraph
	case "heading_1":
		textBlock = block.Heading1
	case "heading_2":
		textBlock = block.Heading2
	case "heading_3":
		textBlock = block.Heading3
	case "bulleted_list_item":
		textBlock = block.BulletedListItem
	case "numbered_list_item":
		textBlock = block.NumberedListItem
	case "toggle":
		textBlock = block.Toggle
	case "quote":
		textBlock = block.Quote
	case "callout":
		textBlock = block.Callout
	case "to_do":
		if block.ToDo != nil {
			text := richTextToPlain(block.ToDo.RichText)
			if block.ToDo.Checked {
				return "[x] " + text
			}
			return "[ ] " + text
		}
	}

	if textBlock != nil {
		return richTextToPlain(textBlock.RichText)
	}
	return ""
}

func richTextToPlain(rt []notionRichText) string {
	var parts []string
	for _, r := range rt {
		if r.PlainText != "" {
			parts = append(parts, r.PlainText)
		}
	}
	return strings.Join(parts, "")
}

type journalEntry struct {
	externalID string
	date       *time.Time
	title      string
	content    string
	tags       []string
	mood       *int
	createdAt  time.Time
	updatedAt  time.Time
}

func (n *NotionConnector) pageToEntry(page notionPage, content string) journalEntry {
	entry := journalEntry{
		externalID: page.ID,
		content:    content,
		createdAt:  page.CreatedTime,
		updatedAt:  page.LastEditedTime,
	}

	for propName, prop := range page.Properties {
		switch {
		case prop.Type == "title" && len(prop.Title) > 0:
			entry.title = richTextToPlain(prop.Title)

		case prop.Type == "date" && prop.Date != nil:
			if t, err := time.Parse("2006-01-02", prop.Date.Start); err == nil {
				entry.date = &t
			} else if t, err := time.Parse(time.RFC3339, prop.Date.Start); err == nil {
				d := t.Truncate(24 * time.Hour)
				entry.date = &d
			}

		case prop.Type == "multi_select":
			for _, s := range prop.MultiSelect {
				entry.tags = append(entry.tags, s.Name)
			}

		case prop.Type == "select" && prop.Select != nil:
			lowerName := strings.ToLower(propName)
			if lowerName == "mood" || lowerName == "настроение" {
				// Try to map mood name to number
				entry.tags = append(entry.tags, prop.Select.Name)
			} else {
				entry.tags = append(entry.tags, prop.Select.Name)
			}

		case prop.Type == "number" && prop.Number != nil:
			lowerName := strings.ToLower(propName)
			if lowerName == "mood" || lowerName == "настроение" {
				m := int(*prop.Number)
				entry.mood = &m
			}
		}
	}

	// If no date property found, use created_time
	if entry.date == nil {
		d := page.CreatedTime.Truncate(24 * time.Hour)
		entry.date = &d
	}

	return entry
}

func (n *NotionConnector) upsertEntry(ctx context.Context, userID string, e journalEntry) error {
	_, err := n.db.Exec(ctx, `
		INSERT INTO journal_entries (user_id, source, external_id, date, title, content, tags, mood, created_at, updated_at)
		VALUES ($1, 'notion', $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (user_id, source, external_id) DO UPDATE SET
			date = EXCLUDED.date,
			title = EXCLUDED.title,
			content = EXCLUDED.content,
			tags = EXCLUDED.tags,
			mood = EXCLUDED.mood,
			updated_at = EXCLUDED.updated_at,
			ingested_at = NOW()
	`, userID, e.externalID, e.date, e.title, e.content, e.tags, e.mood, e.createdAt, e.updatedAt)
	if err != nil {
		return fmt.Errorf("upsert journal entry: %w", err)
	}

	n.logger.Debug().Str("external_id", e.externalID).Str("title", e.title).Msg("journal entry upserted")
	return nil
}

func (n *NotionConnector) updateLastSync(ctx context.Context, userID string) error {
	_, err := n.db.Exec(ctx, `
		INSERT INTO sync_state (source, last_synced_at, updated_at, user_id)
		VALUES ('notion', NOW(), NOW(), $1)
		ON CONFLICT (source, user_id) DO UPDATE SET
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at     = EXCLUDED.updated_at
	`, userID)
	return err
}

func (n *NotionConnector) setHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", notionAPIVersion)
	req.Header.Set("Content-Type", "application/json")
}
