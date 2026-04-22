package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	todoistAuthURL             = "https://app.todoist.com/oauth/authorize"
	todoistOAuthTokenURL       = "https://api.todoist.com/oauth/access_token"
	todoistOAuthScope          = "data:read"
	todoistSyncURL             = "https://api.todoist.com/api/v1/sync"
	todoistCompletedURL        = "https://api.todoist.com/api/v1/tasks/completed/by_completion_date"
	todoistInitialBackfillDays = 60
	todoistIncrementalLookback = 14
	todoistRequestTimeout      = 30 * time.Second
	todoistCompletedPageLimit  = 200
)

var errTodoistCompletedArchiveUnavailable = errors.New("todoist completed archive unavailable")
var errTodoistCompletedArchiveTemporaryUnavailable = errors.New("todoist completed archive temporary unavailable")

type todoistDB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type todoistSyncResponse struct {
	Items    []todoistItem    `json:"items"`
	Projects []todoistProject `json:"projects"`
	Sections []todoistSection `json:"sections"`
}

type todoistItem struct {
	ID          string      `json:"id"`
	ParentID    string      `json:"parent_id"`
	Content     string      `json:"content"`
	Description string      `json:"description"`
	ProjectID   string      `json:"project_id"`
	SectionID   string      `json:"section_id"`
	Labels      []string    `json:"labels"`
	Priority    int         `json:"priority"`
	Checked     bool        `json:"checked"`
	IsDeleted   bool        `json:"is_deleted"`
	AddedAt     string      `json:"added_at"`
	CompletedAt string      `json:"completed_at"`
	Due         *todoistDue `json:"due"`
}

type todoistDue struct {
	Date        string `json:"date"`
	DateTime    string `json:"datetime"`
	Recurring   bool   `json:"recurring"`
	IsRecurring bool   `json:"is_recurring"`
	String      string `json:"string"`
	Timezone    string `json:"timezone"`
}

type todoistProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type todoistSection struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
}

type todoistCompletedResponse struct {
	Items      []todoistItem `json:"items"`
	NextCursor string        `json:"next_cursor"`
}

type todoistCompletionEvent struct {
	TaskID      string
	CompletedAt time.Time
	ProjectID   string
	SectionID   string
	Content     string
	IsRecurring bool
	Raw         json.RawMessage
}

type TodoistConnector struct {
	clientID     string
	clientSecret string
	redirectURI  string
	db           *pgxpool.Pool
	client       *http.Client
	logger       zerolog.Logger
}

func NewTodoist(clientID, clientSecret, redirectURI string, db *pgxpool.Pool, logger zerolog.Logger) *TodoistConnector {
	return &TodoistConnector{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		db:           db,
		client:       &http.Client{Timeout: todoistRequestTimeout},
		logger:       logger.With().Str("connector", "todoist").Logger(),
	}
}

func (t *TodoistConnector) Name() string { return "todoist" }

func (t *TodoistConnector) OAuthConfigured() bool {
	return strings.TrimSpace(t.clientID) != "" && strings.TrimSpace(t.clientSecret) != ""
}

func (t *TodoistConnector) AuthURL(state string) string {
	params := url.Values{
		"client_id": {t.clientID},
		"scope":     {todoistOAuthScope},
		"state":     {state},
	}
	if strings.TrimSpace(t.redirectURI) != "" {
		params.Set("redirect_uri", t.redirectURI)
	}
	return todoistAuthURL + "?" + params.Encode()
}

