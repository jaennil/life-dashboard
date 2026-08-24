package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// aiDailySeriesDefaultDays is the lookback for the paired series. It is much
// longer than a checkup window on purpose: a week of data cannot show whether
// short sleep travels with higher spending, and the model is being asked for
// exactly that kind of relationship.
const aiDailySeriesDefaultDays = 90

// AIDailySeriesRow is one calendar day with every metric that has a daily
// cadence, lined up on the same row. The point is the alignment: the rest of the
// checkup context arrives as separate per-domain sections, which lets a model
// describe each domain but never compare them day by day.
//
// Pointers rather than zero values throughout, because "no reading that day"
// and "a reading of zero" mean different things and collapsing them would
// manufacture relationships that are not there.
type AIDailySeriesRow struct {
	Date          string   `json:"date"`
	SleepMinutes  *int     `json:"sleep_minutes,omitempty"`
	SleepScore    *int     `json:"sleep_score,omitempty"`
	RestingHR     *int     `json:"resting_hr,omitempty"`
	Steps         *int     `json:"steps,omitempty"`
	WeightKg      *float64 `json:"weight_kg,omitempty"`
	Calories      *int     `json:"calories,omitempty"`
	ProteinG      *int     `json:"protein_g,omitempty"`
	SpentRub      *float64 `json:"spent_rub,omitempty"`
	Workouts      *int     `json:"workouts,omitempty"`
	HabitsDone    *int     `json:"habits_done,omitempty"`
	TasksDone     *int     `json:"tasks_done,omitempty"`
	ScreenMinutes *int     `json:"screen_minutes,omitempty"`
}

// AIDailySeriesData is the tool payload. Coverage carries how many days actually
// hold a value per column, which is what stops a model from reporting a
// relationship it derived from three points.
type AIDailySeriesData struct {
	Days     int                `json:"days"`
	From     string             `json:"from"`
	To       string             `json:"to"`
	Coverage map[string]int     `json:"days_with_data"`
	Rows     []AIDailySeriesRow `json:"rows"`
}

