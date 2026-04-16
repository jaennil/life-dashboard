package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (h *AIHandler) buildHabitContext(ctx context.Context, userID string, days int) (string, error) {
	if days <= 0 {
		days = 14
	}
	start := startOfDay(time.Now().AddDate(0, 0, -(days - 1)))
	end := time.Now()
	var sb strings.Builder
	if err := h.appendHabitContextInRange(ctx, &sb, userID, start, end, fmt.Sprintf("=== ПРИВЫЧКИ (%d дней) ===", days), 12); err != nil {
		return "", err
	}
	return strings.TrimSpace(sb.String()), nil
}

func (h *AIHandler) appendCheckupHabitContext(ctx context.Context, sb *strings.Builder, userID string, window checkupWindow) {
	sb.WriteString("\n")
	if err := h.appendHabitContextInRange(ctx, sb, userID, window.Start, window.End, "=== ПРИВЫЧКИ ===", 12); err != nil {
		h.logger.Warn().Err(err).Str("user_id", userID).Msg("build habit checkup context")
		sb.WriteString("=== ПРИВЫЧКИ ===\nДанные Habitify временно недоступны.\n")
	}
}

func (h *AIHandler) appendHabitContextInRange(ctx context.Context, sb *strings.Builder, userID string, start, end time.Time, header string, limit int) error {
	if limit <= 0 {
		limit = 12
	}

	sb.WriteString(header + "\n")
	sb.WriteString("Источник: Habitify. Это факты по отметкам привычек за день, а не план из календаря.\n")

	var activeHabits, archivedHabits int
	if err := h.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE archived = FALSE),
			COUNT(*) FILTER (WHERE archived = TRUE)
		FROM habits
		WHERE user_id = $1 AND source = 'habitify'
	`, userID).Scan(&activeHabits, &archivedHabits); err != nil {
		return err
	}

	var totalStatuses, completed, inProgress, skipped, failed, none int
	if err := h.db.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE s.status = 'completed'),
			COUNT(*) FILTER (WHERE s.status = 'in_progress'),
			COUNT(*) FILTER (WHERE s.status = 'skipped'),
			COUNT(*) FILTER (WHERE s.status = 'failed'),
			COUNT(*) FILTER (WHERE s.status = 'none')
		FROM habit_daily_statuses s
		JOIN habits h ON h.id = s.habit_id
		WHERE h.user_id = $1
			AND h.source = 'habitify'
			AND s.target_date >= $2::date
			AND s.target_date <= $3::date
	`, userID, start, end).Scan(&totalStatuses, &completed, &inProgress, &skipped, &failed, &none); err != nil {
		return err
	}

	sb.WriteString(fmt.Sprintf("Активных привычек: %d", activeHabits))
	if archivedHabits > 0 {
		sb.WriteString(fmt.Sprintf(", архивных: %d", archivedHabits))
	}
	sb.WriteString("\n")

	if totalStatuses == 0 {
		sb.WriteString("Нет дневных отметок Habitify за период\n")
		return nil
	}

	completionRate := 0.0
	if totalStatuses > 0 {
		completionRate = float64(completed) / float64(totalStatuses) * 100
	}
	trackedStatuses := make([]string, 0, 5)
	trackedStatuses = append(trackedStatuses, fmt.Sprintf("выполнено %d", completed))
	if inProgress > 0 {
		trackedStatuses = append(trackedStatuses, fmt.Sprintf("в процессе %d", inProgress))
	}
	if skipped > 0 {
		trackedStatuses = append(trackedStatuses, fmt.Sprintf("пропущено %d", skipped))
	}
	if failed > 0 {
		trackedStatuses = append(trackedStatuses, fmt.Sprintf("не выполнено %d", failed))
	}
	if none > 0 {
		trackedStatuses = append(trackedStatuses, fmt.Sprintf("без отметки %d", none))
	}
	sb.WriteString(fmt.Sprintf("За период: %d дневных статусов, completion rate %.0f%% (%s)\n", totalStatuses, completionRate, strings.Join(trackedStatuses, ", ")))

	rows, err := h.db.Query(ctx, `
		SELECT
			h.name,
			COALESCE(h.area_name, ''),
			h.archived,
			COUNT(s.id) AS tracked_days,
			COUNT(*) FILTER (WHERE s.status = 'completed') AS completed_days,
			COUNT(*) FILTER (WHERE s.status = 'in_progress') AS in_progress_days,
			COUNT(*) FILTER (WHERE s.status = 'skipped') AS skipped_days,
			COUNT(*) FILTER (WHERE s.status = 'failed') AS failed_days,
			COUNT(*) FILTER (WHERE s.status = 'none') AS none_days,
			latest.target_date,
			COALESCE(latest.status, ''),
			latest.current_value,
			latest.target_value,
			COALESCE(latest.unit_type, '')
		FROM habits h
		LEFT JOIN habit_daily_statuses s
			ON s.habit_id = h.id
			AND s.target_date >= $2::date
			AND s.target_date <= $3::date
		LEFT JOIN LATERAL (
			SELECT target_date, status, current_value, target_value, unit_type
			FROM habit_daily_statuses s2
			WHERE s2.habit_id = h.id
				AND s2.target_date >= $2::date
				AND s2.target_date <= $3::date
			ORDER BY s2.target_date DESC
			LIMIT 1
		) latest ON true
		WHERE h.user_id = $1
			AND h.source = 'habitify'
		GROUP BY h.id, latest.target_date, latest.status, latest.current_value, latest.target_value, latest.unit_type
		ORDER BY h.archived ASC, completed_days DESC, tracked_days DESC, h.name ASC
		LIMIT $4
	`, userID, start, end, limit)
	if err != nil {
		return err
	}
	defer rows.Close()

	sb.WriteString("Привычки:\n")
	hasRows := false
	for rows.Next() {
		hasRows = true
		var name, areaName, latestStatus, unitType string
		var archived bool
		var trackedDays, completedDays, inProgressDays, skippedDays, failedDays, noneDays int
		var latestDate *time.Time
		var currentValue, targetValue *float64
		if err := rows.Scan(
			&name,
			&areaName,
			&archived,
			&trackedDays,
			&completedDays,
			&inProgressDays,
			&skippedDays,
			&failedDays,
			&noneDays,
			&latestDate,
			&latestStatus,
			&currentValue,
			&targetValue,
			&unitType,
		); err != nil {
			continue
		}

		parts := []string{fmt.Sprintf("%d/%d выполнено", completedDays, trackedDays)}
		if noneDays > 0 {
			parts = append(parts, fmt.Sprintf("без отметки %d", noneDays))
		}
		if skippedDays > 0 {
			parts = append(parts, fmt.Sprintf("пропущено %d", skippedDays))
		}
		if failedDays > 0 {
			parts = append(parts, fmt.Sprintf("не выполнено %d", failedDays))
		}
		if inProgressDays > 0 {
			parts = append(parts, fmt.Sprintf("в процессе %d", inProgressDays))
		}

		line := fmt.Sprintf("  - %s", name)
		if areaName != "" {
			line += fmt.Sprintf(" [%s]", areaName)
		}
		if archived {
			line += " [архив]"
		}
		line += ": " + strings.Join(parts, ", ")
		if latestDate != nil {
			line += fmt.Sprintf("; последний статус %s %s", latestDate.Format("02.01"), aiHabitStatusLabel(latestStatus))
		}
		if currentValue != nil || targetValue != nil {
			line += fmt.Sprintf("; прогресс %s/%s", formatAIHabitMetric(currentValue), formatAIHabitMetric(targetValue))
			if unitType != "" {
				line += " " + unitType
			}
		}
		sb.WriteString(line + "\n")
	}
	if !hasRows {
		sb.WriteString("  - Нет привычек Habitify\n")
	}

	return nil
}

func aiHabitStatusLabel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return "выполнено"
	case "in_progress":
		return "в процессе"
	case "skipped":
		return "пропущено"
	case "failed":
		return "не выполнено"
	case "none", "":
		return "без отметки"
	default:
		return status
	}
}

func formatAIHabitMetric(value *float64) string {
	if value == nil {
		return "-"
	}
	if *value == float64(int64(*value)) {
		return fmt.Sprintf("%.0f", *value)
	}
	return fmt.Sprintf("%.2f", *value)
}
