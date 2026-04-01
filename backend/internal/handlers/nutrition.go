package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	authmw "life-dashboard/internal/middleware"
)

type NutritionHandler struct {
	db     *pgxpool.Pool
	logger zerolog.Logger
}

func NewNutrition(db *pgxpool.Pool, logger zerolog.Logger) *NutritionHandler {
	return &NutritionHandler{db: db, logger: logger.With().Str("handler", "nutrition").Logger()}
}

type NutritionSummary struct {
	AvgCalories float64 `json:"avg_calories"`
	AvgProtein  float64 `json:"avg_protein"`
	AvgCarbs    float64 `json:"avg_carbs"`
	AvgFat      float64 `json:"avg_fat"`
	DaysTracked int     `json:"days_tracked"`
	TodayKcal   float64 `json:"today_kcal"`
}

type NutritionMealItem struct {
	FoodName string                 `json:"food_name"`
	Serving  string                 `json:"serving"`
	Calories float64                `json:"calories"`
	Macros   map[string]float64     `json:"macros,omitempty"`
}

type NutritionMeal struct {
	MealType string              `json:"meal_type"`
	Items    []NutritionMealItem `json:"items"`
}

type NutritionDay struct {
	Date     string          `json:"date"`
	Calories float64         `json:"calories"`
	Protein  float64         `json:"protein"`
	Carbs    float64         `json:"carbs"`
	Fat      float64         `json:"fat"`
	Fiber    float64         `json:"fiber"`
	Meals    []NutritionMeal `json:"meals"`
}

func (h *NutritionHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	now := time.Now()
	sevenDaysAgo := now.AddDate(0, 0, -7)
	today := now.Truncate(24 * time.Hour)

	var s NutritionSummary
	h.db.QueryRow(ctx, `
		SELECT
			COALESCE(AVG(calories_total), 0),
			COALESCE(AVG(protein_g), 0),
			COALESCE(AVG(carbs_g), 0),
			COALESCE(AVG(fat_g), 0),
			COUNT(*)
		FROM nutrition_daily
		WHERE date >= $1 AND user_id = $2
	`, sevenDaysAgo, userID).Scan(&s.AvgCalories, &s.AvgProtein, &s.AvgCarbs, &s.AvgFat, &s.DaysTracked)

	h.db.QueryRow(ctx, `
		SELECT COALESCE(calories_total, 0) FROM nutrition_daily WHERE date = $1 AND user_id = $2
	`, today, userID).Scan(&s.TodayKcal)

	h.logger.Debug().Interface("summary", s).Msg("nutrition summary")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func (h *NutritionHandler) GetDaily(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	fourteenDaysAgo := time.Now().AddDate(0, 0, -14)

	rows, err := h.db.Query(ctx, `
		SELECT id, TO_CHAR(date, 'YYYY-MM-DD'), calories_total, protein_g, carbs_g, fat_g, fiber_g
		FROM nutrition_daily
		WHERE date >= $1 AND user_id = $2
		ORDER BY date DESC
	`, fourteenDaysAgo, userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("query nutrition daily")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type dayWithID struct {
		NutritionDay
		id string
	}
	days := make([]dayWithID, 0)
	for rows.Next() {
		var d dayWithID
		if err := rows.Scan(&d.id, &d.Date, &d.Calories, &d.Protein, &d.Carbs, &d.Fat, &d.Fiber); err != nil {
			continue
		}
		d.Meals = []NutritionMeal{}
		days = append(days, d)
	}
	rows.Close()

	// Load meal items for each day
	for i := range days {
		itemRows, err := h.db.Query(ctx, `
			SELECT meal_type, food_name, serving_description, calories, COALESCE(macros, '{}')
			FROM nutrition_items
			WHERE daily_id = $1
			ORDER BY meal_type, calories DESC
		`, days[i].id)
		if err != nil {
			h.logger.Warn().Err(err).Str("daily_id", days[i].id).Msg("query nutrition items")
			continue
		}

		mealMap := make(map[string]*NutritionMeal)
		mealOrder := []string{}
		for itemRows.Next() {
			var mealType, foodName, serving string
			var calories float64
			var macrosJSON []byte
			if err := itemRows.Scan(&mealType, &foodName, &serving, &calories, &macrosJSON); err != nil {
				continue
			}
			var macros map[string]float64
			json.Unmarshal(macrosJSON, &macros)
			if _, ok := mealMap[mealType]; !ok {
				mealMap[mealType] = &NutritionMeal{MealType: mealType, Items: []NutritionMealItem{}}
				mealOrder = append(mealOrder, mealType)
			}
			mealMap[mealType].Items = append(mealMap[mealType].Items, NutritionMealItem{
				FoodName: foodName,
				Serving:  serving,
				Calories: calories,
				Macros:   macros,
			})
		}
		itemRows.Close()

		for _, mt := range mealOrder {
			days[i].Meals = append(days[i].Meals, *mealMap[mt])
		}
	}

	result := make([]NutritionDay, len(days))
	for i, d := range days {
		result[i] = d.NutritionDay
	}

	h.logger.Debug().Int("days", len(result)).Msg("nutrition daily")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