func (t *TodoistConnector) ExchangeCode(ctx context.Context, userID, code string) error {
	if !t.OAuthConfigured() {
		return fmt.Errorf("todoist oauth is not configured")
	}

	form := url.Values{
		"client_id":     {t.clientID},
		"client_secret": {t.clientSecret},
		"code":          {code},
	}
	if strings.TrimSpace(t.redirectURI) != "" {
		form.Set("redirect_uri", t.redirectURI)
	}

	resp, err := t.client.PostForm(todoistOAuthTokenURL, form)
	if err != nil {
		return fmt.Errorf("todoist token request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("todoist token exchange returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("decode todoist token response: %w", err)
	}
	if strings.TrimSpace(result.AccessToken) == "" {
		return fmt.Errorf("todoist token response missing access_token")
	}

	_, err = t.db.Exec(ctx, `
		INSERT INTO oauth_tokens (source, access_token, refresh_token, expires_at, updated_at, user_id)
		VALUES ('todoist', $1, '', NOW() + INTERVAL '100 years', NOW(), $2)
		ON CONFLICT (source, user_id) DO UPDATE SET
			access_token = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			expires_at = EXCLUDED.expires_at,
			updated_at = NOW()
	`, result.AccessToken, userID)
	if err != nil {
		return fmt.Errorf("save todoist token: %w", err)
	}

	_, err = t.db.Exec(ctx, `
		INSERT INTO sync_state (source, enabled, updated_at, user_id)
		VALUES ('todoist', TRUE, NOW(), $1)
		ON CONFLICT (source, user_id) DO UPDATE SET enabled = TRUE, updated_at = NOW()
	`, userID)
	if err != nil {
		return fmt.Errorf("enable todoist sync state: %w", err)
	}

	t.logger.Info().Str("user_id", userID).Msg("todoist authorized")
	return nil
}

func (t *TodoistConnector) Sync(ctx context.Context, userID string) error {
	token, err := t.loadToken(ctx, userID)
	if err != nil {
		return err
	}

	tasks, projects, sections, err := t.fetchActiveTasks(ctx, token)
	if err != nil {
		return fmt.Errorf("fetch todoist tasks: %w", err)
	}

	lastSync, err := t.getLastSync(ctx, userID)
	if err != nil {
		return fmt.Errorf("get last sync: %w", err)
	}
	lookbackStart := t.syncStart(lastSync, time.Now())

	completions, completedArchiveAvailable, err := t.fetchCompletionEvents(ctx, token, lookbackStart)
	if err != nil {
		if errors.Is(err, errTodoistCompletedArchiveUnavailable) {
			completedArchiveAvailable = false
			t.logger.Warn().Str("user_id", userID).Msg("todoist completed archive not available on current plan; syncing active recurring tasks only")
		} else if errors.Is(err, errTodoistCompletedArchiveTemporaryUnavailable) {
			completedArchiveAvailable = false
			t.logger.Warn().Str("user_id", userID).Msg("todoist completed archive temporarily unavailable; syncing active recurring tasks only")
		} else {
			return fmt.Errorf("fetch todoist completed items: %w", err)
		}
	}

	tx, err := t.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	projectNames := make(map[string]string, len(projects))
	for _, project := range projects {
		projectNames[project.ID] = strings.TrimSpace(project.Name)
	}

	sectionNames := make(map[string]todoistSection, len(sections))
	for _, section := range sections {
		sectionNames[section.ID] = section
	}

	activeTaskIDs := make([]string, 0, len(tasks))
	recurringTaskIDs := make([]string, 0, len(tasks))
	habitIDs := make(map[string]string, len(tasks))
	for _, task := range tasks {
		raw, err := json.Marshal(task)
		if err != nil {
			return fmt.Errorf("marshal todoist task: %w", err)
		}
		if err := t.upsertTask(ctx, tx, userID, task, raw, projectNames, sectionNames); err != nil {
			return fmt.Errorf("upsert todoist task: %w", err)
		}
		activeTaskIDs = append(activeTaskIDs, task.ID)

		if !todoistIsRecurringHabit(task) {
			continue
		}

		habitID, err := t.upsertTaskAsHabit(ctx, tx, userID, task, raw, projectNames, sectionNames)
		if err != nil {
			return fmt.Errorf("upsert todoist habit: %w", err)
		}
		habitIDs[task.ID] = habitID
		recurringTaskIDs = append(recurringTaskIDs, task.ID)
	}

	if err := t.markInactiveTasks(ctx, tx, userID, activeTaskIDs); err != nil {
		return fmt.Errorf("mark inactive todoist tasks: %w", err)
	}

	if err := t.archiveMissingHabits(ctx, tx, userID, recurringTaskIDs); err != nil {
		return fmt.Errorf("archive missing todoist habits: %w", err)
	}

	if err := t.upsertCompletionEvents(ctx, tx, userID, completions, projectNames, sectionNames); err != nil {
		return fmt.Errorf("upsert todoist completion events: %w", err)
	}

	if completedArchiveAvailable {
		if err := t.clearRecentStatuses(ctx, tx, userID, lookbackStart); err != nil {
			return fmt.Errorf("clear recent todoist statuses: %w", err)
		}
		for _, event := range completions {
			habitID, ok := habitIDs[event.TaskID]
			if !ok {
				continue
			}
			if err := t.upsertCompletionStatus(ctx, tx, userID, habitID, event); err != nil {
				return fmt.Errorf("upsert todoist completion status: %w", err)
			}
		}
	}

	if err := t.updateLastSync(ctx, tx, userID); err != nil {
		return fmt.Errorf("update todoist last sync: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	t.logger.Info().Int("habits", len(tasks)).Int("completions", len(completions)).Bool("completed_archive_available", completedArchiveAvailable).Msg("todoist sync complete")
	return nil
}

func (t *TodoistConnector) loadToken(ctx context.Context, userID string) (string, error) {
	var token string
	err := t.db.QueryRow(ctx, `SELECT access_token FROM oauth_tokens WHERE source = 'todoist' AND user_id = $1`, userID).Scan(&token)
	if err != nil || strings.TrimSpace(token) == "" {
		return "", fmt.Errorf("no Todoist token — add your Todoist personal API token in Settings")
	}
	return token, nil
}

func (t *TodoistConnector) fetchActiveTasks(ctx context.Context, token string) ([]todoistItem, []todoistProject, []todoistSection, error) {
	form := url.Values{}
	form.Set("sync_token", "*")
	form.Set("resource_types", `["items","projects","sections"]`)

	respBody, err := t.doFormRequest(ctx, token, todoistSyncURL, form)
	if err != nil {
		return nil, nil, nil, err
	}

	var payload todoistSyncResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, nil, nil, fmt.Errorf("decode todoist sync response: %w", err)
	}

	active := make([]todoistItem, 0, len(payload.Items))
	for _, item := range payload.Items {
		if item.IsDeleted || item.Checked {
			continue
		}
		active = append(active, item)
	}
	return active, payload.Projects, payload.Sections, nil
}

func todoistIsRecurringHabit(item todoistItem) bool {
	if item.IsDeleted || item.Checked || item.Due == nil {
		return false
	}
	return todoistDueIsRecurring(item.Due)
}

func todoistDueIsRecurring(due *todoistDue) bool {
	return due != nil && (due.Recurring || due.IsRecurring)
}

func (t *TodoistConnector) fetchCompletionEvents(ctx context.Context, token string, since time.Time) ([]todoistCompletionEvent, bool, error) {
	completions := make([]todoistCompletionEvent, 0)
	cursor := ""
	until := time.Now().UTC()

	for {
		query := url.Values{}
		query.Set("since", since.UTC().Format(time.RFC3339))
		query.Set("until", until.Format(time.RFC3339))
		query.Set("limit", fmt.Sprintf("%d", todoistCompletedPageLimit))
		if cursor != "" {
			query.Set("cursor", cursor)
		}

		respBody, err := t.doJSONRequest(ctx, token, todoistCompletedURL+"?"+query.Encode())
		if err != nil {
			if errors.Is(err, errTodoistCompletedArchiveUnavailable) {
				return nil, false, err
			}
			return nil, false, err
		}

		var payload todoistCompletedResponse
		if err := json.Unmarshal(respBody, &payload); err != nil {
			return nil, false, fmt.Errorf("decode todoist completed response: %w", err)
		}

		for _, item := range payload.Items {
			completedAt, err := parseTodoistTime(item.CompletedAt)
			if err != nil {
				continue
			}
			raw, err := json.Marshal(item)
			if err != nil {
				return nil, false, fmt.Errorf("marshal todoist completed item: %w", err)
			}
			completions = append(completions, todoistCompletionEvent{
				TaskID:      strings.TrimSpace(item.ID),
				CompletedAt: completedAt,
				ProjectID:   strings.TrimSpace(item.ProjectID),
				SectionID:   strings.TrimSpace(item.SectionID),
				Content:     strings.TrimSpace(item.Content),
				IsRecurring: todoistDueIsRecurring(item.Due),
				Raw:         raw,
			})
		}

		if strings.TrimSpace(payload.NextCursor) == "" {
			break
		}
		cursor = payload.NextCursor
	}

	return completions, true, nil
}

func (t *TodoistConnector) doFormRequest(ctx context.Context, token, endpoint string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return t.do(req, false)
}

func (t *TodoistConnector) doJSONRequest(ctx context.Context, token, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return t.do(req, true)
}

func (t *TodoistConnector) do(req *http.Request, allowCompletedArchiveUnavailable bool) ([]byte, error) {
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if allowCompletedArchiveUnavailable && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusPaymentRequired) {
		return nil, errTodoistCompletedArchiveUnavailable
	}
	if allowCompletedArchiveUnavailable && (resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout) {
		return nil, errTodoistCompletedArchiveTemporaryUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("todoist api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

func (t *TodoistConnector) upsertTask(ctx context.Context, db todoistDB, userID string, task todoistItem, raw []byte, projects map[string]string, sections map[string]todoistSection) error {
	if _, err := db.Exec(ctx, `
		INSERT INTO raw_events (source, event_type, external_id, payload, user_id)
		VALUES ('todoist', 'active_task', $1, $2, $3)
	`, task.ID, raw, userID); err != nil {
		return fmt.Errorf("insert todoist raw active task event: %w", err)
	}

	projectName, sectionName := todoistProjectSectionNames(task.ProjectID, task.SectionID, projects, sections)
	addedAt, _ := parseTodoistTime(task.AddedAt)
	dueAt, dueDate, dueString, dueTimezone, isRecurring := todoistDueFields(task.Due)

	_, err := db.Exec(ctx, `
		INSERT INTO todoist_tasks (
			user_id, external_id, parent_external_id, project_external_id, project_name,
			section_external_id, section_name, content, description, labels, priority,
			is_recurring, is_active, added_at, due_at, due_date, due_string, due_timezone,
			raw_payload, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, TRUE, $13, $14, $15, $16, $17, $18, NOW())
		ON CONFLICT (user_id, external_id) DO UPDATE SET
			parent_external_id = EXCLUDED.parent_external_id,
			project_external_id = EXCLUDED.project_external_id,
			project_name = EXCLUDED.project_name,
			section_external_id = EXCLUDED.section_external_id,
			section_name = EXCLUDED.section_name,
			content = EXCLUDED.content,
			description = EXCLUDED.description,
			labels = EXCLUDED.labels,
			priority = EXCLUDED.priority,
			is_recurring = EXCLUDED.is_recurring,
			is_active = TRUE,
			added_at = COALESCE(EXCLUDED.added_at, todoist_tasks.added_at),
			due_at = EXCLUDED.due_at,
			due_date = EXCLUDED.due_date,
			due_string = EXCLUDED.due_string,
			due_timezone = EXCLUDED.due_timezone,
			raw_payload = EXCLUDED.raw_payload,
			updated_at = NOW()
	`, userID,
		strings.TrimSpace(task.ID),
		nullIfEmpty(task.ParentID),
		nullIfEmpty(task.ProjectID),
		nullIfEmpty(projectName),
		nullIfEmpty(task.SectionID),
		nullIfEmpty(sectionName),
		strings.TrimSpace(task.Content),
		nullIfEmpty(task.Description),
		task.Labels,
		task.Priority,
		isRecurring,
		nullTime(addedAt),
		nullTime(dueAt),
		nullDate(dueDate),
		nullIfEmpty(dueString),
		nullIfEmpty(dueTimezone),
		raw,
	)
	if err != nil {
		return fmt.Errorf("upsert todoist task row: %w", err)
	}
	return nil
}

func (t *TodoistConnector) upsertTaskAsHabit(ctx context.Context, db todoistDB, userID string, task todoistItem, raw []byte, projects map[string]string, sections map[string]todoistSection) (string, error) {
	if _, err := db.Exec(ctx, `
		INSERT INTO raw_events (source, event_type, external_id, payload, user_id)
		VALUES ('todoist', 'task', $1, $2, $3)
	`, task.ID, raw, userID); err != nil {
		return "", fmt.Errorf("insert todoist raw task event: %w", err)
	}

	areaName := todoistAreaName(task.ProjectID, task.SectionID, projects, sections)
	recurrence := ""
	if task.Due != nil {
		recurrence = strings.TrimSpace(task.Due.String)
	}
	timeOfDay := todoistTimeOfDay(task.Due)
	sourceCreatedAt, _ := parseTodoistTime(task.AddedAt)

	var habitID string
	err := db.QueryRow(ctx, `
		INSERT INTO habits (
			user_id, source, external_id, name, area_name, archived,
			recurrence, log_method, time_of_day, remind_at, goal, goal_history_items,
			raw_payload, source_created_at
		)
		VALUES ($1, 'todoist', $2, $3, $4, FALSE, $5, 'task', $6, $7, $8, $9, $10, $11)
		ON CONFLICT (user_id, source, external_id) DO UPDATE SET
			name = EXCLUDED.name,
			area_name = EXCLUDED.area_name,
			archived = FALSE,
			recurrence = EXCLUDED.recurrence,
			log_method = EXCLUDED.log_method,
			time_of_day = EXCLUDED.time_of_day,
			remind_at = EXCLUDED.remind_at,
			goal = EXCLUDED.goal,
			goal_history_items = EXCLUDED.goal_history_items,
			raw_payload = EXCLUDED.raw_payload,
			source_created_at = COALESCE(EXCLUDED.source_created_at, habits.source_created_at)
		RETURNING id
	`,
		userID,
		task.ID,
		strings.TrimSpace(task.Content),
		nullIfEmpty(areaName),
		nullIfEmpty(recurrence),
		timeOfDay,
		[]string{},
		nullJSON(todoistGoalJSON(task)),
		nil,
		raw,
		nullTime(sourceCreatedAt),
	).Scan(&habitID)
	if err != nil {
		return "", fmt.Errorf("upsert todoist habit row: %w", err)
	}

	return habitID, nil
}

func todoistAreaName(projectID, sectionID string, projects map[string]string, sections map[string]todoistSection) string {
	projectName, sectionName := todoistProjectSectionNames(projectID, sectionID, projects, sections)
	switch {
	case projectName != "" && sectionName != "":
		return projectName + " / " + sectionName
	case sectionName != "":
		return sectionName
	default:
		return projectName
	}
}

func todoistProjectSectionNames(projectID, sectionID string, projects map[string]string, sections map[string]todoistSection) (string, string) {
	projectName := strings.TrimSpace(projects[projectID])
	sectionName := ""
	if section, ok := sections[sectionID]; ok {
		sectionName = strings.TrimSpace(section.Name)
		if projectName == "" {
			projectName = strings.TrimSpace(projects[section.ProjectID])
		}
	}
	return projectName, sectionName
}

func todoistDueFields(due *todoistDue) (time.Time, time.Time, string, string, bool) {
	if due == nil {
		return time.Time{}, time.Time{}, "", "", false
	}

	dueString := strings.TrimSpace(due.String)
	dueTimezone := strings.TrimSpace(due.Timezone)
	isRecurring := todoistDueIsRecurring(due)

	if raw := strings.TrimSpace(due.DateTime); raw != "" {
		if ts, err := parseTodoistTime(raw); err == nil {
			return ts, time.Time{}, dueString, dueTimezone, isRecurring
		}
	}
	if raw := strings.TrimSpace(due.Date); raw != "" {
		if ts, err := parseTodoistTime(raw); err == nil {
			if strings.Contains(raw, "T") {
				return ts, time.Time{}, dueString, dueTimezone, isRecurring
			}
			dateOnly := time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, ts.Location())
			return time.Time{}, dateOnly, dueString, dueTimezone, isRecurring
		}
	}

	return time.Time{}, time.Time{}, dueString, dueTimezone, isRecurring
}

func todoistTimeOfDay(due *todoistDue) []string {
	if due == nil {
		return []string{}
	}
	raw := strings.TrimSpace(due.DateTime)
	if raw == "" && strings.Contains(due.Date, "T") {
		raw = strings.TrimSpace(due.Date)
	}
	if raw == "" {
		return []string{}
	}

	t, err := parseTodoistTime(raw)
	if err != nil {
		return []string{}
	}
	if due.Timezone != "" {
		if loc, err := time.LoadLocation(due.Timezone); err == nil {
			t = t.In(loc)
		}
	}
	return []string{t.Format("15:04")}
}

func todoistGoalJSON(task todoistItem) []byte {
	payload := map[string]any{}
	if len(task.Labels) > 0 {
		payload["labels"] = task.Labels
	}
	if task.Priority > 0 {
		payload["priority"] = task.Priority
	}
	if strings.TrimSpace(task.Description) != "" {
		payload["description"] = strings.TrimSpace(task.Description)
	}
	if len(payload) == 0 {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return data
}

func (t *TodoistConnector) archiveMissingHabits(ctx context.Context, db todoistDB, userID string, activeIDs []string) error {
	if _, err := db.Exec(ctx, `
		UPDATE habits
		SET archived = TRUE
		WHERE user_id = $1
			AND source = 'todoist'
			AND NOT (external_id = ANY($2))
	`, userID, activeIDs); err != nil {
		return err
	}
	return nil
}

func (t *TodoistConnector) markInactiveTasks(ctx context.Context, db todoistDB, userID string, activeIDs []string) error {
	_, err := db.Exec(ctx, `
		UPDATE todoist_tasks
		SET is_active = FALSE, updated_at = NOW()
		WHERE user_id = $1
			AND NOT (external_id = ANY($2))
	`, userID, activeIDs)
	return err
}

func (t *TodoistConnector) upsertCompletionEvents(ctx context.Context, db todoistDB, userID string, events []todoistCompletionEvent, projects map[string]string, sections map[string]todoistSection) error {
	for _, event := range events {
		if _, err := db.Exec(ctx, `
			INSERT INTO raw_events (source, event_type, external_id, payload, user_id)
			VALUES ('todoist', 'completed_task_event', $1, $2, $3)
		`, event.TaskID+":"+event.CompletedAt.UTC().Format(time.RFC3339), event.Raw, userID); err != nil {
			return fmt.Errorf("insert todoist raw completion event: %w", err)
		}

		projectName, sectionName := todoistProjectSectionNames(event.ProjectID, event.SectionID, projects, sections)
		if _, err := db.Exec(ctx, `
			INSERT INTO todoist_task_completions (
				user_id, task_external_id, completed_at, content,
				project_external_id, project_name, section_external_id, section_name,
				is_recurring, raw_payload
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (user_id, task_external_id, completed_at) DO UPDATE SET
				content = EXCLUDED.content,
				project_external_id = EXCLUDED.project_external_id,
				project_name = EXCLUDED.project_name,
				section_external_id = EXCLUDED.section_external_id,
				section_name = EXCLUDED.section_name,
				is_recurring = EXCLUDED.is_recurring,
				raw_payload = EXCLUDED.raw_payload
		`, userID, event.TaskID, event.CompletedAt, nullIfEmpty(event.Content),
			nullIfEmpty(event.ProjectID), nullIfEmpty(projectName), nullIfEmpty(event.SectionID), nullIfEmpty(sectionName),
			event.IsRecurring, event.Raw); err != nil {
			return err
		}

		if _, err := db.Exec(ctx, `
			UPDATE todoist_tasks
			SET last_completed_at = GREATEST(COALESCE(last_completed_at, TIMESTAMPTZ 'epoch'), $3),
				is_active = CASE WHEN is_recurring THEN TRUE ELSE is_active END,
				updated_at = NOW()
			WHERE user_id = $1 AND external_id = $2
		`, userID, event.TaskID, event.CompletedAt); err != nil {
			return err
		}
	}
	return nil
}

func (t *TodoistConnector) clearRecentStatuses(ctx context.Context, db todoistDB, userID string, since time.Time) error {
	_, err := db.Exec(ctx, `
		DELETE FROM habit_daily_statuses s
		USING habits h
		WHERE s.habit_id = h.id
			AND h.user_id = $1
			AND h.source = 'todoist'
			AND s.target_date >= $2::date
	`, userID, since.Format("2006-01-02"))
	return err
}

func (t *TodoistConnector) upsertCompletionStatus(ctx context.Context, db todoistDB, userID, habitID string, event todoistCompletionEvent) error {
	_, err := db.Exec(ctx, `
		INSERT INTO habit_daily_statuses (habit_id, target_date, status, current_value, target_value, unit_type, periodicity, raw_payload)
		VALUES ($1, $2, 'completed', 1, 1, 'task', 'daily', $3)
		ON CONFLICT (habit_id, target_date) DO UPDATE SET
			status = EXCLUDED.status,
			current_value = EXCLUDED.current_value,
			target_value = EXCLUDED.target_value,
			unit_type = EXCLUDED.unit_type,
			periodicity = EXCLUDED.periodicity,
			raw_payload = EXCLUDED.raw_payload
	`, habitID, event.CompletedAt.Format("2006-01-02"), event.Raw)
	if err != nil {
		return err
	}
	return nil
}

func (t *TodoistConnector) syncStart(lastSync time.Time, now time.Time) time.Time {
	if lastSync.IsZero() {
		start := now.AddDate(0, 0, -(todoistInitialBackfillDays - 1))
		return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	}
	start := lastSync.AddDate(0, 0, -todoistIncrementalLookback)
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
}

func (t *TodoistConnector) getLastSync(ctx context.Context, userID string) (time.Time, error) {
	var ts time.Time
	err := t.db.QueryRow(ctx, `SELECT last_synced_at FROM sync_state WHERE source = 'todoist' AND user_id = $1`, userID).Scan(&ts)
	if err != nil {
		return time.Time{}, nil
	}
	return ts, nil
}

func (t *TodoistConnector) updateLastSync(ctx context.Context, db todoistDB, userID string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO sync_state (source, last_synced_at, updated_at, enabled, user_id)
		VALUES ('todoist', NOW(), NOW(), TRUE, $1)
		ON CONFLICT (source, user_id) DO UPDATE SET
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at = EXCLUDED.updated_at,
			enabled = TRUE
	`, userID)
	return err
}

func parseTodoistTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000000",
		"2006-01-02T15:04:05.000",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported todoist time: %s", raw)
}

func nullTime(ts time.Time) any {
	if ts.IsZero() {
		return nil
	}
	return ts
}

func nullDate(ts time.Time) any {
	if ts.IsZero() {
		return nil
	}
	return ts.Format("2006-01-02")
}
