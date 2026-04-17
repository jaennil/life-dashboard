package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (h *AIHandler) appendProductivityContextInRange(ctx context.Context, sb *strings.Builder, userID string, start, end time.Time, title string, limit int) error {
	if limit <= 0 {
		limit = 10
	}
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	nextWeekStart := todayStart.AddDate(0, 0, 8)
	staleBefore := todayStart.AddDate(0, 0, -14)

	sb.WriteString(title + "\n")
	sb.WriteString("Источник: Todoist. Просрочка и план задач считаются только по todoist_tasks, завершения — по todoist_task_completions.\n")

	var activeTotal, overdueTotal, dueTodayTotal, dueNext7DaysTotal, recurringTotal, staleTotal int
	if err := h.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE is_active = TRUE),
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
						(due_at IS NOT NULL AND due_at >= $3 AND due_at < $4)
						OR (due_at IS NULL AND due_date = $3::date)
					)
			),
			COUNT(*) FILTER (
				WHERE is_active = TRUE
					AND (
						(due_at IS NOT NULL AND due_at >= $4 AND due_at < $5)
						OR (due_at IS NULL AND due_date >= $4::date AND due_date < $5::date)
					)
			),
			COUNT(*) FILTER (WHERE is_active = TRUE AND is_recurring = TRUE),
			COUNT(*) FILTER (WHERE is_active = TRUE AND added_at IS NOT NULL AND added_at < $6)
		FROM todoist_tasks
		WHERE user_id = $1
	`, userID, now, todayStart, tomorrowStart, nextWeekStart, staleBefore).Scan(
		&activeTotal, &overdueTotal, &dueTodayTotal, &dueNext7DaysTotal, &recurringTotal, &staleTotal,
	); err != nil {
		return err
	}

	var completedInWindow, completedToday int
	if err := h.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE completed_at >= $2 AND completed_at < $3),
			COUNT(*) FILTER (WHERE completed_at >= $4 AND completed_at < $5)
		FROM todoist_task_completions
		WHERE user_id = $1
	`, userID, start, end, todayStart, tomorrowStart).Scan(&completedInWindow, &completedToday); err != nil {
		return err
	}

	sb.WriteString(fmt.Sprintf("Активных задач: %d | overdue: %d | сегодня: %d | ближайшие 7 дней: %d | recurring: %d | давно висят: %d\n",
		activeTotal, overdueTotal, dueTodayTotal, dueNext7DaysTotal, recurringTotal, staleTotal))
	sb.WriteString(fmt.Sprintf("Завершено за окно: %d | завершено сегодня: %d\n", completedInWindow, completedToday))

	loadRows, err := h.db.Query(ctx, `
		SELECT COALESCE(due_at::date, due_date) AS day, COUNT(*)
		FROM todoist_tasks
		WHERE user_id = $1
			AND is_active = TRUE
			AND COALESCE(due_at::date, due_date) >= $2::date
			AND COALESCE(due_at::date, due_date) < $3::date
		GROUP BY day
		ORDER BY day ASC
		LIMIT 7
	`, userID, todayStart, nextWeekStart)
	if err == nil {
		defer loadRows.Close()
		sb.WriteString("Нагрузка по ближайшим дням:\n")
		hasRows := false
		for loadRows.Next() {
			hasRows = true
			var day time.Time
			var count int
			if loadRows.Scan(&day, &count) == nil {
				sb.WriteString(fmt.Sprintf("  - %s: %d задач\n", day.Format("02.01"), count))
			}
		}
		if !hasRows {
			sb.WriteString("  - На ближайшие дни задач с дедлайном нет\n")
		}
	}

	taskRows, err := h.db.Query(ctx, `
		SELECT
			content,
			COALESCE(project_name, ''),
			COALESCE(section_name, ''),
			COALESCE(priority, 1),
			is_recurring,
			added_at,
			due_at,
			due_date::timestamp
		FROM todoist_tasks
		WHERE user_id = $1
			AND is_active = TRUE
			AND (
				(due_at IS NOT NULL AND due_at < $2)
				OR (due_at IS NULL AND due_date IS NOT NULL AND due_date < $3::date)
				OR (due_at IS NOT NULL AND due_at >= $3 AND due_at < $4)
				OR (due_at IS NULL AND due_date >= $3::date AND due_date < $5::date)
				OR (added_at IS NOT NULL AND added_at < $6)
			)
		ORDER BY
			CASE
				WHEN (due_at IS NOT NULL AND due_at < $2) OR (due_at IS NULL AND due_date IS NOT NULL AND due_date < $3::date) THEN 0
				WHEN (due_at IS NOT NULL AND due_at >= $3 AND due_at < $4) OR (due_at IS NULL AND due_date = $3::date) THEN 1
				WHEN added_at IS NOT NULL AND added_at < $6 THEN 2
				ELSE 3
			END,
			COALESCE(due_at, due_date::timestamptz, added_at) ASC,
			priority DESC,
			content ASC
		LIMIT $7
	`, userID, now, todayStart, tomorrowStart, nextWeekStart, staleBefore, limit)
	if err != nil {
		return err
	}
	defer taskRows.Close()

	sb.WriteString("Ключевые задачи:\n")
	hasTasks := false
	for taskRows.Next() {
		hasTasks = true
		var content, projectName, sectionName string
		var priority int
		var isRecurring bool
		var addedAt, dueAt, dueDate *time.Time
		if taskRows.Scan(&content, &projectName, &sectionName, &priority, &isRecurring, &addedAt, &dueAt, &dueDate) != nil {
			continue
		}

		label := "без срока"
		isOverdue, bucket := productivityDueState(dueAt, dueDate, now, todayStart, tomorrowStart, nextWeekStart)
		switch bucket {
		case "overdue":
			label = "overdue"
		case "today":
			label = "сегодня"
		case "upcoming":
			label = "скоро"
		case "later":
			label = "позже"
		}
		if !isOverdue && addedAt != nil && addedAt.Before(staleBefore) && (bucket == "no_due" || bucket == "later") {
			label = "висит давно"
		}

		line := fmt.Sprintf("  - %s", content)
		if projectName != "" {
			line += fmt.Sprintf(" [%s", projectName)
			if sectionName != "" {
				line += " / " + sectionName
			}
			line += "]"
		}
		line += fmt.Sprintf(" | p%d | %s", priority, label)
		if isRecurring {
			line += " | recurring"
		}
		if dueAt != nil {
			line += " | дедлайн " + dueAt.Format("02.01 15:04")
		} else if dueDate != nil {
			line += " | дедлайн " + dueDate.Format("02.01")
		}
		if addedAt != nil {
			line += " | добавлена " + addedAt.Format("02.01")
		}
		sb.WriteString(line + "\n")
	}
	if !hasTasks {
		sb.WriteString("  - Критичных или срочных задач сейчас нет\n")
	}

	completedRows, err := h.db.Query(ctx, `
		SELECT completed_at, COALESCE(content, ''), COALESCE(project_name, '')
		FROM todoist_task_completions
		WHERE user_id = $1
			AND completed_at >= $2
			AND completed_at < $3
		ORDER BY completed_at DESC
		LIMIT 6
	`, userID, start, end)
	if err == nil {
		defer completedRows.Close()
		sb.WriteString("Недавние закрытые задачи:\n")
		hasCompleted := false
		for completedRows.Next() {
			hasCompleted = true
			var completedAt time.Time
			var content, projectName string
			if completedRows.Scan(&completedAt, &content, &projectName) == nil {
				line := fmt.Sprintf("  - %s: %s", completedAt.Format("02.01 15:04"), content)
				if projectName != "" {
					line += " [" + projectName + "]"
				}
				sb.WriteString(line + "\n")
			}
		}
		if !hasCompleted {
			sb.WriteString("  - Нет завершённых задач за окно\n")
		}
	}

	return nil
}
