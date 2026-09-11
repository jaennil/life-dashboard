package handlers

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	// Enough to show what the diet is made of without pasting the whole diary.
	aiNutritionTopFoodLimit    = 10
	aiNutritionPerMealLimit    = 4
	aiNutritionRecentDayLimit  = 3
	aiNutritionRecentItemLimit = 40
)

// AINutritionFood is one food as it appears across the window: what it cost in
// calories and how regular it is.
type AINutritionFood struct {
	Name     string  `json:"name"`
	Calories float64 `json:"calories"`
	Entries  int     `json:"entries"`
	Days     int     `json:"days"`
	Share    float64 `json:"share_pct,omitempty"`
}

// AINutritionMealPattern answers "what does this person usually eat for lunch".
type AINutritionMealPattern struct {
	Meal  string            `json:"meal"`
	Foods []AINutritionFood `json:"foods"`
}

type AINutritionLoggedItem struct {
	Meal     string  `json:"meal,omitempty"`
	Name     string  `json:"name"`
	Calories float64 `json:"calories,omitempty"`
	Serving  string  `json:"serving,omitempty"`
}

type AINutritionDayMeals struct {
	Date  string                  `json:"date"`
	Items []AINutritionLoggedItem `json:"items"`
}

// AINutritionFoods is the composition of the diet, as opposed to its totals.
// Macros alone cannot answer why protein is low or where the sugar comes from.
type AINutritionFoods struct {
	TopByCalories  []AINutritionFood        `json:"top_by_calories,omitempty"`
	TopByFrequency []AINutritionFood        `json:"top_by_frequency,omitempty"`
	MealPatterns   []AINutritionMealPattern `json:"meal_patterns,omitempty"`
	RecentDays     []AINutritionDayMeals    `json:"recent_days,omitempty"`
}

func (f AINutritionFoods) empty() bool {
	return len(f.TopByCalories) == 0 && len(f.TopByFrequency) == 0 &&
		len(f.MealPatterns) == 0 && len(f.RecentDays) == 0
}

// loadNutritionFoods reads the individual diary entries behind the daily totals.
func (h *AIHandler) loadNutritionFoods(ctx context.Context, userID string, start, end time.Time) (AINutritionFoods, error) {
	data := AINutritionFoods{}
	from := start.Format("2006-01-02")
	to := end.Format("2006-01-02")

	rows, err := h.db.Query(ctx, `
		SELECT COALESCE(NULLIF(TRIM(ni.food_name), ''), 'без названия') AS name,
		       COALESCE(SUM(ni.calories), 0) AS calories,
		       COUNT(*) AS entries,
		       COUNT(DISTINCT nd.date) AS days
		FROM nutrition_items ni
		JOIN nutrition_daily nd ON nd.id = ni.daily_id
		WHERE nd.user_id = $1 AND nd.date >= $2::date AND nd.date <= $3::date
		GROUP BY 1
	`, userID, from, to)
	if err != nil {
		return data, fmt.Errorf("query logged foods: %w", err)
	}

	foods := make([]AINutritionFood, 0, 64)
	totalCalories := 0.0
	for rows.Next() {
		var food AINutritionFood
		if err := rows.Scan(&food.Name, &food.Calories, &food.Entries, &food.Days); err != nil {
			rows.Close()
			return data, err
		}
		totalCalories += food.Calories
		foods = append(foods, food)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return data, err
	}
	if len(foods) == 0 {
		return data, nil
	}

	for i := range foods {
		if totalCalories > 0 {
			foods[i].Share = roundTo(foods[i].Calories/totalCalories*100, 0)
		}
	}

	byCalories := append([]AINutritionFood(nil), foods...)
	sort.Slice(byCalories, func(i, j int) bool {
		if byCalories[i].Calories != byCalories[j].Calories {
			return byCalories[i].Calories > byCalories[j].Calories
		}
		return byCalories[i].Name < byCalories[j].Name
	})
	data.TopByCalories = byCalories[:min(len(byCalories), aiNutritionTopFoodLimit)]

	// Frequency is a different question from calories: a daily coffee moves the
	// routine, a single restaurant dinner moves the total.
	byFrequency := append([]AINutritionFood(nil), foods...)
	sort.Slice(byFrequency, func(i, j int) bool {
		if byFrequency[i].Days != byFrequency[j].Days {
			return byFrequency[i].Days > byFrequency[j].Days
		}
		if byFrequency[i].Entries != byFrequency[j].Entries {
			return byFrequency[i].Entries > byFrequency[j].Entries
		}
		return byFrequency[i].Name < byFrequency[j].Name
	})
	data.TopByFrequency = byFrequency[:min(len(byFrequency), aiNutritionTopFoodLimit)]

	if data.MealPatterns, err = h.loadNutritionMealPatterns(ctx, userID, from, to); err != nil {
		return data, err
	}
	if data.RecentDays, err = h.loadNutritionRecentMeals(ctx, userID, from, to); err != nil {
		return data, err
	}
	return data, nil
}

