package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// VikunjaTaskDraft is a task ready to be created. Priority is on the 1-4 scale
// the task tables store, not Vikunja's 0-5 one, so callers never have to know
// which provider they are talking to.
type VikunjaTaskDraft struct {
	Title       string
	Description string
	ProjectID   int64
	DueAt       time.Time
	Priority    int
	// RepeatEverySeconds repeats the task by duration, RepeatMonthly by calendar
	// month. Vikunja treats the two as different modes, so only one is sent.
	RepeatEverySeconds int64
	RepeatMonthly      bool
	// Labels are titles, not ids: the caller names them the way a person does.
	Labels []string
	// AllowNewLabels separates a person typing a new label, which should be
	// created, from a model guessing one, which should not: an invented label
	// pollutes the workspace and nobody ever filters by it.
	AllowNewLabels bool
}

// VikunjaTaskRef is what a write reports back: enough to answer the request
// without re-reading the database, plus a link into Vikunja itself.
type VikunjaTaskRef struct {
	ExternalID  string     `json:"external_id"`
	Title       string     `json:"title"`
	ProjectID   int64      `json:"project_id"`
	ProjectName string     `json:"project_name"`
	DueAt       *time.Time `json:"due_at"`
	Done        bool       `json:"done"`
	CompletedAt *time.Time `json:"completed_at"`
	Recurrence  string     `json:"recurrence,omitempty"`
	Labels      []string   `json:"labels,omitempty"`
	// SkippedLabels are the ones that do not exist and were not allowed to be
	// created, reported so the caller can say so rather than silently drop them.
	SkippedLabels []string `json:"skipped_labels,omitempty"`
	URL           string   `json:"url"`
}

// VikunjaProjectRef names a project for the task-creation form.
type VikunjaProjectRef struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Path      string `json:"path"`
	Archived  bool   `json:"archived"`
	IsDefault bool   `json:"is_default"`
}

type vikunjaUser struct {
	Settings struct {
		DefaultProjectID int64  `json:"default_project_id"`
		Timezone         string `json:"timezone"`
	} `json:"settings"`
}

// Projects lists the projects a task can be created in, the user's own default
// first-class among them so the form can preselect it.
func (v *VikunjaConnector) Projects(ctx context.Context, userID string) ([]VikunjaProjectRef, error) {
	creds, err := v.loadCredentials(ctx, userID)
	if err != nil {
		return nil, err
	}

	projects, err := v.fetchProjects(ctx, creds)
	if err != nil {
		return nil, fmt.Errorf("fetch vikunja projects: %w", err)
	}
	defaultProjectID := v.defaultProjectID(ctx, creds, projects)

	refs := make([]VikunjaProjectRef, 0, len(projects))
	for id, project := range projects {
		refs = append(refs, VikunjaProjectRef{
			ID:        id,
			Title:     strings.TrimSpace(project.Title),
			Path:      vikunjaProjectPath(id, projects),
			Archived:  project.IsArchived,
			IsDefault: id == defaultProjectID,
		})
	}

	// Map iteration order is random; the picker needs a stable list.
	sort.Slice(refs, func(i, j int) bool { return vikunjaProjectRefLess(refs[i], refs[j]) })
	return refs, nil
}

// CreateTask adds a task to Vikunja and mirrors the result locally.
func (v *VikunjaConnector) CreateTask(ctx context.Context, userID string, draft VikunjaTaskDraft) (VikunjaTaskRef, error) {
	title := strings.TrimSpace(draft.Title)
	if title == "" {
		return VikunjaTaskRef{}, fmt.Errorf("task title is required")
	}

	creds, err := v.loadCredentials(ctx, userID)
	if err != nil {
		return VikunjaTaskRef{}, err
	}

	projects, err := v.fetchProjects(ctx, creds)
	if err != nil {
		return VikunjaTaskRef{}, fmt.Errorf("fetch vikunja projects: %w", err)
	}

	projectID := draft.ProjectID
	if projectID == 0 {
		projectID = v.defaultProjectID(ctx, creds, projects)
	}
	if projectID == 0 {
		return VikunjaTaskRef{}, fmt.Errorf("no Vikunja project to create the task in")
	}
	if _, ok := projects[projectID]; !ok {
		return VikunjaTaskRef{}, fmt.Errorf("unknown Vikunja project %d", projectID)
	}

	body, err := v.write(ctx, creds, http.MethodPut, fmt.Sprintf("/projects/%d/tasks", projectID), vikunjaCreatePayload(draft))
	if err != nil {
		return VikunjaTaskRef{}, fmt.Errorf("create vikunja task: %w", err)
	}

	var created vikunjaTask
	if err := json.Unmarshal(body, &created); err != nil {
		return VikunjaTaskRef{}, fmt.Errorf("decode created vikunja task: %w", err)
	}

	ref := v.taskRef(created, creds, projects)
	if len(draft.Labels) > 0 {
		attached, skipped, err := v.applyTaskLabels(ctx, creds, created.ID, draft.Labels, draft.AllowNewLabels)
		if err != nil {
			// The task exists by now, so a label failure is reported alongside it
			// rather than turned into a failed create.
			v.logger.Warn().Err(err).Int64("task", created.ID).Msg("attach vikunja labels")
		}
		ref.Labels = attached
		ref.SkippedLabels = skipped
	}

	v.resyncAfterWrite(ctx, userID, "create")
	return ref, nil
}

