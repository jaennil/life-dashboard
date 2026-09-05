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

	h.appendSleepStages(ctx, sb, userID, start, end)
	h.appendHeartRateDetail(ctx, sb, userID, start, end)
	h.appendBodyComposition(ctx, sb, userID, start, end)
	h.appendWalkingQuality(ctx, sb, userID, start, end)
}

// sleepStageTotal is one stage summed over the window, with the number of
// nights that actually carried it.
type sleepStageTotal struct {
	Stage   string
	Minutes float64
	Nights  int
}

// appendSleepStages turns the stage intervals into what a person would ask
// about: how much deep and REM sleep a night actually held. The stages are
// stored as intervals, so the minutes are summed from their boundaries.
func (h *AIHandler) appendSleepStages(ctx context.Context, sb *strings.Builder, userID string, start, end time.Time) {
	rows, err := h.db.Query(ctx, `
		SELECT LOWER(st.stage),
		       SUM(EXTRACT(EPOCH FROM (st.ended_at - st.started_at)) / 60.0) AS minutes,
		       COUNT(DISTINCT s.date) AS nights
		FROM sleep_stages st
		JOIN sleep_sessions s ON s.id = st.session_id
		WHERE s.user_id = $1 AND s.date >= $2::date AND s.date <= $3::date
		GROUP BY 1
		ORDER BY minutes DESC
	`, userID, start, end)
	if err != nil {
		h.logger.Warn().Err(err).Msg("query sleep stages")
		return
	}
	defer rows.Close()

	totals := make([]sleepStageTotal, 0, 4)
	for rows.Next() {
		var item sleepStageTotal
		if err := rows.Scan(&item.Stage, &item.Minutes, &item.Nights); err != nil {
			return
		}
		totals = append(totals, item)
	}
	sb.WriteString(formatSleepStages(totals))
}

// formatSleepStages averages each stage over the nights that reported it, not
// over the whole window: "awake" shows up on a fraction of the nights, and
// spreading it over all of them would quietly divide it away.
func formatSleepStages(totals []sleepStageTotal) string {
	if len(totals) == 0 {
		return "Фазы сна: нет данных за период\n"
	}

	overall := 0.0
	nights := 0
	for _, item := range totals {
		overall += item.Minutes
		if item.Nights > nights {
			nights = item.Nights
		}
	}
	if overall <= 0 || nights == 0 {
		return "Фазы сна: нет данных за период\n"
	}

	parts := make([]string, 0, len(totals))
	for _, item := range totals {
		if item.Nights == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %.0f мин за ночь (%.0f%% сна, ночей %d)",
			sleepStageLabel(item.Stage), item.Minutes/float64(item.Nights), item.Minutes/overall*100, item.Nights))
	}
	if len(parts) == 0 {
		return "Фазы сна: нет данных за период\n"
	}
	return "Фазы сна: " + strings.Join(parts, ", ") + "\n"
}

func sleepStageLabel(stage string) string {
	switch stage {
	case "deep":
		return "глубокий"
	case "light":
		return "лёгкий"
	case "rem":
		return "REM"
	case "awake":
		return "пробуждения"
	default:
		return stage
	}
}