func (h *AIHandler) loadNutritionMealPatterns(ctx context.Context, userID, from, to string) ([]AINutritionMealPattern, error) {
	rows, err := h.db.Query(ctx, `
		SELECT COALESCE(NULLIF(TRIM(ni.meal_type), ''), '') AS meal,
		       COALESCE(NULLIF(TRIM(ni.food_name), ''), 'без названия') AS name,
		       COALESCE(SUM(ni.calories), 0) AS calories,
		       COUNT(*) AS entries,
		       COUNT(DISTINCT nd.date) AS days
		FROM nutrition_items ni
		JOIN nutrition_daily nd ON nd.id = ni.daily_id
		WHERE nd.user_id = $1 AND nd.date >= $2::date AND nd.date <= $3::date
			AND NULLIF(TRIM(ni.meal_type), '') IS NOT NULL
		GROUP BY 1, 2
		ORDER BY meal, days DESC, entries DESC, calories DESC
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query meal patterns: %w", err)
	}
	defer rows.Close()

	byMeal := map[string][]AINutritionFood{}
	order := make([]string, 0, 4)
	for rows.Next() {
		var meal string
		var food AINutritionFood
		if err := rows.Scan(&meal, &food.Name, &food.Calories, &food.Entries, &food.Days); err != nil {
			return nil, err
		}
		if _, seen := byMeal[meal]; !seen {
			order = append(order, meal)
		}
		if len(byMeal[meal]) < aiNutritionPerMealLimit {
			byMeal[meal] = append(byMeal[meal], food)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	patterns := make([]AINutritionMealPattern, 0, len(order))
	for _, meal := range order {
		patterns = append(patterns, AINutritionMealPattern{Meal: meal, Foods: byMeal[meal]})
	}
	// Alphabetical meal_type would print dinner before lunch.
	sort.SliceStable(patterns, func(i, j int) bool {
		return mealTypeRank(patterns[i].Meal) < mealTypeRank(patterns[j].Meal)
	})
	return patterns, nil
}

// mealTypeRank puts the meals in the order of the day they happen in.
func mealTypeRank(mealType string) int {
	switch strings.ToLower(strings.TrimSpace(mealType)) {
	case "breakfast":
		return 0
	case "morning snack":
		return 1
	case "lunch":
		return 2
	case "afternoon snack":
		return 3
	case "snack", "snacks":
		return 4
	case "dinner", "supper":
		return 5
	case "evening snack":
		return 6
	case "other":
		return 8
	default:
		return 7
	}
}

// loadNutritionRecentMeals keeps the last few days item by item, which is what
// makes a comment about yesterday specific instead of statistical.
func (h *AIHandler) loadNutritionRecentMeals(ctx context.Context, userID, from, to string) ([]AINutritionDayMeals, error) {
	rows, err := h.db.Query(ctx, `
		WITH recent AS (
			SELECT id, date
			FROM nutrition_daily
			WHERE user_id = $1 AND date >= $2::date AND date <= $3::date
			ORDER BY date DESC
			LIMIT $4
		)
		SELECT recent.date,
		       COALESCE(NULLIF(TRIM(ni.meal_type), ''), '') AS meal,
		       COALESCE(NULLIF(TRIM(ni.food_name), ''), 'без названия') AS name,
		       COALESCE(ni.calories, 0) AS calories,
		       COALESCE(NULLIF(TRIM(ni.serving_description), ''), '') AS serving
		FROM recent
		JOIN nutrition_items ni ON ni.daily_id = recent.id
		ORDER BY recent.date DESC, meal, calories DESC
		LIMIT $5
	`, userID, from, to, aiNutritionRecentDayLimit, aiNutritionRecentItemLimit)
	if err != nil {
		return nil, fmt.Errorf("query recent meals: %w", err)
	}
	defer rows.Close()

	days := make([]AINutritionDayMeals, 0, aiNutritionRecentDayLimit)
	index := map[string]int{}
	for rows.Next() {
		var date time.Time
		var item AINutritionLoggedItem
		if err := rows.Scan(&date, &item.Meal, &item.Name, &item.Calories, &item.Serving); err != nil {
			return nil, err
		}
		item.Serving = formatServingSize(item.Serving)

		key := date.Format("2006-01-02")
		position, ok := index[key]
		if !ok {
			days = append(days, AINutritionDayMeals{Date: key})
			position = len(days) - 1
			index[key] = position
		}
		days[position].Items = append(days[position].Items, item)
	}
	return days, rows.Err()
}

// mealPatternHeading keeps the "other" bucket from reading as a meal.
func mealPatternHeading(mealType string) string {
	label := aiMealTypeLabel(mealType)
	if label == "другое" {
		return "Прочие записи"
	}
	return "Обычно на " + label
}

// FatSecret servings arrive as raw diary strings such as
// "3.2 :custom:320s  g Милти Филе Индейки", where the only part worth reading is
// the portion itself. servingAmountPattern picks the last weight or volume in
// the string, which is the resolved portion when the entry also carries a
// multiplier.
var (
	servingAmountPattern = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)s?\s*(g|ml|г|мл)\b`)
	servingCountPattern  = regexp.MustCompile(`(?i)^(\d+(?:[.,]\d+)?)\s*serving`)
)

