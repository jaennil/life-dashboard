package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	vikunjaRequestTimeout      = 30 * time.Second
	vikunjaInitialBackfillDays = 60
	vikunjaIncrementalLookback = 14
	// The instance advertises its own max_items_per_page (50 on the homelab
	// deployment) and silently clamps anything larger, so pages are followed
	// until the pagination header says the last one was read.
	vikunjaPageSize         = 50
	vikunjaMaxPages         = 100
	vikunjaTotalPagesHeader = "x-pagination-total-pages"
	// Every unset date arrives as the zero value of Go's time.Time rather than
	// null, so "no due date" and "never completed" look like year-1 timestamps.
	vikunjaUnsetTimePrefix = "0001-01-01"
)

type vikunjaCredentials struct {
	apiURL string
	token  string
}

type vikunjaProject struct {
	ID              int64  `json:"id"`
	Title           string `json:"title"`
	ParentProjectID int64  `json:"parent_project_id"`
	IsArchived      bool   `json:"is_archived"`
}

type vikunjaLabel struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type vikunjaTask struct {
	ID          int64          `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Done        bool           `json:"done"`
	DoneAt      string         `json:"done_at"`
	DueDate     string         `json:"due_date"`
	ProjectID   int64          `json:"project_id"`
	Priority    int            `json:"priority"`
	RepeatAfter int64          `json:"repeat_after"`
	RepeatMode  int            `json:"repeat_mode"`
	Labels      []vikunjaLabel `json:"labels"`
	Identifier  string         `json:"identifier"`
	Created     string         `json:"created"`
	Updated     string         `json:"updated"`
}

type vikunjaCompletionEvent struct {
	TaskID      string
	CompletedAt time.Time
	ProjectID   string
	ProjectName string
	Content     string
	IsRecurring bool
	Raw         json.RawMessage
}

type VikunjaConnector struct {
	db     *pgxpool.Pool
	client *http.Client
	logger zerolog.Logger
}

func NewVikunja(db *pgxpool.Pool, logger zerolog.Logger) *VikunjaConnector {
	return &VikunjaConnector{
		db:     db,
		client: &http.Client{Timeout: vikunjaRequestTimeout},
		logger: logger.With().Str("connector", "vikunja").Logger(),
	}
}

func (v *VikunjaConnector) Name() string { return "vikunja" }

// Sync mirrors active tasks and recent completions into the shared task tables.
//
// Unlike Todoist there is no completion archive to page through: a Vikunja task
// carries only its most recent done_at. Completion history is therefore built up
// locally, one sync at a time, which is also why the incremental window overlaps
// the previous run rather than starting where it ended.
func (v *VikunjaConnector) Sync(ctx context.Context, userID string) error {
	creds, err := v.loadCredentials(ctx, userID)
	if err != nil {
		return err
	}

	projects, err := v.fetchProjects(ctx, creds)
	if err != nil {
		return fmt.Errorf("fetch vikunja projects: %w", err)
	}

	lastSync, err := v.getLastSync(ctx, userID)
	if err != nil {
		return fmt.Errorf("get last sync: %w", err)
	}
	lookbackStart := v.syncStart(lastSync, time.Now())

	tasks, err := v.fetchTasks(ctx, creds, lookbackStart)
	if err != nil {
		return fmt.Errorf("fetch vikunja tasks: %w", err)
	}

	tx, err := v.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	activeTaskIDs := make([]string, 0, len(tasks))
	recurringTaskIDs := make([]string, 0, len(tasks))
	habitIDs := make(map[string]string, len(tasks))
	completions := make([]vikunjaCompletionEvent, 0, len(tasks))

	for _, task := range tasks {
		raw, err := json.Marshal(task)
		if err != nil {
			return fmt.Errorf("marshal vikunja task: %w", err)
		}

		externalID := strconv.FormatInt(task.ID, 10)
		if err := v.upsertTask(ctx, tx, userID, task, raw, projects); err != nil {
			return fmt.Errorf("upsert vikunja task: %w", err)
		}
		if !task.Done {
			activeTaskIDs = append(activeTaskIDs, externalID)
		}

		if vikunjaIsRecurring(task) && !task.Done {
			habitID, err := v.upsertTaskAsHabit(ctx, tx, userID, task, raw, projects)
			if err != nil {
				return fmt.Errorf("upsert vikunja habit: %w", err)
			}
			habitIDs[externalID] = habitID
			recurringTaskIDs = append(recurringTaskIDs, externalID)
		}

		// A repeating task is un-done by the server the moment it is completed:
		// done flips back to false and the due date moves on, but done_at keeps
		// the completion instant. Reading completions off done_at rather than
		// done is what makes recurring work visible at all.
		completedAt := parseVikunjaTime(task.DoneAt)
		if completedAt.IsZero() || completedAt.Before(lookbackStart) {
			continue
		}
		completions = append(completions, vikunjaCompletionEvent{
			TaskID:      externalID,
			CompletedAt: completedAt,
			ProjectID:   strconv.FormatInt(task.ProjectID, 10),
			ProjectName: vikunjaProjectPath(task.ProjectID, projects),
			Content:     strings.TrimSpace(task.Title),
			IsRecurring: vikunjaIsRecurring(task),
			Raw:         raw,
		})
	}

	if err := v.markInactiveTasks(ctx, tx, userID, activeTaskIDs); err != nil {
		return fmt.Errorf("mark inactive vikunja tasks: %w", err)
	}

	if err := v.archiveMissingHabits(ctx, tx, userID, recurringTaskIDs); err != nil {
		return fmt.Errorf("archive missing vikunja habits: %w", err)
	}

	if err := v.upsertCompletionEvents(ctx, tx, userID, completions); err != nil {
		return fmt.Errorf("upsert vikunja completion events: %w", err)
	}

	if err := v.clearRecentStatuses(ctx, tx, userID, lookbackStart); err != nil {
		return fmt.Errorf("clear recent vikunja statuses: %w", err)
	}
	for _, event := range completions {
		habitID, ok := habitIDs[event.TaskID]
		if !ok {
			continue
		}
		if err := v.upsertCompletionStatus(ctx, tx, habitID, event); err != nil {
			return fmt.Errorf("upsert vikunja completion status: %w", err)
		}
	}

	if err := v.updateLastSync(ctx, tx, userID); err != nil {
		return fmt.Errorf("update vikunja last sync: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	v.logger.Info().
		Int("tasks", len(tasks)).
		Int("active", len(activeTaskIDs)).
		Int("recurring", len(recurringTaskIDs)).
		Int("completions", len(completions)).
		Msg("vikunja sync complete")
	return nil
}

// loadCredentials reads the API token and the instance URL.
//
// Vikunja is self-hosted, so unlike every hosted connector the base URL is part
// of the credentials rather than a constant, and it is stored per user in the
// refresh_token column the same way Notion keeps its database id there.
func (v *VikunjaConnector) loadCredentials(ctx context.Context, userID string) (vikunjaCredentials, error) {
	var token, baseURL string
	err := v.db.QueryRow(ctx, `
		SELECT access_token, refresh_token FROM oauth_tokens WHERE source = 'vikunja' AND user_id = $1
	`, userID).Scan(&token, &baseURL)
	if err != nil || strings.TrimSpace(token) == "" {
		return vikunjaCredentials{}, fmt.Errorf("no Vikunja credentials — add your instance URL and API token in Settings")
	}

	apiURL, err := vikunjaAPIURL(baseURL)
	if err != nil {
		return vikunjaCredentials{}, err
	}
	return vikunjaCredentials{apiURL: apiURL, token: strings.TrimSpace(token)}, nil
}

// vikunjaAPIURL turns whatever was pasted into Settings into an API base.
//
// The address people copy is the one from their browser, which may or may not
// carry a scheme, a trailing slash or the /api/v1 prefix already.
func vikunjaAPIURL(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", fmt.Errorf("no Vikunja instance URL — add it in Settings")
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("invalid Vikunja instance URL %q", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported Vikunja instance scheme %q", parsed.Scheme)
	}

	path := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(path, "/api/v1") {
		path += "/api/v1"
	}
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (v *VikunjaConnector) fetchProjects(ctx context.Context, creds vikunjaCredentials) (map[int64]vikunjaProject, error) {
	projects := make(map[int64]vikunjaProject)

	for page := 1; page <= vikunjaMaxPages; page++ {
		query := url.Values{}
		query.Set("page", strconv.Itoa(page))
		query.Set("per_page", strconv.Itoa(vikunjaPageSize))
		// Archived projects still name the tasks synced before they were closed.
		query.Set("is_archived", "true")

		body, header, err := v.get(ctx, creds, "/projects", query)
		if err != nil {
			return nil, err
		}

		var batch []vikunjaProject
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, fmt.Errorf("decode vikunja projects: %w", err)
		}
		for _, project := range batch {
			projects[project.ID] = project
		}

		if vikunjaLastPage(header, page, len(batch)) {
			break
		}
	}

	return projects, nil
}

// fetchTasks reads everything still open plus everything completed inside the
// window, in one filtered query rather than a per-project walk.
func (v *VikunjaConnector) fetchTasks(ctx context.Context, creds vikunjaCredentials, since time.Time) ([]vikunjaTask, error) {
	tasks := make([]vikunjaTask, 0, vikunjaPageSize)

	for page := 1; page <= vikunjaMaxPages; page++ {
		query := url.Values{}
		query.Set("page", strconv.Itoa(page))
		query.Set("per_page", strconv.Itoa(vikunjaPageSize))
		query.Set("filter", vikunjaTaskFilter(since))
		query.Set("sort_by", "id")

		body, header, err := v.get(ctx, creds, "/tasks", query)
		if err != nil {
			return nil, err
		}

		var batch []vikunjaTask
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, fmt.Errorf("decode vikunja tasks: %w", err)
		}
		tasks = append(tasks, batch...)

		if vikunjaLastPage(header, page, len(batch)) {
			break
		}
	}

	return tasks, nil
}

// vikunjaTaskFilter asks for open tasks and recent completions at once. Unset
// done_at values sort as year 1, so the comparison excludes them by itself.
func vikunjaTaskFilter(since time.Time) string {
	return fmt.Sprintf("done = false || done_at > '%s'", since.UTC().Format(time.RFC3339))
}

// vikunjaLastPage decides whether to stop paging. The total-pages header is
// authoritative when present; a short page is the fallback for a proxy that
// strips it, since Vikunja never exposes a total count in the body.
func vikunjaLastPage(header http.Header, page, batchSize int) bool {
	if raw := strings.TrimSpace(header.Get(vikunjaTotalPagesHeader)); raw != "" {
		if totalPages, err := strconv.Atoi(raw); err == nil {
			return page >= totalPages
		}
	}
	return batchSize < vikunjaPageSize
}

func (v *VikunjaConnector) get(ctx context.Context, creds vikunjaCredentials, path string, query url.Values) ([]byte, http.Header, error) {
	endpoint := creds.apiURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+creds.token)

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("vikunja api %s returned %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, resp.Header, nil
}

func (v *VikunjaConnector) upsertTask(ctx context.Context, db connectorDB, userID string, task vikunjaTask, raw []byte, projects map[int64]vikunjaProject) error {
	externalID := strconv.FormatInt(task.ID, 10)
	if _, err := db.Exec(ctx, `
		INSERT INTO raw_events (source, event_type, external_id, payload, user_id)
		VALUES ('vikunja', 'task', $1, $2, $3)
	`, externalID, raw, userID); err != nil {
		return fmt.Errorf("insert vikunja raw task event: %w", err)
	}

	_, err := db.Exec(ctx, `
		INSERT INTO tasks (
			user_id, source, external_id, project_external_id, project_name,
			content, description, labels, priority,
			is_recurring, is_active, added_at, due_at, due_string,
			raw_payload, updated_at
		)
		VALUES ($1, 'vikunja', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW())
		ON CONFLICT (user_id, source, external_id) DO UPDATE SET
			project_external_id = EXCLUDED.project_external_id,
			project_name = EXCLUDED.project_name,
			content = EXCLUDED.content,
			description = EXCLUDED.description,
			labels = EXCLUDED.labels,
			priority = EXCLUDED.priority,
			is_recurring = EXCLUDED.is_recurring,
			is_active = EXCLUDED.is_active,
			added_at = COALESCE(EXCLUDED.added_at, tasks.added_at),
			due_at = EXCLUDED.due_at,
			due_string = EXCLUDED.due_string,
			raw_payload = EXCLUDED.raw_payload,
			updated_at = NOW()
	`, userID,
		externalID,
		strconv.FormatInt(task.ProjectID, 10),
		nullIfEmpty(vikunjaProjectPath(task.ProjectID, projects)),
		strings.TrimSpace(task.Title),
		nullIfEmpty(strings.TrimSpace(task.Description)),
		vikunjaLabelTitles(task.Labels),
		vikunjaPriority(task.Priority),
		vikunjaIsRecurring(task),
		!task.Done,
		nullTime(parseVikunjaTime(task.Created)),
		nullTime(parseVikunjaTime(task.DueDate)),
		nullIfEmpty(vikunjaRecurrence(task)),
		raw,
	)
	if err != nil {
		return fmt.Errorf("upsert vikunja task row: %w", err)
	}
	return nil
}

func (v *VikunjaConnector) upsertTaskAsHabit(ctx context.Context, db connectorDB, userID string, task vikunjaTask, raw []byte, projects map[int64]vikunjaProject) (string, error) {
	var habitID string
	err := db.QueryRow(ctx, `
		INSERT INTO habits (
			user_id, source, external_id, name, area_name, archived,
			recurrence, log_method, time_of_day, remind_at, goal, goal_history_items,
			raw_payload, source_created_at
		)
		VALUES ($1, 'vikunja', $2, $3, $4, FALSE, $5, 'task', $6, $7, $8, $9, $10, $11)
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
		strconv.FormatInt(task.ID, 10),
		strings.TrimSpace(task.Title),
		nullIfEmpty(vikunjaProjectPath(task.ProjectID, projects)),
		nullIfEmpty(vikunjaRecurrence(task)),
		vikunjaTimeOfDay(task),
		[]string{},
		nullJSON(vikunjaGoalJSON(task)),
		nil,
		raw,
		nullTime(parseVikunjaTime(task.Created)),
	).Scan(&habitID)
	if err != nil {
		return "", fmt.Errorf("upsert vikunja habit row: %w", err)
	}
	return habitID, nil
}

