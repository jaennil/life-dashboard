package handlers

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"life-dashboard/internal/connectors"
)

type taskWriter interface {
	CreateTask(ctx context.Context, userID string, draft connectors.VikunjaTaskDraft) (connectors.VikunjaTaskRef, error)
}

// taskProject is one project the dictated task can be filed into, mirrored
// locally by the sync.
type taskProject struct {
	ExternalID string
	Name       string
	Path       string
	IsDefault  bool
}

func (h *VoiceWorkoutHandler) loadTaskProjects(ctx context.Context, userID string) ([]taskProject, error) {
	rows, err := h.db.Query(ctx, `
		SELECT external_id, name, path, is_default
		FROM task_projects
		WHERE user_id = $1 AND source = 'vikunja' AND archived = FALSE
		ORDER BY is_default DESC, path
		LIMIT 100
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	projects := make([]taskProject, 0, 16)
	for rows.Next() {
		var project taskProject
		if err := rows.Scan(&project.ExternalID, &project.Name, &project.Path, &project.IsDefault); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

// resolveTaskProject maps what the model answered onto a real project.
//
// The prompt asks for a verbatim copy, but a model that paraphrases must not
// silently file the task somewhere else: only an unambiguous match counts, and
// anything else falls back to the default project with a note to the user.
func resolveTaskProject(answer string, projects []taskProject) (taskProject, bool) {
	needle := strings.ToLower(strings.TrimSpace(answer))
	if needle == "" {
		return taskProject{}, false
	}

	for _, project := range projects {
		if strings.ToLower(project.Path) == needle || strings.ToLower(project.Name) == needle {
			return project, true
		}
	}

	var match taskProject
	found := 0
	for _, project := range projects {
		if strings.Contains(strings.ToLower(project.Path), needle) {
			match = project
			found++
		}
	}
	if found == 1 {
		return match, true
	}
	return taskProject{}, false
}

// taskRepeatSeconds turns a spoken interval into what Vikunja stores. Months are
// the exception: the provider repeats them by calendar rather than by duration,
// which is a separate mode and only exists for a single month.
func taskRepeatSeconds(repeat *voiceParsedRepeat) (seconds int64, monthly bool, ok bool) {
	if repeat == nil {
		return 0, false, false
	}
	every := repeat.Every
	if every <= 0 {
		every = 1
	}

	switch strings.ToLower(strings.TrimSpace(repeat.Unit)) {
	case "day", "days", "день", "дня", "дней":
		return int64(every) * 24 * 3600, false, true
	case "week", "weeks", "неделя", "недели", "недель":
		return int64(every) * 7 * 24 * 3600, false, true
	case "month", "months", "месяц", "месяца", "месяцев":
		if every == 1 {
			return 0, true, true
		}
		return 0, false, false
	default:
		return 0, false, false
	}
}

// applyTask writes a dictated task straight to Vikunja.
//
// Like food there is no confirmation step: a task said out loud is meant to
// leave the head immediately, and a wrong one costs a single click to close.
// What it will not do is invent the part that matters - a phrase with no
// recognizable action is reported back instead of filed as an empty task.
func (h *VoiceWorkoutHandler) applyTask(ctx context.Context, userID, eventID string, interpreted voiceInterpretation, response *voiceWorkoutResponse) {
	response.Unmatched = interpreted.Unmatched

	title := ""
	if interpreted.Task != nil {
		title = strings.TrimSpace(interpreted.Task.Title)
	}
	if title == "" {
		response.Message = "Похоже на задачу, но я не понял, что именно сделать."
		return
	}

	if h.task == nil {
		response.Message = "Похоже на задачу, но запись в Vikunja не настроена."
		return
	}

	draft := connectors.VikunjaTaskDraft{
		Title:       title,
		Description: strings.TrimSpace(interpreted.Task.Description),
		Priority:    interpreted.Task.Priority,
	}

	if named := strings.TrimSpace(interpreted.Task.Project); named != "" {
		projects, err := h.loadTaskProjects(ctx, userID)
		if err != nil {
			h.logger.Warn().Err(err).Str("user_id", userID).Msg("load task projects")
		}
		if project, ok := resolveTaskProject(named, projects); ok {
			if id, err := strconv.ParseInt(project.ExternalID, 10, 64); err == nil {
				draft.ProjectID = id
			}
		} else {
			// Filing into the wrong project is worse than filing into the inbox,
			// so an unrecognized name is reported rather than guessed at.
			response.Unmatched = append(response.Unmatched, "проект \""+named+"\" не нашёл")
		}
	}

	if seconds, monthly, ok := taskRepeatSeconds(interpreted.Task.Repeat); ok {
		draft.RepeatEverySeconds = seconds
		draft.RepeatMonthly = monthly
	} else if interpreted.Task.Repeat != nil {
		response.Unmatched = append(response.Unmatched, "повтор не разобрал")
	}

	if raw := strings.TrimSpace(interpreted.Task.DueAt); raw != "" {
		if dueAt, err := time.Parse(time.RFC3339, raw); err == nil {
			draft.DueAt = dueAt
		} else {
			// The deadline is the part most likely to come back malformed, and it
			// is the part the task can live without. Say so instead of dropping
			// the whole task.
			h.logger.Warn().Str("due_at", raw).Msg("unparsable dictated task deadline")
			response.Unmatched = append(response.Unmatched, "срок \""+raw+"\" не разобрал")
		}
	}

	created, err := h.task.CreateTask(ctx, userID, draft)
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", userID).Msg("create dictated task")
		response.Message = "Похоже на задачу, но записать в Vikunja не удалось: " + err.Error()
		return
	}

	// The created id is kept on the archived phrase: it is the audit trail, and
	// what a later "отмени последнее" would need to undo the write.
	if err := h.recordTaskID(ctx, eventID, created.ExternalID); err != nil {
		h.logger.Warn().Err(err).Str("event_id", eventID).Msg("record created task id")
	}

	response.Task = summarizeCreatedTask(created)
	response.Message = "Записал задачу в Vikunja."
}

// summarizeCreatedTask is what the phone shows back: the task as filed, so a
// misheard word or a deadline landing on the wrong day is caught at once.
func summarizeCreatedTask(task connectors.VikunjaTaskRef) string {
	parts := []string{task.Title}
	if task.ProjectName != "" {
		parts = append(parts, task.ProjectName)
	}
	if task.DueAt != nil {
		parts = append(parts, "до "+task.DueAt.Local().Format("02.01 15:04"))
	}
	if task.Recurrence != "" {
		parts = append(parts, taskRecurrenceRu(task.Recurrence))
	}
	return strings.Join(parts, " · ")
}

// taskRecurrenceRu translates the provider's repeat rule for the phone. The
// stored form stays English because that is what the habit tables already
// carry; only the line someone reads out of a notification is localized.
func taskRecurrenceRu(recurrence string) string {
	rule := strings.TrimSpace(recurrence)
	fromCompletion := strings.HasSuffix(rule, " from completion")
	rule = strings.TrimSuffix(rule, " from completion")

	// Singular for "every <unit>", then the two plural forms Russian needs: one
	// for 2-4 and one for 5 and up.
	units := map[string][3]string{
		"minute": {"минуту", "минуты", "минут"},
		"hour":   {"час", "часа", "часов"},
		"day":    {"день", "дня", "дней"},
		"week":   {"неделю", "недели", "недель"},
		"month":  {"месяц", "месяца", "месяцев"},
	}
	feminine := map[string]bool{"minute": true, "week": true}

	translated := ""
	fields := strings.Fields(rule)
	switch {
	case len(fields) == 2 && fields[0] == "every":
		if forms, ok := units[fields[1]]; ok {
			prefix := "каждый"
			if feminine[fields[1]] {
				prefix = "каждую"
			}
			translated = prefix + " " + forms[0]
		}
	case len(fields) == 3 && fields[0] == "every":
		unit := strings.TrimSuffix(fields[2], "s")
		count, err := strconv.Atoi(fields[1])
		if forms, ok := units[unit]; ok && err == nil {
			translated = "каждые " + fields[1] + " " + russianPlural(count, forms)
		}
	}
	if translated == "" {
		translated = rule
	}
	if fromCompletion {
		translated += " от выполнения"
	}
	return translated
}

// russianPlural picks the form for a count: 1, 2-4, or 5 and up, with the teens
// taking the last form regardless of their final digit.
func russianPlural(count int, forms [3]string) string {
	if count < 0 {
		count = -count
	}
	switch {
	case count%100 >= 11 && count%100 <= 19:
		return forms[2]
	case count%10 == 1:
		return forms[0]
	case count%10 >= 2 && count%10 <= 4:
		return forms[1]
	default:
		return forms[2]
	}
}

func (h *VoiceWorkoutHandler) recordTaskID(ctx context.Context, eventID, taskID string) error {
	if eventID == "" || taskID == "" {
		return nil
	}
	encoded, err := json.Marshal(taskID)
	if err != nil {
		return err
	}
	_, err = h.db.Exec(ctx, `
		UPDATE raw_events SET payload = jsonb_set(payload, '{vikunja_task_id}', $2::jsonb)
		WHERE id = $1
	`, eventID, encoded)
	return err
}