// formatServingSize reduces a diary serving string to the portion size, and to
// nothing at all when it carries none: a junk string costs tokens and teaches
// the model to quote junk back.
func formatServingSize(serving string) string {
	serving = strings.TrimSpace(serving)
	if serving == "" {
		return ""
	}

	if matches := servingAmountPattern.FindAllStringSubmatch(serving, -1); len(matches) > 0 {
		last := matches[len(matches)-1]
		unit := "г"
		if strings.EqualFold(last[2], "ml") || strings.EqualFold(last[2], "мл") {
			unit = "мл"
		}
		return strings.Replace(last[1], ",", ".", 1) + " " + unit
	}

	if match := servingCountPattern.FindStringSubmatch(serving); match != nil {
		return strings.Replace(match[1], ",", ".", 1) + " порц."
	}
	return ""
}

// renderNutritionFoodsText writes the composition in the same plain shape the
// rest of the nutrition section uses.
func renderNutritionFoodsText(data AINutritionFoods) string {
	if data.empty() {
		return "Состав рациона: в логах нет отдельных продуктов за период\n"
	}

	var sb strings.Builder

	if len(data.TopByCalories) > 0 {
		sb.WriteString("Больше всего калорий дали:\n")
		for _, food := range data.TopByCalories {
			sb.WriteString(fmt.Sprintf("  - %s: %.0f ккал", food.Name, food.Calories))
			if food.Share > 0 {
				sb.WriteString(fmt.Sprintf(", %.0f%% залогированных калорий", food.Share))
			}
			sb.WriteString(fmt.Sprintf(", записей %d за %d дн.\n", food.Entries, food.Days))
		}
	}

	if len(data.TopByFrequency) > 0 {
		sb.WriteString("Чаще всего в рационе:\n")
		for _, food := range data.TopByFrequency {
			sb.WriteString(fmt.Sprintf("  - %s: %d дн. из логов, записей %d, всего %.0f ккал\n",
				food.Name, food.Days, food.Entries, food.Calories))
		}
	}

	for _, pattern := range data.MealPatterns {
		if len(pattern.Foods) == 0 {
			continue
		}
		names := make([]string, 0, len(pattern.Foods))
		for _, food := range pattern.Foods {
			names = append(names, fmt.Sprintf("%s (%d дн., %.0f ккал)", food.Name, food.Days, food.Calories))
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", mealPatternHeading(pattern.Meal), strings.Join(names, ", ")))
	}

	for _, day := range data.RecentDays {
		if len(day.Items) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("Съедено %s:\n", formatISODateShort(day.Date)))
		for _, item := range day.Items {
			sb.WriteString("  - " + item.Name)
			if item.Meal != "" {
				sb.WriteString(" (" + aiMealTypeLabel(item.Meal) + ")")
			}
			if item.Serving != "" {
				sb.WriteString(", " + item.Serving)
			}
			if item.Calories > 0 {
				sb.WriteString(fmt.Sprintf(", %.0f ккал", item.Calories))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
