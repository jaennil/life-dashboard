package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// aiRecoveryTrendDays is how much of the HRV series travels with the summary.
// Recovery is read as a direction rather than a number: one night above or
// below the baseline says nothing, a week of them says everything.
const aiRecoveryTrendDays = 7

// AIRecoveryReading is one metric over the window: where it stands now and
// where it has been.
type AIRecoveryReading struct {
	Latest  float64 `json:"latest"`
	Average float64 `json:"average"`
	Samples int     `json:"samples"`
}

type AIRecoveryDay struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// AIRecoveryData is what the band knows about how the body is coping. It is
// separate from the raw health numbers because these are verdicts rather than
// measurements: the watch has already compared them to the person's own normal.
type AIRecoveryData struct {
	Readings map[string]AIRecoveryReading `json:"readings"`
	HRVTrend []AIRecoveryDay              `json:"hrv_trend,omitempty"`
}

// aiRecoveryMetrics are read as a group. Each is either a verdict the band forms
// about the night or a measurement nothing else in this project reports.
var aiRecoveryMetrics = []string{
	"hrv", "hrv_baseline", "hrv_score",
	"readiness_score", "physical_score", "mental_score",
	"stress_avg", "stress_max", "stress_relaxed_share", "stress_high_share",
	"respiratory_rate", "oxygen_desaturation_index", "pai_total",
	"sleep_resting_heart_rate",
}

func (h *AIHandler) buildRecoveryData(ctx context.Context, userID string, start, end time.Time) (AIRecoveryData, error) {
	data := AIRecoveryData{Readings: map[string]AIRecoveryReading{}}

	rows, err := h.db.Query(ctx, `
		SELECT metric_type,
		       (ARRAY_AGG(value ORDER BY timestamp DESC))[1] AS latest,
		       AVG(value) AS average,
		       COUNT(*) AS samples
		FROM biometrics
		WHERE user_id = $1
		  AND timestamp >= $2 AND timestamp < $3
		  AND metric_type = ANY($4)
		GROUP BY metric_type
	`, userID, start, end, aiRecoveryMetrics)
	if err != nil {
		return data, fmt.Errorf("query recovery metrics: %w", err)
	}

	for rows.Next() {
		var metric string
		var reading AIRecoveryReading
		if err := rows.Scan(&metric, &reading.Latest, &reading.Average, &reading.Samples); err != nil {
			rows.Close()
			return data, err
		}
		data.Readings[metric] = reading
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return data, err
	}

	trend, err := h.loadHRVTrend(ctx, userID, start, end)
	if err != nil {
		return data, err
	}
	data.HRVTrend = trend
	return data, nil
}

func (h *AIHandler) loadHRVTrend(ctx context.Context, userID string, start, end time.Time) ([]AIRecoveryDay, error) {
	rows, err := h.db.Query(ctx, `
		SELECT to_char(timestamp AT TIME ZONE 'Europe/Moscow', 'DD.MM'), value
		FROM biometrics
		WHERE user_id = $1 AND metric_type = 'hrv'
		  AND timestamp >= $2 AND timestamp < $3
		ORDER BY timestamp DESC
		LIMIT $4
	`, userID, start, end, aiRecoveryTrendDays)
	if err != nil {
		return nil, fmt.Errorf("query hrv trend: %w", err)
	}
	defer rows.Close()

	days := make([]AIRecoveryDay, 0, aiRecoveryTrendDays)
	for rows.Next() {
		var day AIRecoveryDay
		if err := rows.Scan(&day.Date, &day.Value); err != nil {
			return nil, err
		}
		days = append(days, day)
	}
	return days, rows.Err()
}

