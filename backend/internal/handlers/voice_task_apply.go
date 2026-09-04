package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"life-dashboard/internal/connectors"
)

type taskWriter interface {
	CreateTask(ctx context.Context, userID string, draft connectors.VikunjaTaskDraft) (connectors.VikunjaTaskRef, error)
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
		Title:    title,
		Priority: interpreted.Task.Priority,
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
	return strings.Join(parts, " · ")
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
