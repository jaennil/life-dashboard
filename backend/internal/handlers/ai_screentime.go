package handlers

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

// aiScreenTimeItemLimit bounds the app and website lists. The tail of a Screen
// Time day is dozens of items with seconds on them, which costs tokens and says
// nothing.
const aiScreenTimeItemLimit = 8

// AIScreenTimeOverviewData is what the model gets about phone use: the totals,
// the trend against the previous window of the same length, and what the time
// actually went into.
type AIScreenTimeOverviewData struct {
	Source        string             `json:"source"`
	Start         string             `json:"start"`
	End           string             `json:"end"`
	DaysWithData  int                `json:"days_with_data"`
	PartialDays   int                `json:"partial_days"`
	AppHours      float64            `json:"app_hours"`
	WebsiteHours  float64            `json:"website_hours"`
	DailyAvgHours float64            `json:"daily_avg_hours"`
	PrevDailyAvg  *float64           `json:"prev_daily_avg_hours,omitempty"`
	BusiestDay    *AIScreenTimeDay   `json:"busiest_day,omitempty"`
	QuietestDay   *AIScreenTimeDay   `json:"quietest_day,omitempty"`
	TopApps       []AIScreenTimeItem `json:"top_apps"`
	TopWebsites   []AIScreenTimeItem `json:"top_websites"`
	DailySeries   []AIScreenTimeDay  `json:"daily_series,omitempty"`
}

type AIScreenTimeDay struct {
	Date  string  `json:"date"`
	Hours float64 `json:"hours"`
}

type AIScreenTimeItem struct {
	Name  string  `json:"name"`
	Hours float64 `json:"hours"`
	Share float64 `json:"share_pct"`
	Days  int     `json:"days"`
}