// applyTaskLabels resolves label titles to ids and puts them on the task in one
// call. Vikunja has no "create task with labels": labels are a separate
// relation, so this is a second round trip by design.
func (v *VikunjaConnector) applyTaskLabels(ctx context.Context, creds vikunjaCredentials, taskID int64, wanted []string, allowNew bool) ([]string, []string, error) {
	existing, err := v.fetchLabels(ctx, creds)
	if err != nil {
		return nil, nil, err
	}

	ids := make([]int64, 0, len(wanted))
	attached := make([]string, 0, len(wanted))
	skipped := make([]string, 0)
	seen := make(map[int64]bool, len(wanted))

	for _, title := range wanted {
		trimmed := strings.TrimSpace(title)
		if trimmed == "" {
			continue
		}

		id, ok := existing[strings.ToLower(trimmed)]
		if !ok {
			if !allowNew {
				skipped = append(skipped, trimmed)
				continue
			}
			created, err := v.createLabel(ctx, creds, trimmed)
			if err != nil {
				skipped = append(skipped, trimmed)
				continue
			}
			id = created
			existing[strings.ToLower(trimmed)] = created
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		attached = append(attached, trimmed)
	}

	if len(ids) == 0 {
		return attached, skipped, nil
	}

	labels := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		labels = append(labels, map[string]any{"id": id})
	}
	if _, err := v.write(ctx, creds, http.MethodPost, fmt.Sprintf("/tasks/%d/labels/bulk", taskID),
		map[string]any{"labels": labels}); err != nil {
		return nil, skipped, err
	}
	return attached, skipped, nil
}

// fetchLabels maps every label the account can use, keyed by lowercased title.
func (v *VikunjaConnector) fetchLabels(ctx context.Context, creds vikunjaCredentials) (map[string]int64, error) {
	labels := make(map[string]int64)

	for page := 1; page <= vikunjaMaxPages; page++ {
		query := url.Values{}
		query.Set("page", strconv.Itoa(page))
		query.Set("per_page", strconv.Itoa(vikunjaPageSize))

		body, header, err := v.get(ctx, creds, "/labels", query)
		if err != nil {
			return nil, fmt.Errorf("fetch vikunja labels: %w", err)
		}

		var batch []vikunjaLabel
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, fmt.Errorf("decode vikunja labels: %w", err)
		}
		for _, label := range batch {
			if title := strings.TrimSpace(label.Title); title != "" {
				labels[strings.ToLower(title)] = label.ID
			}
		}

		if vikunjaLastPage(header, page, len(batch)) {
			break
		}
	}
	return labels, nil
}

func (v *VikunjaConnector) createLabel(ctx context.Context, creds vikunjaCredentials, title string) (int64, error) {
	body, err := v.write(ctx, creds, http.MethodPut, "/labels", map[string]any{"title": title})
	if err != nil {
		return 0, err
	}
	var created vikunjaLabel
	if err := json.Unmarshal(body, &created); err != nil {
		return 0, err
	}
	if created.ID == 0 {
		return 0, fmt.Errorf("vikunja returned no label id for %q", title)
	}
	return created.ID, nil
}