// buildDailySeries lines up one row per calendar day over the lookback. Every
// domain is aggregated to a day in its own CTE first, so a domain with several
// rows per day - two weigh-ins, several transactions - cannot multiply the rows
// of every other domain through the joins.
func (h *AIHandler) buildDailySeries(ctx context.Context, userID string, days int) (AIDailySeriesData, error) {
	if days <= 0 {
		days = aiDailySeriesDefaultDays
	}
	end := time.Now().In(aiDisplayLocation)
	start := end.AddDate(0, 0, -(days - 1))
	from := start.Format("2006-01-02")
	to := end.Format("2006-01-02")

	data := AIDailySeriesData{
		Days:     days,
		From:     from,
		To:       to,
		Coverage: map[string]int{},
		Rows:     make([]AIDailySeriesRow, 0, days),
	}

	rows, err := h.db.Query(ctx, `
		WITH calendar AS (
			SELECT d::date AS day
			FROM generate_series($2::date, $3::date, interval '1 day') d
		),
		sleep AS (
			SELECT date AS day,
			       MAX(total_sleep_minutes) AS minutes,
			       MAX(sleep_score)         AS score,
			       MAX(avg_resting_hr)      AS resting_hr
			FROM sleep_sessions
			WHERE user_id = $1 AND date BETWEEN $2::date AND $3::date
			GROUP BY 1
		),
		body AS (
			SELECT DATE(timestamp) AS day,
			       MAX(value) FILTER (WHERE metric_type = 'steps')  AS steps,
			       AVG(value) FILTER (WHERE metric_type = 'weight')  AS weight
			FROM biometrics
			WHERE user_id = $1
			  AND metric_type IN ('steps', 'weight')
			  AND DATE(timestamp) BETWEEN $2::date AND $3::date
			GROUP BY 1
		),
		food AS (
			SELECT date AS day,
			       MAX(calories_total) AS calories,
			       MAX(protein_g)      AS protein
			FROM nutrition_daily
			WHERE user_id = $1 AND date BETWEEN $2::date AND $3::date
			GROUP BY 1
		),
		money AS (
			SELECT DATE(occurred_at) AS day, SUM(-amount) AS spent
			FROM transactions
			WHERE user_id = $1 AND amount < 0 AND is_transfer = false
			  AND DATE(occurred_at) BETWEEN $2::date AND $3::date
			GROUP BY 1
		),
		gym AS (
			SELECT DATE(started_at) AS day, COUNT(*) AS workouts
			FROM workouts
			WHERE user_id = $1 AND DATE(started_at) BETWEEN $2::date AND $3::date
			GROUP BY 1
		),
		habits AS (
			SELECT s.target_date AS day,
			       COUNT(*) FILTER (WHERE s.status = 'completed') AS done
			FROM habit_daily_statuses s
			JOIN habits hb ON hb.id = s.habit_id
			WHERE hb.user_id = $1 AND s.target_date BETWEEN $2::date AND $3::date
			GROUP BY 1
		),
		tasks AS (
			SELECT DATE(completed_at) AS day, COUNT(*) AS done
			FROM todoist_task_completions
			WHERE user_id = $1 AND DATE(completed_at) BETWEEN $2::date AND $3::date
			GROUP BY 1
		),
		screen AS (
			SELECT day, MAX(app_seconds) AS app_seconds
			FROM screen_time_daily
			WHERE user_id = $1 AND day BETWEEN $2::date AND $3::date
			GROUP BY 1
		)
		SELECT c.day,
		       sleep.minutes    AS sleep_minutes,
		       sleep.score      AS sleep_score,
		       sleep.resting_hr AS resting_hr,
		       body.steps       AS steps,
		       body.weight      AS weight_kg,
		       food.calories    AS calories,
		       food.protein     AS protein_g,
		       money.spent      AS spent_rub,
		       gym.workouts     AS workouts,
		       habits.done      AS habits_done,
		       tasks.done       AS tasks_done,
		       screen.app_seconds AS app_seconds
		FROM calendar c
		LEFT JOIN sleep  ON sleep.day  = c.day
		LEFT JOIN body   ON body.day   = c.day
		LEFT JOIN food   ON food.day   = c.day
		LEFT JOIN money  ON money.day  = c.day
		LEFT JOIN gym    ON gym.day    = c.day
		LEFT JOIN habits ON habits.day = c.day
		LEFT JOIN tasks  ON tasks.day  = c.day
		LEFT JOIN screen ON screen.day = c.day
		ORDER BY c.day
	`, userID, from, to)
	if err != nil {
		return data, fmt.Errorf("query daily series: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		// Column types differ across these tables: sleep minutes, score and
		// resting HR are integers, while biometrics.value, both nutrition columns
		// and the money sum are numeric, and the counts come back as bigint.
		var (
			day                             time.Time
			sleepMinutes, sleepScore        *int
			restingHR                       *int
			steps, weight                   *float64
			calories, protein               *float64
			spent                           *float64
			workouts, habitsDone, tasksDone *int64
			appSeconds                      *int64
		)
		if err := rows.Scan(&day, &sleepMinutes, &sleepScore, &restingHR,
			&steps, &weight, &calories, &protein, &spent,
			&workouts, &habitsDone, &tasksDone, &appSeconds); err != nil {
			return data, fmt.Errorf("scan daily series: %w", err)
		}

		row := AIDailySeriesRow{
			Date:          day.Format("2006-01-02"),
			SleepMinutes:  sleepMinutes,
			SleepScore:    sleepScore,
			RestingHR:     restingHR,
			Steps:         roundedInt(steps),
			WeightKg:      roundedFloat(weight, 1),
			Calories:      roundedInt(calories),
			ProteinG:      roundedInt(protein),
			SpentRub:      roundedFloat(spent, 0),
			Workouts:      countValue(workouts),
			HabitsDone:    countValue(habitsDone),
			TasksDone:     countValue(tasksDone),
			ScreenMinutes: minutesFromSeconds(appSeconds),
		}
		data.Rows = append(data.Rows, row)
		countCoverage(data.Coverage, row)
	}
	if err := rows.Err(); err != nil {
		return data, fmt.Errorf("iterate daily series: %w", err)
	}
	return data, nil
}

// countCoverage tallies days that actually carry a value, per column.
func countCoverage(coverage map[string]int, row AIDailySeriesRow) {
	bump := func(name string, present bool) {
		if present {
			coverage[name]++
		}
	}
	bump("sleep_minutes", row.SleepMinutes != nil)
	bump("sleep_score", row.SleepScore != nil)
	bump("resting_hr", row.RestingHR != nil)
	bump("steps", row.Steps != nil)
	bump("weight_kg", row.WeightKg != nil)
	bump("calories", row.Calories != nil)
	bump("protein_g", row.ProteinG != nil)
	bump("spent_rub", row.SpentRub != nil)
	bump("workouts", row.Workouts != nil)
	bump("habits_done", row.HabitsDone != nil)
	bump("tasks_done", row.TasksDone != nil)
	bump("screen_minutes", row.ScreenMinutes != nil)
}

var aiDailySeriesColumns = []string{
	"date", "sleep_min", "sleep_score", "rest_hr", "steps", "weight_kg",
	"kcal", "protein_g", "spent_rub", "workouts", "habits_done", "tasks_done",
	"screen_min",
}

// renderDailySeriesText writes the series as a fixed-column table. A table costs
// far fewer tokens than repeating field names on every one of ninety rows, and
// an empty cell reads as "no data" rather than as a zero.
func renderDailySeriesText(data AIDailySeriesData) string {
	var sb strings.Builder
	sb.WriteString("=== СВОДНЫЙ ДНЕВНОЙ РЯД ===\n")
	sb.WriteString(fmt.Sprintf("Период: %s — %s (%d дней). Пустая клетка означает отсутствие данных, а не ноль.\n",
		data.From, data.To, data.Days))

	sb.WriteString("Дней с данными по столбцам: ")
	parts := make([]string, 0, len(data.Coverage))
	for _, name := range []string{"sleep_minutes", "sleep_score", "resting_hr", "steps",
		"weight_kg", "calories", "protein_g", "spent_rub", "workouts", "habits_done",
		"tasks_done", "screen_minutes"} {
		parts = append(parts, fmt.Sprintf("%s=%d", name, data.Coverage[name]))
	}
	sb.WriteString(strings.Join(parts, ", "))
	sb.WriteString("\n\n")

	sb.WriteString(strings.Join(aiDailySeriesColumns, "\t"))
	sb.WriteString("\n")
	for _, row := range data.Rows {
		cells := []string{
			row.Date,
			intCell(row.SleepMinutes),
			intCell(row.SleepScore),
			intCell(row.RestingHR),
			intCell(row.Steps),
			floatCell(row.WeightKg, 1),
			intCell(row.Calories),
			intCell(row.ProteinG),
			floatCell(row.SpentRub, 0),
			intCell(row.Workouts),
			intCell(row.HabitsDone),
			intCell(row.TasksDone),
			intCell(row.ScreenMinutes),
		}
		sb.WriteString(strings.Join(cells, "\t"))
		sb.WriteString("\n")
	}
	return sb.String()
}

func intCell(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func floatCell(value *float64, decimals int) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', decimals, 64)
}

func roundedInt(value *float64) *int {
	if value == nil {
		return nil
	}
	rounded := int(*value + 0.5)
	return &rounded
}

func roundedFloat(value *float64, decimals int) *float64 {
	if value == nil {
		return nil
	}
	factor := 1.0
	for i := 0; i < decimals; i++ {
		factor *= 10
	}
	rounded := float64(int64(*value*factor+0.5)) / factor
	return &rounded
}

// countValue keeps a count as a value only when the day produced a row at all,
// so a day with no workout data stays empty instead of claiming zero workouts.
func countValue(value *int64) *int {
	if value == nil {
		return nil
	}
	converted := int(*value)
	return &converted
}

func minutesFromSeconds(seconds *int64) *int {
	if seconds == nil {
		return nil
	}
	minutes := int((*seconds + 30) / 60)
	return &minutes
}
