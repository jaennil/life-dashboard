package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	authmw "life-dashboard/internal/middleware"
)

type NutritionHandler struct {
	db     *pgxpool.Pool
	logger zerolog.Logger
}

const (
	defaultNutritionWindowDays = 14
	maxNutritionWindowDays     = 90
)

func NewNutrition(db *pgxpool.Pool, logger zerolog.Logger) *NutritionHandler {
	return &NutritionHandler{db: db, logger: logger.With().Str("handler", "nutrition").Logger()}
}

type NutritionSummary struct {
	AvgCalories float64           `json:"avg_calories"`
	AvgProtein  float64           `json:"avg_protein"`
	AvgCarbs    float64           `json:"avg_carbs"`
	AvgFat      float64           `json:"avg_fat"`
	AvgWaterML  float64           `json:"avg_water_ml"`
	DaysTracked int               `json:"days_tracked"`
	TodayKcal   float64           `json:"today_kcal"`
	TodayWater  float64           `json:"today_water_ml"`
	Targets     *NutritionTargets `json:"targets,omitempty"`
}

type NutritionMealItem struct {
	FoodName string             `json:"food_name"`
	Serving  string             `json:"serving"`
	Calories float64            `json:"calories"`
	Macros   map[string]float64 `json:"macros,omitempty"`
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
	WaterML  float64         `json:"water_ml"`
	Meals    []NutritionMeal `json:"meals"`
}

type SaveNutritionTargetsRequest struct {
	TargetWeightKg *float64 `json:"target_weight_kg"`
	TargetCalories *float64 `json:"target_calories"`
	TargetProteinG *float64 `json:"target_protein_g"`
	TargetCarbsG   *float64 `json:"target_carbs_g"`
	TargetFatG     *float64 `json:"target_fat_g"`
	TargetWaterML  *float64 `json:"target_water_ml"`
}

type SaveNutritionWaterRequest struct {
	Date    string   `json:"date"`
	DeltaML *float64 `json:"delta_ml"`
	WaterML *float64 `json:"water_ml"`
}

