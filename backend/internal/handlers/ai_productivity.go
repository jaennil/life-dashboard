package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type AIProductivityCompletion struct {
	CompletedAt time.Time `json:"completed_at"`
	Content     string    `json:"content"`
	ProjectName string    `json:"project_name,omitempty"`
}

type AIProductivityOverviewData struct {
	Source            string                     `json:"source"`
	Summary           ProductivitySummary        `json:"summary"`
	CompletedInWindow int                        `json:"completed_in_window"`
	KeyTasks          []ProductivityTask         `json:"key_tasks,omitempty"`
	RecentCompleted   []AIProductivityCompletion `json:"recent_completed,omitempty"`
}

func (h *AIHandler) appendProductivityContextInRange(ctx context.Context, sb *strings.Builder, userID string, start, end time.Time, title string, limit int) error {
	data, err := h.buildProductivityOverviewInRange(ctx, userID, start, end, limit)
	if err != nil {
		return err
	}
	sb.WriteString(renderProductivityOverviewText(title, data))
	return nil
}

func (h *AIHandler) buildProductivityOverviewInRange(ctx context.Context, userID string, start, end time.Time, limit int) (AIProductivityOverviewData, error) {
	if limit <= 0 {
		limit = 10
	}

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	nextWeekStart := todayStart.AddDate(0, 0, 8)
	staleBefore := todayStart.AddDate(0, 0, -14)
	staleCondition := productivityStaleConditionExpr(3, 6)

	data := AIProductivityOverviewData{
		Source: "tasks + task_completions",
	}

	if err := h.db.QueryRow(ctx, fmt.Sprintf(`
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
			COUNT(*) FILTER (WHERE is_active = TRUE AND %s)
		FROM tasks
		WHERE user_id = $1
	`, staleCondition), userID, now, todayStart, tomorrowStart, nextWeekStart, staleBefore).Scan(
		&data.Summary.ActiveTotal,
		&data.Summary.OverdueTotal,
		&data.Summary.DueTodayTotal,
		&data.Summary.DueNext7DaysTotal,
		&data.Summary.RecurringTotal,
		&data.Summary.StaleTotal,
	); err != nil {
		return AIProductivityOverviewData{}, err
	}

	if err := h.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE completed_at >= $2 AND completed_at < $3),
			COUNT(*) FILTER (WHERE completed_at >= $4 AND completed_at < $5),
			COUNT(*) FILTER (WHERE completed_at >= $6 AND completed_at < $5)
		FROM task_completions
		WHERE user_id = $1
	`, userID, start, end, todayStart, tomorrowStart, todayStart.AddDate(0, 0, -6)).Scan(
		&data.CompletedInWindow,
		&data.Summary.CompletedTodayTotal,
		&data.Summary.Completed7DaysTotal,
	); err != nil {
		return AIProductivityOverviewData{}, err
	}

	loadRows, err := h.db.Query(ctx, `
		SELECT COALESCE(due_at::date, due_date) AS day, COUNT(*)
		FROM tasks
		WHERE user_id = $1
			AND is_active = TRUE
			AND COALESCE(due_at::date, due_date) >= $2::date
			AND COALESCE(due_at::date, due_date) < $3::date
		GROUP BY day
		ORDER BY day ASC
		LIMIT 7
	`, userID, todayStart, nextWeekStart)
	if err != nil {
		return AIProductivityOverviewData{}, err
	}
	defer loadRows.Close()

	data.Summary.UpcomingLoad = make([]ProductivityDayBucket, 0, 7)
	for loadRows.Next() {
		var day time.Time
		var count int
		if err := loadRows.Scan(&day, &count); err != nil {
			return AIProductivityOverviewData{}, err
		}
		data.Summary.UpcomingLoad = append(data.Summary.UpcomingLoad, ProductivityDayBucket{
			Date:  day.Format("2006-01-02"),
			Count: count,
		})
	}
	if err := loadRows.Err(); err != nil {
		return AIProductivityOverviewData{}, err
	}

	taskRows, err := h.db.Query(ctx, fmt.Sprintf(`
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
		FROM tasks
		WHERE user_id = $1
			AND is_active = TRUE
			AND (
				(due_at IS NOT NULL AND due_at < $2)
				OR (due_at IS NULL AND due_date IS NOT NULL AND due_date < $3::date)
				OR (due_at IS NOT NULL AND due_at >= $3 AND due_at < $4)
				OR (due_at IS NULL AND due_date >= $3::date AND due_date < $5::date)
				OR %s
			)
		ORDER BY
			CASE
				WHEN (due_at IS NOT NULL AND due_at < $2) OR (due_at IS NULL AND due_date IS NOT NULL AND due_date < $3::date) THEN 0
				WHEN (due_at IS NOT NULL AND due_at >= $3 AND due_at < $4) OR (due_at IS NULL AND due_date = $3::date) THEN 1
				WHEN %s THEN 2
				ELSE 3
			END,
			COALESCE(due_at, due_date::timestamptz, added_at) ASC,
			priority DESC,
			content ASC
		LIMIT $7
	`, staleCondition, staleCondition), userID, now, todayStart, tomorrowStart, nextWeekStart, staleBefore, limit)
	if err != nil {
		return AIProductivityOverviewData{}, err
	}
	defer taskRows.Close()

	data.KeyTasks = make([]ProductivityTask, 0, limit)
	for taskRows.Next() {
		var task ProductivityTask
		if err := taskRows.Scan(
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
			return AIProductivityOverviewData{}, err
		}
		task.IsOverdue, task.DueBucket = productivityDueState(task.DueAt, task.DueDate, now, todayStart, tomorrowStart, nextWeekStart)
		data.KeyTasks = append(data.KeyTasks, task)
	}
	if err := taskRows.Err(); err != nil {
		return AIProductivityOverviewData{}, err
	}

	completedRows, err := h.db.Query(ctx, `
		SELECT completed_at, COALESCE(content, ''), COALESCE(project_name, '')
		FROM task_completions
		WHERE user_id = $1
			AND completed_at >= $2
			AND completed_at < $3
		ORDER BY completed_at DESC
		LIMIT 6
	`, userID, start, end)
	if err != nil {
		return AIProductivityOverviewData{}, err
	}
	defer completedRows.Close()

	data.RecentCompleted = make([]AIProductivityCompletion, 0, 6)
	for completedRows.Next() {
		var item AIProductivityCompletion
		if err := completedRows.Scan(&item.CompletedAt, &item.Content, &item.ProjectName); err != nil {
			return AIProductivityOverviewData{}, err
		}
		data.RecentCompleted = append(data.RecentCompleted, item)
	}
	if err := completedRows.Err(); err != nil {
		return AIProductivityOverviewData{}, err
	}

	return data, nil
}

func renderProductivityOverviewText(title string, data AIProductivityOverviewData) string {
	var sb strings.Builder
	sb.WriteString(title + "\n")
	sb.WriteString("Источник: Todoist. Просрочка и план задач считаются только по tasks, завершения — по task_completions.\n")
	sb.WriteString(fmt.Sprintf("Активных задач: %d | overdue: %d | сегодня: %d | ближайшие 7 дней: %d | recurring: %d | давно висят: %d\n",
		data.Summary.ActiveTotal,
		data.Summary.OverdueTotal,
		data.Summary.DueTodayTotal,
		data.Summary.DueNext7DaysTotal,
		data.Summary.RecurringTotal,
		data.Summary.StaleTotal,
	))
	sb.WriteString(fmt.Sprintf("Завершено за окно: %d | завершено сегодня: %d | завершено за 7 дней: %d\n",
		data.CompletedInWindow,
		data.Summary.CompletedTodayTotal,
		data.Summary.Completed7DaysTotal,
	))

	sb.WriteString("Нагрузка по ближайшим дням:\n")
	if len(data.Summary.UpcomingLoad) == 0 {
		sb.WriteString("  - На ближайшие дни задач с дедлайном нет\n")
	} else {
		for _, bucket := range data.Summary.UpcomingLoad {
			day, err := time.Parse("2006-01-02", bucket.Date)
			if err != nil {
				sb.WriteString(fmt.Sprintf("  - %s: %d задач\n", bucket.Date, bucket.Count))
				continue
			}
			sb.WriteString(fmt.Sprintf("  - %s: %d задач\n", day.Format("02.01"), bucket.Count))
		}
	}

	sb.WriteString("Ключевые задачи:\n")
	if len(data.KeyTasks) == 0 {
		sb.WriteString("  - Критичных или срочных задач сейчас нет\n")
	} else {
		now := aiNow()
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, aiTimeLocation())
		tomorrowStart := todayStart.AddDate(0, 0, 1)
		nextWeekStart := todayStart.AddDate(0, 0, 8)
		staleBefore := todayStart.AddDate(0, 0, -14)

		for _, task := range data.KeyTasks {
			label := "без срока"
			_, bucket := productivityDueState(task.DueAt, task.DueDate, now, todayStart, tomorrowStart, nextWeekStart)
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
			if productivityIsStaleTask(task, now, todayStart, tomorrowStart, nextWeekStart, staleBefore) {
				label = "висит давно"
			}

			line := fmt.Sprintf("  - %s", task.Content)
			if task.ProjectName != "" {
				line += fmt.Sprintf(" [%s", task.ProjectName)
				if task.SectionName != "" {
					line += " / " + task.SectionName
				}
				line += "]"
			}
			line += fmt.Sprintf(" | p%d | %s", task.Priority, label)
			if task.IsRecurring {
				line += " | recurring"
			}
			if task.DueAt != nil {
				line += " | дедлайн " + formatAITimestampLocal(*task.DueAt, "02.01 15:04")
			} else if task.DueDate != nil {
				line += " | дедлайн " + task.DueDate.Format("02.01")
			}
			if task.AddedAt != nil {
				line += " | добавлена " + task.AddedAt.Format("02.01")
			}
			sb.WriteString(line + "\n")
		}
	}

	sb.WriteString("Недавние закрытые задачи:\n")
	if len(data.RecentCompleted) == 0 {
		sb.WriteString("  - Нет завершённых задач за окно\n")
	} else {
		for _, item := range data.RecentCompleted {
			line := fmt.Sprintf("  - %s: %s", formatAITimestampLocal(item.CompletedAt, "02.01 15:04"), item.Content)
			if item.ProjectName != "" {
				line += " [" + item.ProjectName + "]"
			}
			sb.WriteString(line + "\n")
		}
	}

	return strings.TrimSpace(sb.String())
}
