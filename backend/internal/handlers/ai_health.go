package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (h *AIHandler) appendHealthContextInRange(ctx context.Context, sb *strings.Builder, userID string, start, end time.Time, title string) {
	if title == "" {
		title = "=== ЗДОРОВЬЕ ==="
	}

	sb.WriteString(title + "\n")
	sb.WriteString("Фактические показатели здоровья берутся только из biometrics/sleep_sessions (health webhook / Apple Health, если настроено). Календарь сюда не относится.\n")

	var totalSteps, avgSteps float64
	var stepDaysCount int
	h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(day_steps), 0), COALESCE(AVG(day_steps), 0), COUNT(*)
		FROM (
			SELECT DATE(timestamp) AS day, SUM(value) AS day_steps
			FROM biometrics
			WHERE user_id = $1
				AND metric_type = 'steps'
				AND timestamp >= $2
				AND timestamp < $3
			GROUP BY DATE(timestamp)
		) steps
	`, userID, start, end).Scan(&totalSteps, &avgSteps, &stepDaysCount)

	var bestStepDay string
	var bestSteps float64
	_ = h.db.QueryRow(ctx, `
		SELECT TO_CHAR(DATE(timestamp), 'DD.MM'), COALESCE(SUM(value), 0) AS day_steps
		FROM biometrics
		WHERE user_id = $1
			AND metric_type = 'steps'
			AND timestamp >= $2
			AND timestamp < $3
		GROUP BY DATE(timestamp)
		ORDER BY day_steps DESC
		LIMIT 1
	`, userID, start, end).Scan(&bestStepDay, &bestSteps)

	if stepDaysCount > 0 {
		sb.WriteString(fmt.Sprintf("Шаги: всего %.0f, среднее %.0f в день", totalSteps, avgSteps))
		if bestStepDay != "" {
			sb.WriteString(fmt.Sprintf(", лучший день %s (%.0f)", bestStepDay, bestSteps))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("Шаги: нет данных за период\n")
	}

	type weightPoint struct {
		value     float64
		timestamp time.Time
	}
	var firstWeight, lastWeight weightPoint
	firstWeightErr := h.db.QueryRow(ctx, `
		SELECT value, timestamp
		FROM biometrics
		WHERE user_id = $1
			AND metric_type = 'weight'
			AND timestamp >= $2
			AND timestamp < $3
		ORDER BY timestamp ASC
		LIMIT 1
	`, userID, start, end).Scan(&firstWeight.value, &firstWeight.timestamp)
	lastWeightErr := h.db.QueryRow(ctx, `
		SELECT value, timestamp
		FROM biometrics
		WHERE user_id = $1
			AND metric_type = 'weight'
			AND timestamp >= $2
			AND timestamp < $3
		ORDER BY timestamp DESC
		LIMIT 1
	`, userID, start, end).Scan(&lastWeight.value, &lastWeight.timestamp)
	if firstWeightErr == nil && lastWeightErr == nil {
		sb.WriteString(fmt.Sprintf("Вес: %.1f → %.1f кг (Δ %.1f кг)\n", firstWeight.value, lastWeight.value, lastWeight.value-firstWeight.value))
	} else {
		sb.WriteString("Вес: нет данных за период\n")
	}

	var avgRestingHR float64
	var restingCount int
	h.db.QueryRow(ctx, `
		SELECT COALESCE(AVG(value), 0), COUNT(*)
		FROM biometrics
		WHERE user_id = $1
			AND metric_type = 'resting_heart_rate'
			AND timestamp >= $2
			AND timestamp < $3
	`, userID, start, end).Scan(&avgRestingHR, &restingCount)
	if restingCount > 0 {
		sb.WriteString(fmt.Sprintf("Пульс покоя: среднее %.0f bpm\n", avgRestingHR))
	} else {
		sb.WriteString("Пульс покоя: нет данных за период\n")
	}

	var avgHRV float64
	var hrvCount int
	h.db.QueryRow(ctx, `
		SELECT COALESCE(AVG(value), 0), COUNT(*)
		FROM biometrics
		WHERE user_id = $1
			AND metric_type = 'hrv'
			AND timestamp >= $2
			AND timestamp < $3
	`, userID, start, end).Scan(&avgHRV, &hrvCount)
	if hrvCount > 0 {
		sb.WriteString(fmt.Sprintf("HRV: среднее %.0f ms\n", avgHRV))
	} else {
		sb.WriteString("HRV: нет данных за период\n")
	}

	var activeEnergy float64
	var activeEnergyDays int
	h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(day_energy), 0), COUNT(*)
		FROM (
			SELECT DATE(timestamp) AS day, SUM(value) AS day_energy
			FROM biometrics
			WHERE user_id = $1
				AND metric_type = 'active_energy'
				AND timestamp >= $2
				AND timestamp < $3
			GROUP BY DATE(timestamp)
		) energy
	`, userID, start, end).Scan(&activeEnergy, &activeEnergyDays)
	if activeEnergyDays > 0 {
		sb.WriteString(fmt.Sprintf("Активная энергия: всего %.0f ккал за %d дн.\n", activeEnergy, activeEnergyDays))
	} else {
		sb.WriteString("Активная энергия: нет данных за период\n")
	}

	var sleepSessions int
	var avgSleepMinutes float64
	h.db.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(AVG(total_sleep_minutes), 0)
		FROM sleep_sessions
		WHERE user_id = $1
			AND date >= $2::date
			AND date <= $3::date
	`, userID, start, end).Scan(&sleepSessions, &avgSleepMinutes)
	if sleepSessions > 0 {
		sb.WriteString(fmt.Sprintf("Сон: %d сессий, среднее %.1f ч\n", sleepSessions, avgSleepMinutes/60.0))
	} else {
		sb.WriteString("Сон: нет данных за период\n")
	}
}
