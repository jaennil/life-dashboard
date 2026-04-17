package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	authmw "life-dashboard/internal/middleware"
)

type ProductivityHandler struct {
	db     *pgxpool.Pool
	logger zerolog.Logger
}

func NewProductivity(db *pgxpool.Pool, logger zerolog.Logger) *ProductivityHandler {
	return &ProductivityHandler{db: db, logger: logger.With().Str("handler", "productivity").Logger()}
}

type ProductivitySummary struct {
	ActiveTotal         int                     `json:"active_total"`
	OverdueTotal        int                     `json:"overdue_total"`
	DueTodayTotal       int                     `json:"due_today_total"`
	DueNext7DaysTotal   int                     `json:"due_next_7_days_total"`
	RecurringTotal      int                     `json:"recurring_total"`
	StaleTotal          int                     `json:"stale_total"`
	CompletedTodayTotal int                     `json:"completed_today_total"`
	Completed7DaysTotal int                     `json:"completed_7_days_total"`
	UpcomingLoad        []ProductivityDayBucket `json:"upcoming_load"`
}

type ProductivityDayBucket struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type ProductivityTask struct {
	ID              string     `json:"id"`
	ExternalID      string     `json:"external_id"`
	Content         string     `json:"content"`
	Description     string     `json:"description"`
	ProjectName     string     `json:"project_name"`
	SectionName     string     `json:"section_name"`
	Priority        int        `json:"priority"`
	IsRecurring     bool       `json:"is_recurring"`
	AddedAt         *time.Time `json:"added_at"`
	DueAt           *time.Time `json:"due_at"`
	DueDate         *time.Time `json:"due_date"`
	LastCompletedAt *time.Time `json:"last_completed_at"`
	IsOverdue       bool       `json:"is_overdue"`
	DueBucket       string     `json:"due_bucket"`
}