type NutritionWaterState struct {
	Date    string  `json:"date"`
	WaterML float64 `json:"water_ml"`
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
			COALESCE(AVG(water_ml) FILTER (WHERE water_ml IS NOT NULL AND water_ml > 0), 0),
			COUNT(*) FILTER (WHERE calories_total IS NOT NULL)
		FROM nutrition_daily
		WHERE date >= $1 AND user_id = $2
	`, sevenDaysAgo, userID).Scan(&s.AvgCalories, &s.AvgProtein, &s.AvgCarbs, &s.AvgFat, &s.AvgWaterML, &s.DaysTracked)

	h.db.QueryRow(ctx, `
		SELECT COALESCE(calories_total, 0), COALESCE(water_ml, 0)
		FROM nutrition_daily WHERE date = $1 AND user_id = $2
	`, today, userID).Scan(&s.TodayKcal, &s.TodayWater)

	targets, err := loadNutritionTargets(ctx, h.db, userID)
	if err != nil {
		h.logger.Warn().Err(err).Msg("load nutrition targets")
	} else {
		s.Targets = targets
	}

	h.logger.Debug().Interface("summary", s).Msg("nutrition summary")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func (h *NutritionHandler) SaveTargets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)

	var req SaveNutritionTargetsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := validateNutritionTargetsRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if isEmptyNutritionTargetsRequest(req) {
		if _, err := h.db.Exec(ctx, `DELETE FROM nutrition_targets WHERE user_id = $1 AND source = 'manual'`, userID); err != nil {
			h.logger.Error().Err(err).Msg("delete manual nutrition targets")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	} else {
		if _, err := h.db.Exec(ctx, `
			INSERT INTO nutrition_targets (
				user_id, source, target_weight_kg, target_calories, target_protein_g, target_carbs_g, target_fat_g, target_water_ml, synced_at, updated_at
			)
			VALUES ($1, 'manual', $2, $3, $4, $5, $6, $7, NOW(), NOW())
			ON CONFLICT (user_id, source) DO UPDATE SET
				target_weight_kg = EXCLUDED.target_weight_kg,
				target_calories = EXCLUDED.target_calories,
				target_protein_g = EXCLUDED.target_protein_g,
				target_carbs_g = EXCLUDED.target_carbs_g,
				target_fat_g = EXCLUDED.target_fat_g,
				target_water_ml = EXCLUDED.target_water_ml,
				synced_at = NOW(),
				updated_at = NOW()
		`, userID, req.TargetWeightKg, req.TargetCalories, req.TargetProteinG, req.TargetCarbsG, req.TargetFatG, req.TargetWaterML); err != nil {
			h.logger.Error().Err(err).Msg("save manual nutrition targets")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	targets, err := loadNutritionTargets(ctx, h.db, userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("reload nutrition targets")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(targets)
}

func (h *NutritionHandler) GetDaily(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	windowDays := parseNutritionWindowDays(r, defaultNutritionWindowDays)
	startDate := time.Now().AddDate(0, 0, -(windowDays - 1))

	rows, err := h.db.Query(ctx, `
		SELECT id, TO_CHAR(date, 'YYYY-MM-DD'), COALESCE(calories_total, 0), COALESCE(protein_g, 0), COALESCE(carbs_g, 0), COALESCE(fat_g, 0), COALESCE(fiber_g, 0), COALESCE(water_ml, 0)
		FROM nutrition_daily
		WHERE date >= $1 AND user_id = $2
		ORDER BY date DESC
	`, startDate, userID)
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
	dayRows := make([]dayWithID, 0)
	for rows.Next() {
		var d dayWithID
		if err := rows.Scan(&d.id, &d.Date, &d.Calories, &d.Protein, &d.Carbs, &d.Fat, &d.Fiber, &d.WaterML); err != nil {
			continue
		}
		d.Meals = []NutritionMeal{}
		dayRows = append(dayRows, d)
	}
	rows.Close()

	// Load meal items for each day
	for i := range dayRows {
		itemRows, err := h.db.Query(ctx, `
			SELECT meal_type, food_name, serving_description, calories, COALESCE(macros, '{}')
			FROM nutrition_items
			WHERE daily_id = $1
			ORDER BY meal_type, calories DESC
		`, dayRows[i].id)
		if err != nil {
			h.logger.Warn().Err(err).Str("daily_id", dayRows[i].id).Msg("query nutrition items")
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
			dayRows[i].Meals = append(dayRows[i].Meals, *mealMap[mt])
		}
	}

	result := make([]NutritionDay, len(dayRows))
	for i, d := range dayRows {
		result[i] = d.NutritionDay
	}

	h.logger.Debug().Int("days", len(result)).Msg("nutrition daily")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *NutritionHandler) SaveWater(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)

	var req SaveNutritionWaterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.DeltaML == nil && req.WaterML == nil {
		http.Error(w, "delta_ml or water_ml is required", http.StatusBadRequest)
		return
	}
	if req.DeltaML != nil && req.WaterML != nil {
		http.Error(w, "use either delta_ml or water_ml", http.StatusBadRequest)
		return
	}
	if req.DeltaML != nil && *req.DeltaML <= 0 {
		http.Error(w, "delta_ml must be > 0", http.StatusBadRequest)
		return
	}
	if req.WaterML != nil && *req.WaterML < 0 {
		http.Error(w, "water_ml must be >= 0", http.StatusBadRequest)
		return
	}

	targetDate, err := parseNutritionTargetDate(req.Date)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var currentWater float64
	if req.WaterML != nil {
		err = h.db.QueryRow(ctx, `
			INSERT INTO nutrition_daily (user_id, date, water_ml, source)
			VALUES ($1, $2, $3, 'manual')
			ON CONFLICT (user_id, date) DO UPDATE SET
				water_ml = GREATEST(EXCLUDED.water_ml, 0)
			RETURNING COALESCE(water_ml, 0)
		`, userID, targetDate, *req.WaterML).Scan(&currentWater)
	} else {
		err = h.db.QueryRow(ctx, `
			INSERT INTO nutrition_daily (user_id, date, water_ml, source)
			VALUES ($1, $2, GREATEST($3, 0), 'manual')
			ON CONFLICT (user_id, date) DO UPDATE SET
				water_ml = GREATEST(COALESCE(nutrition_daily.water_ml, 0) + EXCLUDED.water_ml, 0)
			RETURNING COALESCE(water_ml, 0)
		`, userID, targetDate, *req.DeltaML).Scan(&currentWater)
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("save nutrition water")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(NutritionWaterState{
		Date:    targetDate.Format("2006-01-02"),
		WaterML: currentWater,
	})
}

func parseNutritionWindowDays(r *http.Request, fallback int) int {
	raw := r.URL.Query().Get("days")
	if raw == "" {
		return fallback
	}

	days, err := strconv.Atoi(raw)
	if err != nil || days <= 0 {
		return fallback
	}
	if days > maxNutritionWindowDays {
		return maxNutritionWindowDays
	}
	return days
}

func parseNutritionTargetDate(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		now := time.Now().In(aiDisplayLocation)
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, aiDisplayLocation), nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(value), aiDisplayLocation)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date")
	}
	return parsed, nil
}

func isEmptyNutritionTargetsRequest(req SaveNutritionTargetsRequest) bool {
	return req.TargetWeightKg == nil &&
		req.TargetCalories == nil &&
		req.TargetProteinG == nil &&
		req.TargetCarbsG == nil &&
		req.TargetFatG == nil &&
		req.TargetWaterML == nil
}

func validateNutritionTargetsRequest(req SaveNutritionTargetsRequest) error {
	values := map[string]*float64{
		"target_weight_kg": req.TargetWeightKg,
		"target_calories":  req.TargetCalories,
		"target_protein_g": req.TargetProteinG,
		"target_carbs_g":   req.TargetCarbsG,
		"target_fat_g":     req.TargetFatG,
		"target_water_ml":  req.TargetWaterML,
	}
	for name, value := range values {
		if value != nil && *value < 0 {
			return fmt.Errorf("%s must be >= 0", name)
		}
	}
	return nil
}