// renderRecoveryText writes what the band concluded, in the order a person would
// ask: how recovered, on what evidence, and under how much strain.
func renderRecoveryText(data AIRecoveryData) string {
	read := func(metric string) (AIRecoveryReading, bool) {
		reading, ok := data.Readings[metric]
		return reading, ok && reading.Samples > 0
	}

	var sb strings.Builder
	if hrv, ok := read("hrv"); ok {
		sb.WriteString(fmt.Sprintf("HRV: последнее %.0f ms, среднее за период %.0f", hrv.Latest, hrv.Average))
		if baseline, ok := read("hrv_baseline"); ok {
			// The baseline is the person's own normal as the watch computed it, so
			// "above" and "below" mean something here that an absolute number does
			// not: healthy HRV differs several times over between people.
			sb.WriteString(fmt.Sprintf(", личная базовая линия %.0f (%s)",
				baseline.Latest, compareToBaseline(hrv.Latest, baseline.Latest)))
		}
		sb.WriteString("\n")
	}

	if len(data.HRVTrend) > 0 {
		parts := make([]string, 0, len(data.HRVTrend))
		for _, day := range data.HRVTrend {
			parts = append(parts, fmt.Sprintf("%s %.0f", day.Date, day.Value))
		}
		sb.WriteString("HRV по ночам (от свежей к старой): " + strings.Join(parts, ", ") + "\n")
	}

	if readiness, ok := read("readiness_score"); ok {
		sb.WriteString(fmt.Sprintf("Готовность: %.0f из 100, среднее за период %.0f", readiness.Latest, readiness.Average))
		physical, hasPhysical := read("physical_score")
		mental, hasMental := read("mental_score")
		switch {
		case hasPhysical && hasMental:
			sb.WriteString(fmt.Sprintf(" (физическая %.0f, ментальная %.0f)", physical.Latest, mental.Latest))
		case hasPhysical:
			sb.WriteString(fmt.Sprintf(" (физическая %.0f)", physical.Latest))
		}
		sb.WriteString("\n")
	}

	if stress, ok := read("stress_avg"); ok {
		sb.WriteString(fmt.Sprintf("Стресс: средний за период %.0f", stress.Average))
		if peak, ok := read("stress_max"); ok {
			sb.WriteString(fmt.Sprintf(", пиковый %.0f", peak.Latest))
		}
		if relaxed, ok := read("stress_relaxed_share"); ok {
			sb.WriteString(fmt.Sprintf(", в расслабленной зоне %.0f%% времени", relaxed.Average))
		}
		if high, ok := read("stress_high_share"); ok && high.Average > 0 {
			sb.WriteString(fmt.Sprintf(", в высокой %.0f%%", high.Average))
		}
		sb.WriteString("\n")
	}

	if breathing, ok := read("respiratory_rate"); ok {
		sb.WriteString(fmt.Sprintf("Частота дыхания: %.0f вдохов в минуту\n", breathing.Average))
	}
	if odi, ok := read("oxygen_desaturation_index"); ok {
		// Not a saturation percentage: this counts how often oxygen dipped during
		// the night, which is a breathing-quality signal rather than a level.
		sb.WriteString(fmt.Sprintf("Эпизоды снижения кислорода во сне: %.1f в час\n", odi.Average))
	}
	if pai, ok := read("pai_total"); ok {
		sb.WriteString(fmt.Sprintf("PAI: %.0f при цели 100 (баллы за нагрузку на сердце за последние 7 дней)\n", pai.Latest))
	}
	if rhr, ok := read("sleep_resting_heart_rate"); ok {
		sb.WriteString(fmt.Sprintf("Пульс покоя во сне: %.0f, среднее за период %.0f\n", rhr.Latest, rhr.Average))
	}

	if sb.Len() == 0 {
		return "Восстановление: нет данных за период\n"
	}
	return "Восстановление (оценки браслета):\n" + sb.String()
}

// compareToBaseline says which side of normal a reading falls on. The margin
// keeps a one-millisecond difference from being reported as a change.
func compareToBaseline(value, baseline float64) string {
	if baseline <= 0 {
		return "базовая линия неизвестна"
	}
	switch difference := value - baseline; {
	case difference > baseline*0.05:
		return "выше своей нормы"
	case difference < -baseline*0.05:
		return "ниже своей нормы"
	default:
		return "на уровне своей нормы"
	}
}

func (h *AIHandler) appendRecoveryContext(ctx context.Context, sb *strings.Builder, userID string, start, end time.Time) {
	data, err := h.buildRecoveryData(ctx, userID, start, end)
	if err != nil {
		h.logger.Warn().Err(err).Str("user_id", userID).Msg("build recovery context")
		return
	}
	sb.WriteString(renderRecoveryText(data))
}