func (h *ProductivityHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	nextWeekStart := todayStart.AddDate(0, 0, 8)
	staleBefore := todayStart.AddDate(0, 0, -14)

	var summary ProductivitySummary
	_ = h.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (
				WHERE is_active = TRUE
			),
			COUNT(*) FILTER (
				WHERE is_active = TRUE
					AND (
						(due_at IS NOT NULL AND due_at < $2)
						OR (due_at IS NULL AND due_date IS NOT NULL AND due_date < $3::date)
					)
			),
			COUNT(*) FILTER (
				WHERE is_active = TRUE
					AND (
						(due_at IS NOT NULL AND due_at >= $2 AND due_at < $3)
						OR (due_at IS NULL AND due_date = $2::date)
					)
			),
			COUNT(*) FILTER (
				WHERE is_active = TRUE
					AND (
						(due_at IS NOT NULL AND due_at >= $3 AND due_at < $4)
						OR (due_at IS NULL AND due_date >= $3::date AND due_date < $4::date)
					)
			),
			COUNT(*) FILTER (WHERE is_active = TRUE AND is_recurring = TRUE),
			COUNT(*) FILTER (WHERE is_active = TRUE AND added_at IS NOT NULL AND added_at < $5)
		FROM todoist_tasks
		WHERE user_id = $1
	`, userID, todayStart, tomorrowStart, nextWeekStart, staleBefore).Scan(
		&summary.ActiveTotal,
		&summary.OverdueTotal,
		&summary.DueTodayTotal,
		&summary.DueNext7DaysTotal,
		&summary.RecurringTotal,
		&summary.StaleTotal,
	)

	_ = h.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE completed_at >= $2 AND completed_at < $3),
			COUNT(*) FILTER (WHERE completed_at >= $4 AND completed_at < $3)
		FROM todoist_task_completions
		WHERE user_id = $1
	`, userID, todayStart, tomorrowStart, todayStart.AddDate(0, 0, -6)).Scan(
		&summary.CompletedTodayTotal,
		&summary.Completed7DaysTotal,
	)

	rows, err := h.db.Query(ctx, `
		SELECT day::date, COUNT(*)
		FROM (
			SELECT COALESCE(due_at::date, due_date) AS day
			FROM todoist_tasks
			WHERE user_id = $1
				AND is_active = TRUE
				AND COALESCE(due_at::date, due_date) >= $2::date
				AND COALESCE(due_at::date, due_date) < $3::date
		) tasks
		GROUP BY day
		ORDER BY day ASC
	`, userID, todayStart, nextWeekStart)
	if err == nil {
		defer rows.Close()
		summary.UpcomingLoad = make([]ProductivityDayBucket, 0, 7)
		for rows.Next() {
			var day time.Time
			var count int
			if rows.Scan(&day, &count) == nil {
				summary.UpcomingLoad = append(summary.UpcomingLoad, ProductivityDayBucket{
					Date:  day.Format("2006-01-02"),
					Count: count,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func (h *ProductivityHandler) GetTasks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)
	filter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("filter")))
	if filter == "" {
		filter = "all"
	}

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	nextWeekStart := todayStart.AddDate(0, 0, 8)
	staleBefore := todayStart.AddDate(0, 0, -14)

	baseWhere := `
		WHERE user_id = $1
			AND is_active = TRUE
	`
	orderBy := `
		ORDER BY
			CASE
				WHEN (due_at IS NOT NULL AND due_at < $2) OR (due_at IS NULL AND due_date IS NOT NULL AND due_date < $3::date) THEN 0
				WHEN (due_at IS NOT NULL AND due_at >= $2 AND due_at < $3) OR (due_at IS NULL AND due_date = $2::date) THEN 1
				WHEN COALESCE(due_at::date, due_date) IS NOT NULL THEN 2
				ELSE 3
			END,
			COALESCE(due_at, due_date::timestamptz, added_at, created_at) ASC,
			priority DESC,
			content ASC
	`

	switch filter {
	case "overdue":
		baseWhere += `
			AND (
				(due_at IS NOT NULL AND due_at < $2)
				OR (due_at IS NULL AND due_date IS NOT NULL AND due_date < $3::date)
			)
		`
	case "today":
		baseWhere += `
			AND (
				(due_at IS NOT NULL AND due_at >= $2 AND due_at < $3)
				OR (due_at IS NULL AND due_date = $2::date)
			)
		`
	case "upcoming":
		baseWhere += `
			AND (
				(due_at IS NOT NULL AND due_at >= $3 AND due_at < $4)
				OR (due_at IS NULL AND due_date >= $3::date AND due_date < $4::date)
			)
		`
	case "stale":
		baseWhere += `
			AND added_at IS NOT NULL AND added_at < $5
		`
	case "all":
	default:
		http.Error(w, "unknown filter", http.StatusBadRequest)
		return
	}

	rows, err := h.db.Query(ctx, `
		SELECT
			id,
			external_id,
			content,
			COALESCE(description, ''),
			COALESCE(project_name, ''),
			COALESCE(section_name, ''),
			COALESCE(priority, 1),
			is_recurring,
			added_at,
			due_at,
			due_date::timestamp,
			last_completed_at
		FROM todoist_tasks
	`+baseWhere+orderBy+`
		LIMIT 100
	`, userID, todayStart, tomorrowStart, nextWeekStart, staleBefore)
	if err != nil {
		h.logger.Error().Err(err).Str("filter", filter).Msg("query productivity tasks")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	tasks := make([]ProductivityTask, 0, 64)
	for rows.Next() {
		var task ProductivityTask
		if err := rows.Scan(
			&task.ID,
			&task.ExternalID,
			&task.Content,
			&task.Description,
			&task.ProjectName,
			&task.SectionName,
			&task.Priority,
			&task.IsRecurring,
			&task.AddedAt,
			&task.DueAt,
			&task.DueDate,
			&task.LastCompletedAt,
		); err != nil {
			continue
		}
		task.IsOverdue, task.DueBucket = productivityDueState(task.DueAt, task.DueDate, now, todayStart, tomorrowStart, nextWeekStart)
		tasks = append(tasks, task)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func productivityDueState(dueAt, dueDate *time.Time, now, todayStart, tomorrowStart, nextWeekStart time.Time) (bool, string) {
	if dueAt != nil {
		switch {
		case dueAt.Before(now):
			return true, "overdue"
		case dueAt.Before(tomorrowStart):
			return false, "today"
		case dueAt.Before(nextWeekStart):
			return false, "upcoming"
		default:
			return false, "later"
		}
	}
	if dueDate != nil {
		day := time.Date(dueDate.Year(), dueDate.Month(), dueDate.Day(), 0, 0, 0, 0, dueDate.Location())
		switch {
		case day.Before(todayStart):
			return true, "overdue"
		case day.Equal(todayStart):
			return false, "today"
		case day.Before(nextWeekStart):
			return false, "upcoming"
		default:
			return false, "later"
		}
	}
	return false, "no_due"
}