func (v *VikunjaConnector) markInactiveTasks(ctx context.Context, db connectorDB, userID string, activeIDs []string) error {
	_, err := db.Exec(ctx, `
		UPDATE tasks
		SET is_active = FALSE, updated_at = NOW()
		WHERE user_id = $1
			AND source = 'vikunja'
			AND NOT (external_id = ANY($2))
	`, userID, activeIDs)
	return err
}

func (v *VikunjaConnector) archiveMissingHabits(ctx context.Context, db connectorDB, userID string, activeIDs []string) error {
	_, err := db.Exec(ctx, `
		UPDATE habits
		SET archived = TRUE
		WHERE user_id = $1
			AND source = 'vikunja'
			AND NOT (external_id = ANY($2))
	`, userID, activeIDs)
	return err
}

func (v *VikunjaConnector) upsertCompletionEvents(ctx context.Context, db connectorDB, userID string, events []vikunjaCompletionEvent) error {
	for _, event := range events {
		if _, err := db.Exec(ctx, `
			INSERT INTO raw_events (source, event_type, external_id, payload, user_id)
			VALUES ('vikunja', 'completed_task_event', $1, $2, $3)
		`, event.TaskID+":"+event.CompletedAt.UTC().Format(time.RFC3339), event.Raw, userID); err != nil {
			return fmt.Errorf("insert vikunja raw completion event: %w", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO task_completions (
				user_id, source, task_external_id, completed_at, content,
				project_external_id, project_name, is_recurring, raw_payload
			)
			VALUES ($1, 'vikunja', $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (user_id, source, task_external_id, completed_at) DO UPDATE SET
				content = EXCLUDED.content,
				project_external_id = EXCLUDED.project_external_id,
				project_name = EXCLUDED.project_name,
				is_recurring = EXCLUDED.is_recurring,
				raw_payload = EXCLUDED.raw_payload
		`, userID, event.TaskID, event.CompletedAt, nullIfEmpty(event.Content),
			nullIfEmpty(event.ProjectID), nullIfEmpty(event.ProjectName),
			event.IsRecurring, event.Raw); err != nil {
			return err
		}

		if _, err := db.Exec(ctx, `
			UPDATE tasks
			SET last_completed_at = GREATEST(COALESCE(last_completed_at, TIMESTAMPTZ 'epoch'), $3),
				updated_at = NOW()
			WHERE user_id = $1 AND source = 'vikunja' AND external_id = $2
		`, userID, event.TaskID, event.CompletedAt); err != nil {
			return err
		}
	}
	return nil
}

// clearRecentStatuses drops the habit statuses inside the window before they are
// rewritten, so a task completed by mistake and reopened in Vikunja stops
// counting as done here too.
func (v *VikunjaConnector) clearRecentStatuses(ctx context.Context, db connectorDB, userID string, since time.Time) error {
	_, err := db.Exec(ctx, `
		DELETE FROM habit_daily_statuses s
		USING habits h
		WHERE s.habit_id = h.id
			AND h.user_id = $1
			AND h.source = 'vikunja'
			AND s.target_date >= $2::date
	`, userID, since.Format("2006-01-02"))
	return err
}

func (v *VikunjaConnector) upsertCompletionStatus(ctx context.Context, db connectorDB, habitID string, event vikunjaCompletionEvent) error {
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
	return err
}

func (v *VikunjaConnector) syncStart(lastSync time.Time, now time.Time) time.Time {
	if lastSync.IsZero() {
		start := now.AddDate(0, 0, -(vikunjaInitialBackfillDays - 1))
		return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	}
	start := lastSync.AddDate(0, 0, -vikunjaIncrementalLookback)
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
}

func (v *VikunjaConnector) getLastSync(ctx context.Context, userID string) (time.Time, error) {
	var ts time.Time
	err := v.db.QueryRow(ctx, `SELECT last_synced_at FROM sync_state WHERE source = 'vikunja' AND user_id = $1`, userID).Scan(&ts)
	if err != nil {
		return time.Time{}, nil
	}
	return ts, nil
}

func (v *VikunjaConnector) updateLastSync(ctx context.Context, db connectorDB, userID string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO sync_state (source, last_synced_at, updated_at, enabled, user_id)
		VALUES ('vikunja', NOW(), NOW(), TRUE, $1)
		ON CONFLICT (source, user_id) DO UPDATE SET
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at = EXCLUDED.updated_at,
			enabled = TRUE
	`, userID)
	return err
}

// vikunjaProjectPath names a task's project the way the app shows it, walking up
// the parent chain so a nested project reads "Дом / Ремонт" instead of "Ремонт".
func vikunjaProjectPath(projectID int64, projects map[int64]vikunjaProject) string {
	segments := make([]string, 0, 4)
	seen := make(map[int64]bool, 4)

	for id := projectID; id != 0; {
		project, ok := projects[id]
		if !ok || seen[id] {
			break
		}
		seen[id] = true

		if title := strings.TrimSpace(project.Title); title != "" {
			segments = append([]string{title}, segments...)
		}
		id = project.ParentProjectID
	}

	return strings.Join(segments, " / ")
}

func vikunjaLabelTitles(labels []vikunjaLabel) []string {
	titles := make([]string, 0, len(labels))
	for _, label := range labels {
		if title := strings.TrimSpace(label.Title); title != "" {
			titles = append(titles, title)
		}
	}
	return titles
}

// vikunjaPriority maps Vikunja's 0-5 scale onto the 1-4 one the task tables
// already store for Todoist, so productivity queries can compare them: unset
// stays unset and "DO NOW" collapses onto urgent.
func vikunjaPriority(priority int) any {
	switch {
	case priority <= 0:
		return nil
	case priority >= 4:
		return 4
	default:
		return priority
	}
}

func vikunjaIsRecurring(task vikunjaTask) bool {
	return task.RepeatAfter > 0 || task.RepeatMode != 0
}

// vikunjaRecurrence describes the repeat rule in words, the way Todoist's own
// due_string already does, because Vikunja only exposes it as a duration.
func vikunjaRecurrence(task vikunjaTask) string {
	if !vikunjaIsRecurring(task) {
		return ""
	}

	// repeat_mode 1 repeats monthly and ignores repeat_after entirely.
	if task.RepeatMode == 1 {
		return "every month"
	}

	rule := vikunjaRepeatInterval(task.RepeatAfter)
	if rule == "" {
		return "repeating"
	}
	// repeat_mode 2 counts from the completion instead of the old due date.
	if task.RepeatMode == 2 {
		rule += " from completion"
	}
	return rule
}

func vikunjaRepeatInterval(seconds int64) string {
	if seconds <= 0 {
		return ""
	}

	units := []struct {
		seconds int64
		name    string
	}{
		{7 * 24 * 3600, "week"},
		{24 * 3600, "day"},
		{3600, "hour"},
		{60, "minute"},
	}

	for _, unit := range units {
		if seconds%unit.seconds != 0 {
			continue
		}
		count := seconds / unit.seconds
		if count == 1 {
			return "every " + unit.name
		}
		return fmt.Sprintf("every %d %ss", count, unit.name)
	}
	return fmt.Sprintf("every %d seconds", seconds)
}

// vikunjaTimeOfDay reports the due time in the offset the instance sent, which
// is the user's own timezone: Vikunja renders due dates in the configured
// service timezone rather than UTC.
func vikunjaTimeOfDay(task vikunjaTask) []string {
	due := parseVikunjaTime(task.DueDate)
	if due.IsZero() {
		return []string{}
	}
	return []string{due.Format("15:04")}
}

func vikunjaGoalJSON(task vikunjaTask) []byte {
	payload := map[string]any{}
	if labels := vikunjaLabelTitles(task.Labels); len(labels) > 0 {
		payload["labels"] = labels
	}
	if priority := vikunjaPriority(task.Priority); priority != nil {
		payload["priority"] = priority
	}
	if description := strings.TrimSpace(task.Description); description != "" {
		payload["description"] = description
	}
	if identifier := strings.TrimSpace(task.Identifier); identifier != "" {
		payload["identifier"] = identifier
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

// parseVikunjaTime returns the zero time for anything Vikunja means as "unset".
func parseVikunjaTime(raw string) time.Time {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, vikunjaUnsetTimePrefix) {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339Nano, trimmed)
	if err != nil {
		return time.Time{}
	}
	return ts
}