// appendHeartRateDetail summarizes the continuous watch stream. Averaging it
// whole would drown the useful part, so the daily minimum and maximum travel
// with the mean, and SpO2 rides along as the other watch metric.
func (h *AIHandler) appendHeartRateDetail(ctx context.Context, sb *strings.Builder, userID string, start, end time.Time) {
	var avgHR, minHR, maxHR float64
	var samples, days int
	err := h.db.QueryRow(ctx, `
		SELECT COALESCE(AVG(value), 0), COALESCE(MIN(value), 0), COALESCE(MAX(value), 0),
		       COUNT(*), COUNT(DISTINCT DATE(timestamp))
		FROM biometrics
		WHERE user_id = $1 AND metric_type = 'heart_rate'
			AND timestamp >= $2 AND timestamp < $3
	`, userID, start, end).Scan(&avgHR, &minHR, &maxHR, &samples, &days)
	if err == nil && samples > 0 {
		sb.WriteString(fmt.Sprintf("Пульс за сутки: среднее %.0f bpm, диапазон %.0f-%.0f, замеров %d за %d дн.\n",
			avgHR, minHR, maxHR, samples, days))
	} else {
		sb.WriteString("Пульс за сутки: нет данных за период\n")
	}

	var avgSpO2, minSpO2 float64
	var spo2Count int
	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE(AVG(value), 0), COALESCE(MIN(value), 0), COUNT(*)
		FROM biometrics
		WHERE user_id = $1 AND metric_type = 'spo2'
			AND timestamp >= $2 AND timestamp < $3
	`, userID, start, end).Scan(&avgSpO2, &minSpO2, &spo2Count); err == nil && spo2Count > 0 {
		sb.WriteString(fmt.Sprintf("SpO2: среднее %.0f%%, минимум %.0f%%, замеров %d\n", avgSpO2, minSpO2, spo2Count))
	}
}

// bodyCompositionMetrics is the scale's output in the order a person reads it.
var bodyCompositionMetrics = []struct {
	metric string
	label  string
	unit   string
}{
	{"body_fat", "жир", "%"},
	{"muscle_mass", "мышцы", "кг"},
	{"skeletal_muscle_mass", "скелетные мышцы", "кг"},
	{"body_water", "вода", "%"},
	{"visceral_fat", "висцеральный жир", ""},
	{"bone_mass", "кости", "кг"},
	{"protein_mass", "белок", "кг"},
	{"bmi", "ИМТ", ""},
	{"metabolic_age", "метаболический возраст", "лет"},
}

// appendBodyComposition reports the first and last measurement of each metric,
// because a single number says nothing: the point of stepping on the scale
// repeatedly is the direction.
func (h *AIHandler) appendBodyComposition(ctx context.Context, sb *strings.Builder, userID string, start, end time.Time) {
	lines := make([]string, 0, len(bodyCompositionMetrics))
	for _, metric := range bodyCompositionMetrics {
		var first, last float64
		var measurements int
		err := h.db.QueryRow(ctx, `
			SELECT
				(ARRAY_AGG(value ORDER BY timestamp ASC))[1],
				(ARRAY_AGG(value ORDER BY timestamp DESC))[1],
				COUNT(*)
			FROM biometrics
			WHERE user_id = $1 AND metric_type = $2
				AND timestamp >= $3 AND timestamp < $4
		`, userID, metric.metric, start, end).Scan(&first, &last, &measurements)
		if err != nil {
			continue
		}
		if measurements == 0 {
			// Body composition moves slowly and the scale is not stepped on every
			// week, so a window with no measurement is not the same as no data:
			// the last known value still describes the body today.
			if line, ok := h.lastKnownBodyMetric(ctx, userID, metric.metric, metric.label, metric.unit, end); ok {
				lines = append(lines, line)
			}
			continue
		}

		unit := metric.unit
		if unit != "" && unit != "%" {
			unit = " " + unit
		}
		line := fmt.Sprintf("%s %.1f%s", metric.label, last, unit)
		if measurements > 1 {
			line += fmt.Sprintf(" (Δ %+.1f)", last-first)
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		sb.WriteString("Состав тела: нет данных\n")
		return
	}
	sb.WriteString("Состав тела (последнее значение, Δ за период; для метрик без замеров в периоде указан возраст замера): " + strings.Join(lines, ", ") + "\n")
}

// lastKnownBodyMetric falls back to the most recent measurement before the
// window, labelled with its age so the report never passes an old number off as
// a fresh one.
func (h *AIHandler) lastKnownBodyMetric(ctx context.Context, userID, metric, label, unit string, before time.Time) (string, bool) {
	var value float64
	var measuredAt time.Time
	err := h.db.QueryRow(ctx, `
		SELECT value, timestamp
		FROM biometrics
		WHERE user_id = $1 AND metric_type = $2 AND timestamp < $3
		ORDER BY timestamp DESC
		LIMIT 1
	`, userID, metric, before).Scan(&value, &measuredAt)
	if err != nil {
		return "", false
	}

	if unit != "" && unit != "%" {
		unit = " " + unit
	}
	return fmt.Sprintf("%s %.1f%s (замер %s назад)", label, value, unit, formatAIAge(before.Sub(measuredAt))), true
}

// walkingQualityMetrics are the gait numbers Apple Health records on their own.
var walkingQualityMetrics = []struct {
	metric string
	label  string
	unit   string
	sum    bool
}{
	{"walking_speed", "скорость ходьбы", "км/ч", false},
	{"walking_step_length", "длина шага", "см", false},
	{"walking_asymmetry_percentage", "асимметрия", "%", false},
	{"walking_double_support_percentage", "двойная опора", "%", false},
	{"flights_climbed", "этажей", "", true},
}

func (h *AIHandler) appendWalkingQuality(ctx context.Context, sb *strings.Builder, userID string, start, end time.Time) {
	lines := make([]string, 0, len(walkingQualityMetrics))
	for _, metric := range walkingQualityMetrics {
		var value float64
		var samples int
		query := `
			SELECT COALESCE(AVG(value), 0), COUNT(*)
			FROM biometrics
			WHERE user_id = $1 AND metric_type = $2 AND timestamp >= $3 AND timestamp < $4`
		if metric.sum {
			query = `
			SELECT COALESCE(SUM(value), 0), COUNT(*)
			FROM biometrics
			WHERE user_id = $1 AND metric_type = $2 AND timestamp >= $3 AND timestamp < $4`
		}
		if err := h.db.QueryRow(ctx, query, userID, metric.metric, start, end).Scan(&value, &samples); err != nil || samples == 0 {
			continue
		}

		unit := metric.unit
		if unit != "" && unit != "%" {
			unit = " " + unit
		}
		lines = append(lines, fmt.Sprintf("%s %.1f%s", metric.label, value, unit))
	}

	if len(lines) == 0 {
		return
	}
	sb.WriteString("Походка (среднее за период): " + strings.Join(lines, ", ") + "\n")
}
