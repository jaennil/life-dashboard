package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type aiHabitSourceMeta struct {
	Title                     string
	Intro                     string
	NoStatusMessage           string
	MissingStatusCaveat       string
	TreatStatusesAsFullWindow bool
}

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
		sb.WriteString("=== ПРИВЫЧКИ ===\nДанные по привычкам временно недоступны.\n")
	}
}

func (h *AIHandler) appendHabitContextInRange(ctx context.Context, sb *strings.Builder, userID string, start, end time.Time, header string, limit int) error {
	if limit <= 0 {
		limit = 12
	}

	sources, err := h.loadHabitSources(ctx, userID)
	if err != nil {
		return err
	}

	sb.WriteString(header + "\n")
	if len(sources) == 0 {
		sb.WriteString("Нет подключённых источников привычек\n")
		return nil
	}

	for idx, source := range sources {
		if idx > 0 {
			sb.WriteString("\n")
		}
		if err := h.appendHabitSourceContext(ctx, sb, userID, source, start, end, limit); err != nil {
			return err
		}
	}
	return nil
}

func (h *AIHandler) loadHabitSources(ctx context.Context, userID string) ([]string, error) {
	// GROUP BY rather than DISTINCT: Postgres rejects ordering a DISTINCT
	// select by an expression that is not in the select list, and this query
	// failed with 42P10 on every call, which silently dropped the whole habit
	// section from the AI context instead of failing loudly.
	rows, err := h.db.Query(ctx, `
		SELECT source
		FROM habits
		WHERE user_id = $1
			AND source IN ('manual', 'habitify', 'todoist', 'vikunja')
		GROUP BY source
		ORDER BY CASE source
			WHEN 'manual' THEN 1
			WHEN 'habitify' THEN 2
			WHEN 'todoist' THEN 3
			WHEN 'vikunja' THEN 4
			ELSE 99
		END
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sources := make([]string, 0, 2)
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err == nil && strings.TrimSpace(source) != "" {
			sources = append(sources, source)
		}
	}
	return sources, nil
}

func habitSourceMeta(source string) aiHabitSourceMeta {
	switch source {
	case "manual":
		return aiHabitSourceMeta{
			Title:                     "Life Dashboard Routines",
			Intro:                     "Источник: встроенные рутинные чеклисты Life Dashboard. Отсутствие отметки за день считается пропуском, это не план из календаря.",
			NoStatusMessage:           "Нет локальных рутин или отметок за период",
			TreatStatusesAsFullWindow: true,
		}
	case "todoist":
		return aiHabitSourceMeta{
			Title:               "Todoist",
			Intro:               "Источник: recurring tasks из Todoist. Фактом выполнения считаются completion events, а не план из календаря.",
			NoStatusMessage:     "Нет completion events Todoist за период",
			MissingStatusCaveat: "Todoist completed archive может быть недоступен на текущем плане, поэтому пропуски и полная история выполнения могут быть видны не полностью.",
		}
	case "vikunja":
		return aiHabitSourceMeta{
			Title:           "Vikunja",
			Intro:           "Источник: повторяющиеся задачи Vikunja. Фактом выполнения считаются отметки done_at, а не план из календаря.",
			NoStatusMessage: "Нет выполнений повторяющихся задач Vikunja за период",
			// Vikunja keeps only the last done_at per task, so history before the
			// first sync of a task does not exist and its absence is not a skip.
			MissingStatusCaveat: "История выполнений Vikunja накапливается только с момента подключения интеграции, поэтому ранние пропуски могут быть не видны.",
		}
	default:
		return aiHabitSourceMeta{
			Title:                     "Habitify",
			Intro:                     "Источник: Habitify. Это факты по отметкам привычек за день, а не план из календаря.",
			NoStatusMessage:           "Нет дневных отметок Habitify за период",
			TreatStatusesAsFullWindow: true,
		}
	}
}

func (h *AIHandler) appendHabitSourceContext(ctx context.Context, sb *strings.Builder, userID, source string, start, end time.Time, limit int) error {
	meta := habitSourceMeta(source)
	sb.WriteString(fmt.Sprintf("--- %s ---\n", meta.Title))
	sb.WriteString(meta.Intro + "\n")

	var activeHabits, archivedHabits int
	if err := h.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE archived = FALSE),
			COUNT(*) FILTER (WHERE archived = TRUE)
		FROM habits
		WHERE user_id = $1 AND source = $2
	`, userID, source).Scan(&activeHabits, &archivedHabits); err != nil {
		return err
	}

	var totalStatuses, completed, inProgress, skipped, failed, none int
	var countErr error
	if source == manualHabitSource {
		countErr = h.db.QueryRow(ctx, `
			WITH days AS (
				SELECT generate_series($3::date, $4::date, interval '1 day')::date AS target_date
			)
			SELECT
				COUNT(*),
				COUNT(*) FILTER (WHERE COALESCE(s.status, 'none') = 'completed'),
				COUNT(*) FILTER (WHERE COALESCE(s.status, 'none') = 'in_progress'),
				COUNT(*) FILTER (WHERE COALESCE(s.status, 'none') = 'skipped'),
				COUNT(*) FILTER (WHERE COALESCE(s.status, 'none') = 'failed'),
				COUNT(*) FILTER (WHERE COALESCE(s.status, 'none') = 'none')
			FROM habits h
			CROSS JOIN days d
			LEFT JOIN habit_daily_statuses s
				ON s.habit_id = h.id
				AND s.target_date = d.target_date
			WHERE h.user_id = $1
				AND h.source = $2
				AND h.archived = FALSE
		`, userID, source, start, end).Scan(&totalStatuses, &completed, &inProgress, &skipped, &failed, &none)
	} else {
		countErr = h.db.QueryRow(ctx, `
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
				AND h.source = $2
				AND s.target_date >= $3::date
				AND s.target_date <= $4::date
		`, userID, source, start, end).Scan(&totalStatuses, &completed, &inProgress, &skipped, &failed, &none)
	}
	if countErr != nil {
		return countErr
	}

	sb.WriteString(fmt.Sprintf("Активных привычек: %d", activeHabits))
	if archivedHabits > 0 {
		sb.WriteString(fmt.Sprintf(", архивных: %d", archivedHabits))
	}
	sb.WriteString("\n")

	if totalStatuses == 0 {
		sb.WriteString(meta.NoStatusMessage + "\n")
		if meta.MissingStatusCaveat != "" {
			sb.WriteString(meta.MissingStatusCaveat + "\n")
		}
	} else {
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

		if meta.TreatStatusesAsFullWindow {
			completionRate := 0.0
			if totalStatuses > 0 {
				completionRate = float64(completed) / float64(totalStatuses) * 100
			}
			sb.WriteString(fmt.Sprintf("За период: %d дневных статусов, completion rate %.0f%% (%s)\n", totalStatuses, completionRate, strings.Join(trackedStatuses, ", ")))
		} else {
			sb.WriteString(fmt.Sprintf("За период: %d зафиксированных статусов (%s)\n", totalStatuses, strings.Join(trackedStatuses, ", ")))
			if meta.MissingStatusCaveat != "" {
				sb.WriteString(meta.MissingStatusCaveat + "\n")
			}
		}
	}

	query := `
		SELECT
			h.name,
			COALESCE(h.area_name, ''),
			COALESCE(h.recurrence, ''),
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
			AND s.target_date >= $3::date
			AND s.target_date <= $4::date
		LEFT JOIN LATERAL (
			SELECT target_date, status, current_value, target_value, unit_type
			FROM habit_daily_statuses s2
			WHERE s2.habit_id = h.id
				AND s2.target_date >= $3::date
				AND s2.target_date <= $4::date
			ORDER BY s2.target_date DESC
			LIMIT 1
		) latest ON true
		WHERE h.user_id = $1
			AND h.source = $2
		GROUP BY h.id, latest.target_date, latest.status, latest.current_value, latest.target_value, latest.unit_type
		ORDER BY h.archived ASC, completed_days DESC, tracked_days DESC, h.name ASC
		LIMIT $5
	`
	if source == manualHabitSource {
		query = `
			WITH days AS (
				SELECT generate_series($3::date, $4::date, interval '1 day')::date AS target_date
			)
			SELECT
				h.name,
				COALESCE(h.area_name, ''),
				COALESCE(h.recurrence, ''),
				h.archived,
				COUNT(*) AS tracked_days,
				COUNT(*) FILTER (WHERE COALESCE(s.status, 'none') = 'completed') AS completed_days,
				COUNT(*) FILTER (WHERE COALESCE(s.status, 'none') = 'in_progress') AS in_progress_days,
				COUNT(*) FILTER (WHERE COALESCE(s.status, 'none') = 'skipped') AS skipped_days,
				COUNT(*) FILTER (WHERE COALESCE(s.status, 'none') = 'failed') AS failed_days,
				COUNT(*) FILTER (WHERE COALESCE(s.status, 'none') = 'none') AS none_days,
				$4::date AS latest_target_date,
				MAX(CASE WHEN d.target_date = $4::date THEN COALESCE(s.status, 'none') END) AS latest_status,
				NULL::double precision AS current_value,
				NULL::double precision AS target_value,
				'' AS unit_type
			FROM habits h
			CROSS JOIN days d
			LEFT JOIN habit_daily_statuses s
				ON s.habit_id = h.id
				AND s.target_date = d.target_date
			WHERE h.user_id = $1
				AND h.source = $2
			GROUP BY h.id
			ORDER BY h.archived ASC, completed_days DESC, tracked_days DESC, h.name ASC
			LIMIT $5
		`
	}
	rows, err := h.db.Query(ctx, query, userID, source, start, end, limit)
	if err != nil {
		return err
	}
	defer rows.Close()

	sb.WriteString("Привычки:\n")
	hasRows := false
	for rows.Next() {
		hasRows = true
		var name, areaName, recurrence, latestStatus, unitType string
		var archived bool
		var trackedDays, completedDays, inProgressDays, skippedDays, failedDays, noneDays int
		var latestDate *time.Time
		var currentValue, targetValue *float64
		if err := rows.Scan(
			&name,
			&areaName,
			&recurrence,
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

		line := fmt.Sprintf("  - %s", name)
		if areaName != "" {
			line += fmt.Sprintf(" [%s]", areaName)
		}
		if recurrence != "" {
			line += fmt.Sprintf(" {%s}", recurrence)
		}
		if archived {
			line += " [архив]"
		}

		parts := make([]string, 0, 6)
		if meta.TreatStatusesAsFullWindow {
			parts = append(parts, fmt.Sprintf("%d/%d выполнено", completedDays, trackedDays))
		} else if completedDays > 0 {
			parts = append(parts, fmt.Sprintf("выполнено %d раз", completedDays))
		} else if trackedDays > 0 {
			parts = append(parts, fmt.Sprintf("статусов %d", trackedDays))
		} else {
			parts = append(parts, "нет completion events за период")
		}
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
		sb.WriteString(fmt.Sprintf("  - Нет привычек %s\n", meta.Title))
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
