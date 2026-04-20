package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type AINutritionOverviewData struct {
	Days        int               `json:"days"`
	Targets     *NutritionTargets `json:"targets,omitempty"`
	TrackedDays int               `json:"tracked_days"`
	AvgCalories float64           `json:"avg_calories"`
	AvgProteinG float64           `json:"avg_protein_g"`
	AvgCarbsG   float64           `json:"avg_carbs_g"`
	AvgFatG     float64           `json:"avg_fat_g"`
	RecentDays  []AINutritionDay  `json:"recent_days"`
}

type AINutritionDay struct {
	Date      string  `json:"date"`
	Calories  float64 `json:"calories"`
	ProteinG  float64 `json:"protein_g"`
	CarbsG    float64 `json:"carbs_g"`
	FatG      float64 `json:"fat_g"`
	FiberG    float64 `json:"fiber_g,omitempty"`
}

func (h *AIHandler) buildNutritionOverviewData(ctx context.Context, userID string, days int) (AINutritionOverviewData, error) {
	since := time.Now().AddDate(0, 0, -days)
	data := AINutritionOverviewData{Days: days}

	targets, err := loadNutritionTargets(ctx, h.db, userID)
	if err != nil {
		return data, err
	}
	data.Targets = targets

	if err := h.db.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COALESCE(AVG(calories_total), 0),
			COALESCE(AVG(protein_g), 0),
			COALESCE(AVG(carbs_g), 0),
			COALESCE(AVG(fat_g), 0)
		FROM nutrition_daily
		WHERE user_id = $1
			AND date >= $2::date
	`, userID, since).Scan(&data.TrackedDays, &data.AvgCalories, &data.AvgProteinG, &data.AvgCarbsG, &data.AvgFatG); err != nil {
		return data, err
	}

	rows, err := h.db.Query(ctx, `
		SELECT date, calories_total, protein_g, carbs_g, fat_g, fiber_g
		FROM nutrition_daily
		WHERE user_id = $1
			AND date >= $2::date
		ORDER BY date DESC
		LIMIT 5
	`, userID, since)
	if err != nil {
		return data, err
	}
	defer rows.Close()

	for rows.Next() {
		var day AINutritionDay
		var date time.Time
		if err := rows.Scan(&date, &day.Calories, &day.ProteinG, &day.CarbsG, &day.FatG, &day.FiberG); err != nil {
			return data, err
		}
		day.Date = date.Format("2006-01-02")
		data.RecentDays = append(data.RecentDays, day)
	}
	if err := rows.Err(); err != nil {
		return data, err
	}

	return data, nil
}

func renderNutritionOverviewText(title string, data AINutritionOverviewData) string {
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(title))
	sb.WriteString("\n")
	sb.WriteString("Только залогированные приёмы пищи. Неполный лог не означает, что других приёмов пищи не было.\n")
	sb.WriteString(renderNutritionTargetsForAI(data.Targets))
	sb.WriteString(fmt.Sprintf("Дней с логами: %d, среднее %.0f ккал | Б %.0f г | Ж %.0f г | У %.0f г\n",
		data.TrackedDays, data.AvgCalories, data.AvgProteinG, data.AvgFatG, data.AvgCarbsG))

	if len(data.RecentDays) == 0 {
		sb.WriteString("  Нет логов питания за период\n")
		return sb.String()
	}

	for _, day := range data.RecentDays {
		sb.WriteString(fmt.Sprintf("  %s: %.0f ккал | Б %.0f г | Ж %.0f г | У %.0f г\n",
			formatISODateShort(day.Date), day.Calories, day.ProteinG, day.FatG, day.CarbsG))
	}
	return sb.String()
}

func formatISODateShort(value string) string {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return date.Format("02.01")
}
