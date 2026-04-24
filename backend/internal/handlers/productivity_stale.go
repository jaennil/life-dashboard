package handlers

import (
	"fmt"
	"time"
)

func productivityStaleConditionExpr(todayStartArg, staleBeforeArg int) string {
	return fmt.Sprintf(`(
		added_at IS NOT NULL
		AND added_at < $%d
		AND (
			(due_at IS NULL AND due_date IS NULL)
			OR (due_at IS NOT NULL AND due_at < $%d)
			OR (due_at IS NULL AND due_date IS NOT NULL AND due_date < $%d::date)
		)
	)`, staleBeforeArg, todayStartArg, todayStartArg)
}

func productivityIsStaleTask(task ProductivityTask, now, todayStart, tomorrowStart, nextWeekStart, staleBefore time.Time) bool {
	if task.AddedAt == nil || !task.AddedAt.Before(staleBefore) {
		return false
	}

	isOverdue, bucket := productivityDueState(task.DueAt, task.DueDate, now, todayStart, tomorrowStart, nextWeekStart)
	return isOverdue || bucket == "no_due"
}
