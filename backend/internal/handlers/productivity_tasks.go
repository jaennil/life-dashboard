package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"life-dashboard/internal/connectors"
	authmw "life-dashboard/internal/middleware"
)

// taskSourceVikunja is the only task provider the dashboard can write to.
// Todoist arrives through a read-only scope, so its rows stay read-only here.
const taskSourceVikunja = "vikunja"

type ProductivityTasksHandler struct {
	db      *pgxpool.Pool
	vikunja *connectors.VikunjaConnector
	logger  zerolog.Logger
}

func NewProductivityTasks(db *pgxpool.Pool, vikunja *connectors.VikunjaConnector, logger zerolog.Logger) *ProductivityTasksHandler {
	return &ProductivityTasksHandler{
		db:      db,
		vikunja: vikunja,
		logger:  logger.With().Str("handler", "productivity_tasks").Logger(),
	}
}

type createTaskRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	ProjectID   int64    `json:"project_id"`
	DueAt       string   `json:"due_at"`
	Priority    int      `json:"priority"`
	Labels      []string `json:"labels"`
	// RepeatEvery counts RepeatUnit periods: 2 + "week" is every two weeks.
	RepeatEvery int    `json:"repeat_every"`
	RepeatUnit  string `json:"repeat_unit"`
}

// CreateTask adds a task to Vikunja.
func (h *ProductivityTasksHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)

	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}

	draft := connectors.VikunjaTaskDraft{
		Title:       req.Title,
		Description: req.Description,
		ProjectID:   req.ProjectID,
		Priority:    req.Priority,
		Labels:      req.Labels,
		// Typed by hand, so a label that does not exist yet is a new label
		// rather than a mistake.
		AllowNewLabels: true,
	}
	if strings.TrimSpace(req.RepeatUnit) != "" {
		seconds, monthly, ok := taskRepeatSeconds(&voiceParsedRepeat{Every: req.RepeatEvery, Unit: req.RepeatUnit})
		if !ok {
			http.Error(w, "unsupported repeat_unit", http.StatusBadRequest)
			return
		}
		draft.RepeatEverySeconds = seconds
		draft.RepeatMonthly = monthly
	}
	if raw := strings.TrimSpace(req.DueAt); raw != "" {
		dueAt, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "due_at must be an RFC3339 timestamp", http.StatusBadRequest)
			return
		}
		draft.DueAt = dueAt
	}

	task, err := h.vikunja.CreateTask(ctx, userID, draft)
	if err != nil {
		h.writeConnectorError(w, err, "create vikunja task", userID)
		return
	}

	h.logger.Info().Str("user_id", userID).Str("task", task.ExternalID).Msg("vikunja task created")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

// CompleteTask closes a task in the provider it came from.
//
// The path id is the local row id rather than the provider's, both because that
// is what the task list already hands the frontend and because it is what proves
// the task belongs to this user before anything is sent upstream.
func (h *ProductivityTasksHandler) CompleteTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)

	taskID := strings.TrimSpace(chi.URLParam(r, "taskID"))
	if taskID == "" {
		http.Error(w, "missing task id", http.StatusBadRequest)
		return
	}

	var source, externalID string
	err := h.db.QueryRow(ctx, `
		SELECT source, external_id FROM tasks WHERE id = $1 AND user_id = $2
	`, taskID, userID).Scan(&source, &externalID)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.logger.Error().Err(err).Str("task_id", taskID).Msg("look up task to complete")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if source != taskSourceVikunja {
		http.Error(w, "tasks from "+source+" are read-only", http.StatusConflict)
		return
	}

	task, err := h.vikunja.CompleteTask(ctx, userID, externalID)
	if err != nil {
		h.writeConnectorError(w, err, "complete vikunja task", userID)
		return
	}

	h.logger.Info().Str("user_id", userID).Str("task", task.ExternalID).Msg("vikunja task completed")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

// GetVikunjaProjects lists the projects a new task can go into.
func (h *ProductivityTasksHandler) GetVikunjaProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)

	projects, err := h.vikunja.Projects(ctx, userID)
	if err != nil {
		h.writeConnectorError(w, err, "list vikunja projects", userID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

// writeConnectorError keeps a missing integration apart from a failing one: the
// first is the user's to fix in Settings and carries the message that says so,
// the second is an upstream failure and stays out of the response body.
func (h *ProductivityTasksHandler) writeConnectorError(w http.ResponseWriter, err error, action, userID string) {
	if errors.Is(err, connectors.ErrVikunjaNotConnected) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	h.logger.Error().Err(err).Str("user_id", userID).Msg(action)
	http.Error(w, "vikunja request failed", http.StatusBadGateway)
}