func (h *AIHandler) buildScreenTimeOverviewInRange(ctx context.Context, userID string, start, end time.Time) (AIScreenTimeOverviewData, error) {
	data := AIScreenTimeOverviewData{
		Source: "screen_time_daily + screen_time_app_usage",
		Start:  start.Format("2006-01-02"),
		End:    end.Format("2006-01-02"),
	}

	from := start.Format("2006-01-02")
	to := end.Format("2006-01-02")

	var appSeconds, websiteSeconds int
	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(app_seconds), 0), COALESCE(SUM(website_seconds), 0),
		       COUNT(*), COUNT(*) FILTER (WHERE is_partial)
		FROM screen_time_daily
		WHERE user_id = $1 AND day BETWEEN $2::date AND $3::date
	`, userID, from, to).Scan(&appSeconds, &websiteSeconds, &data.DaysWithData, &data.PartialDays); err != nil {
		return data, fmt.Errorf("query screen time totals: %w", err)
	}
	if data.DaysWithData == 0 {
		return data, nil
	}

	data.AppHours = hoursFromSeconds(appSeconds)
	data.WebsiteHours = hoursFromSeconds(websiteSeconds)
	data.DailyAvgHours = roundTo(data.AppHours/float64(data.DaysWithData), 1)

	// The window immediately before this one, same length, so "more or less than
	// usual" is answerable without the model guessing at a baseline.
	windowDays := int(end.Sub(start).Hours()/24) + 1
	if windowDays > 0 {
		prevEnd := start.AddDate(0, 0, -1)
		prevStart := prevEnd.AddDate(0, 0, -(windowDays - 1))
		var prevSeconds, prevDays int
		if err := h.db.QueryRow(ctx, `
			SELECT COALESCE(SUM(app_seconds), 0), COUNT(*)
			FROM screen_time_daily
			WHERE user_id = $1 AND day BETWEEN $2::date AND $3::date
		`, userID, prevStart.Format("2006-01-02"), prevEnd.Format("2006-01-02")).Scan(&prevSeconds, &prevDays); err == nil && prevDays > 0 {
			average := roundTo(hoursFromSeconds(prevSeconds)/float64(prevDays), 1)
			data.PrevDailyAvg = &average
		}
	}

	rows, err := h.db.Query(ctx, `
		SELECT day, app_seconds
		FROM screen_time_daily
		WHERE user_id = $1 AND day BETWEEN $2::date AND $3::date
		ORDER BY day
	`, userID, from, to)
	if err != nil {
		return data, fmt.Errorf("query screen time days: %w", err)
	}
	for rows.Next() {
		var day time.Time
		var seconds int
		if err := rows.Scan(&day, &seconds); err != nil {
			rows.Close()
			return data, err
		}
		entry := AIScreenTimeDay{Date: day.Format("2006-01-02"), Hours: hoursFromSeconds(seconds)}
		data.DailySeries = append(data.DailySeries, entry)
		if data.BusiestDay == nil || entry.Hours > data.BusiestDay.Hours {
			busiest := entry
			data.BusiestDay = &busiest
		}
		if data.QuietestDay == nil || entry.Hours < data.QuietestDay.Hours {
			quietest := entry
			data.QuietestDay = &quietest
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return data, err
	}

	if data.TopApps, err = h.topScreenTimeItems(ctx, userID, from, to, "app", data.AppHours); err != nil {
		return data, err
	}
	if data.TopWebsites, err = h.topScreenTimeItems(ctx, userID, from, to, "website", data.WebsiteHours); err != nil {
		return data, err
	}

	return data, nil
}

func (h *AIHandler) topScreenTimeItems(ctx context.Context, userID, from, to, kind string, totalHours float64) ([]AIScreenTimeItem, error) {
	rows, err := h.db.Query(ctx, `
		SELECT COALESCE(NULLIF(display_name, ''), item_key) AS name,
		       SUM(seconds) AS seconds,
		       COUNT(DISTINCT day) AS days
		FROM screen_time_app_usage
		WHERE user_id = $1 AND kind = $4 AND day BETWEEN $2::date AND $3::date
		GROUP BY 1
		ORDER BY seconds DESC
		LIMIT $5
	`, userID, from, to, kind, aiScreenTimeItemLimit)
	if err != nil {
		return nil, fmt.Errorf("query top screen time %s: %w", kind, err)
	}
	defer rows.Close()

	items := make([]AIScreenTimeItem, 0, aiScreenTimeItemLimit)
	for rows.Next() {
		var item AIScreenTimeItem
		var seconds int
		if err := rows.Scan(&item.Name, &seconds, &item.Days); err != nil {
			return nil, err
		}
		item.Hours = hoursFromSeconds(seconds)
		if totalHours > 0 {
			item.Share = roundTo(item.Hours/totalHours*100, 0)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// renderScreenTimeOverviewText spells out the two traps in this data: website
// time is already inside the browser's own app total, and the app total is
// below what iOS itself shows because system screens are not reported.
func renderScreenTimeOverviewText(title string, data AIScreenTimeOverviewData) string {
	var sb strings.Builder
	sb.WriteString(title + "\n")

	if data.DaysWithData == 0 {
		sb.WriteString("Нет данных экранного времени за период.\n")
		return sb.String()
	}

	sb.WriteString("Источник: iOS Screen Time через ярлык. Время сайтов уже входит в время браузера, складывать их нельзя. Общая сумма ниже той, что показывает сам iOS: домашний экран, библиотека приложений и переключатель приложений не отдаются ярлыком.\n")

	trend := ""
	if data.PrevDailyAvg != nil {
		delta := roundTo(data.DailyAvgHours-*data.PrevDailyAvg, 1)
		switch {
		case delta > 0:
			trend = fmt.Sprintf(" | пред. период %.1f ч/день (+%.1f)", *data.PrevDailyAvg, delta)
		case delta < 0:
			trend = fmt.Sprintf(" | пред. период %.1f ч/день (%.1f)", *data.PrevDailyAvg, delta)
		default:
			trend = fmt.Sprintf(" | пред. период %.1f ч/день (без изменений)", *data.PrevDailyAvg)
		}
	}
	sb.WriteString(fmt.Sprintf("Дней с данными: %d | всего в приложениях: %.1f ч | в среднем %.1f ч/день%s\n",
		data.DaysWithData, data.AppHours, data.DailyAvgHours, trend))
	sb.WriteString(fmt.Sprintf("Из них в сайтах через браузер: %.1f ч\n", data.WebsiteHours))

	if data.PartialDays > 0 {
		sb.WriteString(fmt.Sprintf("Незакрытых дней (выгрузка в середине дня): %d - по ним время занижено.\n", data.PartialDays))
	}
	if data.BusiestDay != nil && data.QuietestDay != nil {
		sb.WriteString(fmt.Sprintf("Максимум: %s - %.1f ч | минимум: %s - %.1f ч\n",
			formatAIDate(data.BusiestDay.Date), data.BusiestDay.Hours,
			formatAIDate(data.QuietestDay.Date), data.QuietestDay.Hours))
	}

	writeScreenTimeItems(&sb, "Приложения", data.TopApps)
	writeScreenTimeItems(&sb, "Сайты", data.TopWebsites)

	return sb.String()
}

func writeScreenTimeItems(sb *strings.Builder, title string, items []AIScreenTimeItem) {
	if len(items) == 0 {
		return
	}
	sb.WriteString(title + ":\n")
	for _, item := range items {
		sb.WriteString(fmt.Sprintf("  - %s: %.1f ч", item.Name, item.Hours))
		if item.Share > 0 {
			sb.WriteString(fmt.Sprintf(", %.0f%%", item.Share))
		}
		if item.Days > 0 {
			sb.WriteString(fmt.Sprintf(", дней с использованием %d", item.Days))
		}
		sb.WriteString("\n")
	}
}

func formatAIDate(value string) string {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return parsed.Format("02.01")
}

func hoursFromSeconds(seconds int) float64 {
	return roundTo(float64(seconds)/3600, 1)
}

func roundTo(value float64, digits int) float64 {
	factor := math.Pow(10, float64(digits))
	return math.Round(value*factor) / factor
}