// CompleteTask marks a task done.
//
// The update endpoint replaces the task with the payload rather than patching
// it, so the task is read first and sent back whole: posting only {done:true}
// silently cleared this task's repeat rule during development. The full object
// is round-tripped as a map so fields this connector does not model survive.
func (v *VikunjaConnector) CompleteTask(ctx context.Context, userID, externalID string) (VikunjaTaskRef, error) {
	taskID, err := strconv.ParseInt(strings.TrimSpace(externalID), 10, 64)
	if err != nil || taskID <= 0 {
		return VikunjaTaskRef{}, fmt.Errorf("invalid Vikunja task id %q", externalID)
	}

	creds, err := v.loadCredentials(ctx, userID)
	if err != nil {
		return VikunjaTaskRef{}, err
	}

	current, _, err := v.get(ctx, creds, fmt.Sprintf("/tasks/%d", taskID), nil)
	if err != nil {
		return VikunjaTaskRef{}, fmt.Errorf("read vikunja task: %w", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(current, &payload); err != nil {
		return VikunjaTaskRef{}, fmt.Errorf("decode vikunja task: %w", err)
	}
	payload["done"] = true

	body, err := v.write(ctx, creds, http.MethodPost, fmt.Sprintf("/tasks/%d", taskID), payload)
	if err != nil {
		return VikunjaTaskRef{}, fmt.Errorf("complete vikunja task: %w", err)
	}

	var updated vikunjaTask
	if err := json.Unmarshal(body, &updated); err != nil {
		return VikunjaTaskRef{}, fmt.Errorf("decode completed vikunja task: %w", err)
	}

	projects, err := v.fetchProjects(ctx, creds)
	if err != nil {
		// The write already happened; a naming lookup must not fail the request.
		v.logger.Warn().Err(err).Str("user_id", userID).Msg("name vikunja project after completion")
		projects = map[int64]vikunjaProject{}
	}

	v.resyncAfterWrite(ctx, userID, "complete")
	return v.taskRef(updated, creds, projects), nil
}

// resyncAfterWrite re-reads Vikunja so tasks, completions and habit statuses all
// land through the one code path that already knows how to derive them.
//
// The write itself is done by this point, so a failure here is logged and left
// to the scheduler rather than reported as a failed write.
func (v *VikunjaConnector) resyncAfterWrite(ctx context.Context, userID, action string) {
	if err := v.Sync(ctx, userID); err != nil {
		v.logger.Warn().Err(err).Str("user_id", userID).Str("action", action).Msg("resync after vikunja write failed")
	}
}

// defaultProjectID follows the user's own default project setting, which is
// where the Vikunja UI itself puts a task typed without picking a project. The
// lowest project id is the fallback for a token that cannot read the setting.
func (v *VikunjaConnector) defaultProjectID(ctx context.Context, creds vikunjaCredentials, projects map[int64]vikunjaProject) int64 {
	body, _, err := v.get(ctx, creds, "/user", nil)
	if err == nil {
		var user vikunjaUser
		if err := json.Unmarshal(body, &user); err == nil {
			if id := user.Settings.DefaultProjectID; id != 0 {
				if _, ok := projects[id]; ok {
					return id
				}
			}
		}
	} else {
		v.logger.Debug().Err(err).Msg("read vikunja default project setting")
	}

	var fallback int64
	for id, project := range projects {
		if project.IsArchived {
			continue
		}
		if fallback == 0 || id < fallback {
			fallback = id
		}
	}
	return fallback
}

func (v *VikunjaConnector) taskRef(task vikunjaTask, creds vikunjaCredentials, projects map[int64]vikunjaProject) VikunjaTaskRef {
	ref := VikunjaTaskRef{
		ExternalID:  strconv.FormatInt(task.ID, 10),
		Title:       strings.TrimSpace(task.Title),
		ProjectID:   task.ProjectID,
		ProjectName: vikunjaProjectPath(task.ProjectID, projects),
		Done:        task.Done,
		Recurrence:  vikunjaRecurrence(task),
		URL:         fmt.Sprintf("%s/tasks/%d", creds.webURL, task.ID),
	}
	if due := parseVikunjaTime(task.DueDate); !due.IsZero() {
		ref.DueAt = &due
	}
	if completed := parseVikunjaTime(task.DoneAt); !completed.IsZero() {
		ref.CompletedAt = &completed
	}
	return ref
}

func (v *VikunjaConnector) write(ctx context.Context, creds vikunjaCredentials, method, path string, payload any) ([]byte, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, creds.apiURL+path, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+creds.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("vikunja api %s %s returned %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// vikunjaCreatePayload builds the create body. Only fields the caller actually
// set are sent: Vikunja stores what it is given, so an empty description or a
// zero due date would overwrite nothing but still read as a deliberate value.
func vikunjaCreatePayload(draft VikunjaTaskDraft) map[string]any {
	payload := map[string]any{"title": strings.TrimSpace(draft.Title)}
	if description := strings.TrimSpace(draft.Description); description != "" {
		payload["description"] = description
	}
	if !draft.DueAt.IsZero() {
		payload["due_date"] = draft.DueAt.UTC().Format(time.RFC3339)
	}
	if priority := vikunjaAPIPriority(draft.Priority); priority > 0 {
		payload["priority"] = priority
	}
	// The two repeat kinds are exclusive: repeat_mode 1 makes Vikunja ignore
	// repeat_after, so sending both would silently drop the interval.
	switch {
	case draft.RepeatMonthly:
		payload["repeat_mode"] = vikunjaRepeatModeMonthly
	case draft.RepeatEverySeconds > 0:
		payload["repeat_after"] = draft.RepeatEverySeconds
	}
	return payload
}

// vikunjaAPIPriority converts a stored 1-4 priority back to what Vikunja
// accepts. Anything outside the scale is dropped rather than guessed.
func vikunjaAPIPriority(priority int) int {
	switch {
	case priority <= 0:
		return 0
	case priority >= 4:
		return 4
	default:
		return priority
	}
}

// vikunjaProjectRefLess puts the default project first, then live projects
// before archived ones, then alphabetically by full path.
func vikunjaProjectRefLess(a, b VikunjaProjectRef) bool {
	if a.IsDefault != b.IsDefault {
		return a.IsDefault
	}
	if a.Archived != b.Archived {
		return b.Archived
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	return a.ID < b.ID
}
