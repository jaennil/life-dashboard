package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
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
	AvgCalories        float64           `json:"avg_calories"`
	AvgProtein         float64           `json:"avg_protein"`
	AvgCarbs           float64           `json:"avg_carbs"`
	AvgFat             float64           `json:"avg_fat"`
	AvgWaterML         float64           `json:"avg_water_ml"`
	AvgHydrationML     float64           `json:"avg_hydration_ml"`
	DaysTracked        int               `json:"days_tracked"`
	TodayKcal          float64           `json:"today_kcal"`
	TodayWater         float64           `json:"today_water_ml"`
	TodayHydrationML   float64           `json:"today_hydration_ml"`
	TodayCountedDrinks float64           `json:"today_counted_drinks_ml"`
	TodayOtherDrinksML float64           `json:"today_other_drinks_ml"`
	HydrationMode      string            `json:"hydration_mode"`
	Targets            *NutritionTargets `json:"targets,omitempty"`
}

type NutritionGoldenCard struct {
	Key    string `json:"key"`
	Title  string `json:"title"`
	Value  string `json:"value"`
	Detail string `json:"detail"`
	Tone   string `json:"tone"`
}

type NutritionGoldenMetrics struct {
	Cards []NutritionGoldenCard `json:"cards"`
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
	Date            string                       `json:"date"`
	Calories        float64                      `json:"calories"`
	Protein         float64                      `json:"protein"`
	Carbs           float64                      `json:"carbs"`
	Fat             float64                      `json:"fat"`
	Fiber           float64                      `json:"fiber"`
	WaterML         float64                      `json:"water_ml"`
	HydrationML     float64                      `json:"hydration_ml"`
	CountedDrinksML float64                      `json:"counted_drinks_ml"`
	OtherDrinksML   float64                      `json:"other_drinks_ml"`
	Beverages       []NutritionHydrationBeverage `json:"beverages"`
	Meals           []NutritionMeal              `json:"meals"`
}

type SaveNutritionTargetsRequest struct {
	TargetWeightKg *float64 `json:"target_weight_kg"`
	TargetCalories *float64 `json:"target_calories"`
	TargetProteinG *float64 `json:"target_protein_g"`
	TargetCarbsG   *float64 `json:"target_carbs_g"`
	TargetFatG     *float64 `json:"target_fat_g"`
	TargetWaterML  *float64 `json:"target_water_ml"`
	HydrationMode  *string  `json:"hydration_mode"`
}

type SaveNutritionWaterRequest struct {
	Date    string   `json:"date"`
	DeltaML *float64 `json:"delta_ml"`
	WaterML *float64 `json:"water_ml"`
}

type SaveNutritionHydrationRequest struct {
	Date         string   `json:"date"`
	BeverageType string   `json:"beverage_type"`
	DeltaML      *float64 `json:"delta_ml"`
	AmountML     *float64 `json:"amount_ml"`
}

type nutritionGoldenDay struct {
	Calories      float64
	Protein       float64
	HydrationML   float64
	MealTypeCount int
}

func (h *NutritionHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	now := time.Now().In(aiDisplayLocation)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, aiDisplayLocation)
	sevenDaysAgo := today.AddDate(0, 0, -6)

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
		s.HydrationMode = normalizeHydrationMode(targets.HydrationMode)
	}
	if s.HydrationMode == "" {
		s.HydrationMode = hydrationModeStrict
	}

	hydrationRange, err := loadHydrationRange(ctx, h.db, userID, sevenDaysAgo, today, s.HydrationMode)
	if err != nil {
		h.logger.Warn().Err(err).Msg("load nutrition hydration summary")
	} else {
		hydrationDays := 0
		hydrationTotal := 0.0
		todayKey := today.Format("2006-01-02")
		for day, aggregate := range hydrationRange {
			if aggregate.HydrationML > 0 {
				hydrationDays++
				hydrationTotal += aggregate.HydrationML
			}
			if day == todayKey {
				s.TodayHydrationML = aggregate.HydrationML
				s.TodayCountedDrinks = aggregate.CountedDrinksML
				s.TodayOtherDrinksML = aggregate.OtherDrinksML
			}
		}
		if hydrationDays > 0 {
			s.AvgHydrationML = hydrationTotal / float64(hydrationDays)
		}
	}

	h.logger.Debug().Interface("summary", s).Msg("nutrition summary")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func (h *NutritionHandler) GetGoldenMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)
	windowDays := parseNutritionWindowDays(r, defaultNutritionWindowDays)
	now := time.Now().In(aiDisplayLocation)
	endDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, aiDisplayLocation)
	startDate := endDate.AddDate(0, 0, -(windowDays - 1))

	rows, err := h.db.Query(ctx, `
		SELECT
			TO_CHAR(d.date, 'YYYY-MM-DD'),
			COALESCE(d.calories_total, 0),
			COALESCE(d.protein_g, 0),
			COALESCE((
				SELECT COUNT(DISTINCT i.meal_type)
				FROM nutrition_items i
				WHERE i.daily_id = d.id
			), 0)
		FROM nutrition_daily d
		WHERE d.date >= $1 AND d.date <= $3 AND d.user_id = $2
	`, startDate, userID, endDate)
	if err != nil {
		h.logger.Error().Err(err).Msg("query nutrition golden days")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	dayMap := make(map[string]nutritionGoldenDay)
	for rows.Next() {
		var dayKey string
		var day nutritionGoldenDay
		if err := rows.Scan(&dayKey, &day.Calories, &day.Protein, &day.MealTypeCount); err != nil {
			h.logger.Warn().Err(err).Msg("scan nutrition golden day")
			continue
		}
		dayMap[dayKey] = day
	}
	if err := rows.Err(); err != nil {
		h.logger.Error().Err(err).Msg("iterate nutrition golden days")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	mealPresence := map[string]int{}
	presenceRows, err := h.db.Query(ctx, `
		SELECT i.meal_type, COUNT(DISTINCT i.daily_id)
		FROM nutrition_items i
		JOIN nutrition_daily d ON d.id = i.daily_id
		WHERE d.date >= $1 AND d.date <= $3 AND d.user_id = $2
		GROUP BY i.meal_type
	`, startDate, userID, endDate)
	if err != nil {
		h.logger.Error().Err(err).Msg("query nutrition meal presence")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer presenceRows.Close()

	for presenceRows.Next() {
		var mealType string
		var count int
		if err := presenceRows.Scan(&mealType, &count); err != nil {
			h.logger.Warn().Err(err).Msg("scan nutrition meal presence")
			continue
		}
		mealPresence[mealType] = count
	}
	if err := presenceRows.Err(); err != nil {
		h.logger.Error().Err(err).Msg("iterate nutrition meal presence")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	targets, err := loadNutritionTargets(ctx, h.db, userID)
	if err != nil {
		h.logger.Warn().Err(err).Msg("load nutrition golden targets")
	}
	hydrationMode := hydrationModeStrict
	if targets != nil {
		hydrationMode = normalizeHydrationMode(targets.HydrationMode)
	}
	hydrationRange, err := loadHydrationRange(ctx, h.db, userID, startDate, endDate, hydrationMode)
	if err != nil {
		h.logger.Warn().Err(err).Msg("load nutrition golden hydration")
	}

	days := make([]nutritionGoldenDay, 0, len(dayMap)+len(hydrationRange))
	for dayKey, day := range dayMap {
		if hydration, ok := hydrationRange[dayKey]; ok {
			day.HydrationML = hydration.HydrationML
		}
		days = append(days, day)
	}
	for dayKey, hydration := range hydrationRange {
		if _, ok := dayMap[dayKey]; ok {
			continue
		}
		days = append(days, nutritionGoldenDay{HydrationML: hydration.HydrationML})
	}

	resp := buildNutritionGoldenMetrics(windowDays, days, targets, mealPresence)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
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
				user_id, source, target_weight_kg, target_calories, target_protein_g, target_carbs_g, target_fat_g, target_water_ml, hydration_mode, synced_at, updated_at
			)
			VALUES ($1, 'manual', $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
			ON CONFLICT (user_id, source) DO UPDATE SET
				target_weight_kg = EXCLUDED.target_weight_kg,
				target_calories = EXCLUDED.target_calories,
				target_protein_g = EXCLUDED.target_protein_g,
				target_carbs_g = EXCLUDED.target_carbs_g,
				target_fat_g = EXCLUDED.target_fat_g,
				target_water_ml = EXCLUDED.target_water_ml,
				hydration_mode = EXCLUDED.hydration_mode,
				synced_at = NOW(),
				updated_at = NOW()
		`, userID, req.TargetWeightKg, req.TargetCalories, req.TargetProteinG, req.TargetCarbsG, req.TargetFatG, req.TargetWaterML, normalizedHydrationMode(req.HydrationMode)); err != nil {
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
	now := time.Now().In(aiDisplayLocation)
	endDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, aiDisplayLocation)
	startDate := endDate.AddDate(0, 0, -(windowDays - 1))

	targets, err := loadNutritionTargets(ctx, h.db, userID)
	if err != nil {
		h.logger.Warn().Err(err).Msg("load nutrition daily targets")
	}
	hydrationMode := hydrationModeStrict
	if targets != nil {
		hydrationMode = normalizeHydrationMode(targets.HydrationMode)
	}

	rows, err := h.db.Query(ctx, `
		SELECT id, TO_CHAR(date, 'YYYY-MM-DD'), COALESCE(calories_total, 0), COALESCE(protein_g, 0), COALESCE(carbs_g, 0), COALESCE(fat_g, 0), COALESCE(fiber_g, 0), COALESCE(water_ml, 0)
		FROM nutrition_daily
		WHERE date >= $1 AND date <= $3 AND user_id = $2
		ORDER BY date DESC
	`, startDate, userID, endDate)
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
	dayRows := make(map[string]*dayWithID)
	for rows.Next() {
		var d dayWithID
		if err := rows.Scan(&d.id, &d.Date, &d.Calories, &d.Protein, &d.Carbs, &d.Fat, &d.Fiber, &d.WaterML); err != nil {
			continue
		}
		d.Meals = []NutritionMeal{}
		d.Beverages = []NutritionHydrationBeverage{}
		dayRows[d.Date] = &d
	}
	rows.Close()

	hydrationRange, err := loadHydrationRange(ctx, h.db, userID, startDate, endDate, hydrationMode)
	if err != nil {
		h.logger.Warn().Err(err).Msg("load nutrition daily hydration")
	} else {
		for dayKey, aggregate := range hydrationRange {
			entry, ok := dayRows[dayKey]
			if !ok {
				entry = &dayWithID{
					NutritionDay: NutritionDay{
						Date:      dayKey,
						Meals:     []NutritionMeal{},
						Beverages: []NutritionHydrationBeverage{},
					},
				}
				dayRows[dayKey] = entry
			}
			entry.WaterML = aggregate.WaterML
			entry.HydrationML = aggregate.HydrationML
			entry.CountedDrinksML = aggregate.CountedDrinksML
			entry.OtherDrinksML = aggregate.OtherDrinksML
			entry.Beverages = aggregate.Beverages
		}
	}

	// Load meal items for each day
	for _, entry := range dayRows {
		if entry.id == "" {
			continue
		}
		itemRows, err := h.db.Query(ctx, `
			SELECT meal_type, food_name, serving_description, calories, COALESCE(macros, '{}')
			FROM nutrition_items
			WHERE daily_id = $1
			ORDER BY meal_type, calories DESC
		`, entry.id)
		if err != nil {
			h.logger.Warn().Err(err).Str("daily_id", entry.id).Msg("query nutrition items")
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
			entry.Meals = append(entry.Meals, *mealMap[mt])
		}
	}

	keys := make([]string, 0, len(dayRows))
	for dayKey := range dayRows {
		keys = append(keys, dayKey)
	}
	slices.SortFunc(keys, func(a, b string) int {
		switch {
		case a > b:
			return -1
		case a < b:
			return 1
		default:
			return 0
		}
	})

	result := make([]NutritionDay, 0, len(keys))
	for _, dayKey := range keys {
		result = append(result, dayRows[dayKey].NutritionDay)
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

	if req.WaterML != nil {
		err = h.db.QueryRow(ctx, `
			INSERT INTO nutrition_daily (user_id, date, water_ml, source)
			VALUES ($1, $2, $3, 'manual')
			ON CONFLICT (user_id, date) DO UPDATE SET
				water_ml = GREATEST(EXCLUDED.water_ml, 0)
			RETURNING COALESCE(water_ml, 0)
		`, userID, targetDate, *req.WaterML).Scan(new(float64))
	} else {
		err = h.db.QueryRow(ctx, `
			INSERT INTO nutrition_daily (user_id, date, water_ml, source)
			VALUES ($1, $2, GREATEST($3, 0), 'manual')
			ON CONFLICT (user_id, date) DO UPDATE SET
				water_ml = GREATEST(COALESCE(nutrition_daily.water_ml, 0) + EXCLUDED.water_ml, 0)
			RETURNING COALESCE(water_ml, 0)
		`, userID, targetDate, *req.DeltaML).Scan(new(float64))
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("save nutrition water")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	targets, err := loadNutritionTargets(ctx, h.db, userID)
	if err != nil {
		h.logger.Warn().Err(err).Msg("load hydration mode after water save")
	}
	hydrationMode := hydrationModeStrict
	if targets != nil {
		hydrationMode = normalizeHydrationMode(targets.HydrationMode)
	}
	state, err := loadHydrationStateForDate(ctx, h.db, userID, targetDate, hydrationMode)
	if err != nil {
		h.logger.Error().Err(err).Msg("load hydration state after water save")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

func (h *NutritionHandler) SaveHydration(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Context().Value(authmw.UserIDKey).(string)

	var req SaveNutritionHydrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := validateHydrationBeverageType(req.BeverageType); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.DeltaML == nil && req.AmountML == nil {
		http.Error(w, "delta_ml or amount_ml is required", http.StatusBadRequest)
		return
	}
	if req.DeltaML != nil && req.AmountML != nil {
		http.Error(w, "use either delta_ml or amount_ml", http.StatusBadRequest)
		return
	}
	if req.DeltaML != nil && *req.DeltaML <= 0 {
		http.Error(w, "delta_ml must be > 0", http.StatusBadRequest)
		return
	}
	if req.AmountML != nil && *req.AmountML < 0 {
		http.Error(w, "amount_ml must be >= 0", http.StatusBadRequest)
		return
	}

	targetDate, err := parseNutritionTargetDate(req.Date)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.AmountML != nil {
		if _, err := h.db.Exec(ctx, `
			INSERT INTO nutrition_hydration_entries (user_id, date, beverage_type, amount_ml)
			VALUES ($1, $2, $3, GREATEST($4, 0))
			ON CONFLICT (user_id, date, beverage_type) DO UPDATE SET
				amount_ml = GREATEST(EXCLUDED.amount_ml, 0),
				updated_at = NOW()
		`, userID, targetDate, normalizeHydrationBeverageType(req.BeverageType), *req.AmountML); err != nil {
			h.logger.Error().Err(err).Msg("set nutrition hydration amount")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	} else {
		if _, err := h.db.Exec(ctx, `
			INSERT INTO nutrition_hydration_entries (user_id, date, beverage_type, amount_ml)
			VALUES ($1, $2, $3, GREATEST($4, 0))
			ON CONFLICT (user_id, date, beverage_type) DO UPDATE SET
				amount_ml = GREATEST(COALESCE(nutrition_hydration_entries.amount_ml, 0) + EXCLUDED.amount_ml, 0),
				updated_at = NOW()
		`, userID, targetDate, normalizeHydrationBeverageType(req.BeverageType), *req.DeltaML); err != nil {
			h.logger.Error().Err(err).Msg("increment nutrition hydration amount")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	targets, err := loadNutritionTargets(ctx, h.db, userID)
	if err != nil {
		h.logger.Warn().Err(err).Msg("load hydration mode after hydration save")
	}
	hydrationMode := hydrationModeStrict
	if targets != nil {
		hydrationMode = normalizeHydrationMode(targets.HydrationMode)
	}
	state, err := loadHydrationStateForDate(ctx, h.db, userID, targetDate, hydrationMode)
	if err != nil {
		h.logger.Error().Err(err).Msg("load hydration state after hydration save")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

func buildNutritionGoldenMetrics(windowDays int, days []nutritionGoldenDay, targets *NutritionTargets, mealPresence map[string]int) NutritionGoldenMetrics {
	nutritionDays := make([]nutritionGoldenDay, 0, len(days))
	hydrationDays := make([]nutritionGoldenDay, 0, len(days))
	fullMealDays := 0
	totalMealTypes := 0

	for _, day := range days {
		if day.Calories > 0 || day.MealTypeCount > 0 {
			nutritionDays = append(nutritionDays, day)
			totalMealTypes += day.MealTypeCount
			if day.MealTypeCount >= 3 {
				fullMealDays++
			}
		}
		if day.HydrationML > 0 {
			hydrationDays = append(hydrationDays, day)
		}
	}

	loggedDays := len(nutritionDays)
	hydrationTrackedDays := len(hydrationDays)
	avgCalories := averageNutritionField(nutritionDays, func(day nutritionGoldenDay) float64 { return day.Calories })
	avgProtein := averageNutritionField(nutritionDays, func(day nutritionGoldenDay) float64 { return day.Protein })
	avgHydration := averageNutritionField(hydrationDays, func(day nutritionGoldenDay) float64 { return day.HydrationML })
	avgMealTypes := 0.0
	if loggedDays > 0 {
		avgMealTypes = float64(totalMealTypes) / float64(loggedDays)
	}

	calorieTarget := nutritionTargetValue(targets, func(t *NutritionTargets) *float64 { return t.TargetCalories })
	proteinTarget := nutritionTargetValue(targets, func(t *NutritionTargets) *float64 { return t.TargetProteinG })
	waterTarget := nutritionTargetValue(targets, func(t *NutritionTargets) *float64 { return t.TargetWaterML })
	hydrationMode := hydrationModeStrict
	if targets != nil {
		hydrationMode = normalizeHydrationMode(targets.HydrationMode)
	}

	calorieHitDays := 0
	if calorieTarget != nil && *calorieTarget > 0 {
		for _, day := range nutritionDays {
			if nutritionWithinBand(day.Calories, *calorieTarget, 0.10) {
				calorieHitDays++
			}
		}
	}

	proteinHitDays := 0
	if proteinTarget != nil && *proteinTarget > 0 {
		for _, day := range nutritionDays {
			if day.Protein >= *proteinTarget {
				proteinHitDays++
			}
		}
	}

	waterHitDays := 0
	if waterTarget != nil && *waterTarget > 0 {
		for _, day := range hydrationDays {
			if day.HydrationML >= *waterTarget {
				waterHitDays++
			}
		}
	}

	regimeTone := nutritionToneForMinimumRatio(float64(loggedDays), float64(windowDays), 0.85, 0.5)
	calorieTone := "muted"
	calorieValue := "—"
	calorieDetail := "нужна цель по калориям и хотя бы один залоганный день"
	if loggedDays > 0 {
		if calorieTarget != nil && *calorieTarget > 0 {
			avgDelta := avgCalories - *calorieTarget
			calorieTone = nutritionToneForDistance(avgDelta, *calorieTarget, 0.10, 0.20)
			calorieValue = formatNutritionSignedDelta(avgDelta, "ккал")
			calorieDetail = fmt.Sprintf("%d/%d дней в коридоре ±10%% от %d ккал", calorieHitDays, loggedDays, int(*calorieTarget))
		} else {
			calorieTone = nutritionToneForMinimumRatio(float64(loggedDays), float64(windowDays), 0.85, 0.5)
			calorieValue = fmt.Sprintf("%.0f ккал", avgCalories)
			calorieDetail = fmt.Sprintf("среднее за %d залоганных дней", loggedDays)
		}
	}

	proteinTone := "muted"
	proteinValue := "—"
	proteinDetail := "нужна цель по белку и залоганные дни"
	if loggedDays > 0 {
		if proteinTarget != nil && *proteinTarget > 0 {
			coverage := avgProtein / *proteinTarget
			proteinTone = nutritionToneForMinimumRatio(avgProtein, *proteinTarget, 0.95, 0.75)
			proteinValue = fmt.Sprintf("%.0f%% цели", coverage*100)
			proteinDetail = fmt.Sprintf("%d/%d дней ≥ %.0f г", proteinHitDays, loggedDays, *proteinTarget)
		} else {
			proteinTone = nutritionToneForMinimumRatio(float64(loggedDays), float64(windowDays), 0.85, 0.5)
			proteinValue = fmt.Sprintf("%.0f г", avgProtein)
			proteinDetail = fmt.Sprintf("среднее за %d залоганных дней", loggedDays)
		}
	}

	waterTone := "muted"
	waterValue := "—"
	waterDetail := "цель воды не задана или вода ещё не логировалась"
	if hydrationTrackedDays > 0 {
		if waterTarget != nil && *waterTarget > 0 {
			coverage := avgHydration / *waterTarget
			waterTone = nutritionToneForMinimumRatio(avgHydration, *waterTarget, 0.95, 0.65)
			waterValue = fmt.Sprintf("%.0f%% цели", coverage*100)
			waterDetail = fmt.Sprintf("%d/%d дней закрыто ≥ %.0f мл · %s", waterHitDays, hydrationTrackedDays, *waterTarget, hydrationModeDescription(hydrationMode))
		} else {
			waterTone = nutritionToneForMinimumRatio(float64(hydrationTrackedDays), float64(windowDays), 0.7, 0.35)
			waterValue = fmt.Sprintf("%.0f мл", avgHydration)
			waterDetail = fmt.Sprintf("гидратация есть в %d/%d дней периода · %s", hydrationTrackedDays, windowDays, hydrationModeDescription(hydrationMode))
		}
	}

	structureTone := "muted"
	structureValue := "—"
	structureDetail := "нет данных по приёмам пищи"
	if loggedDays > 0 {
		structureTone = nutritionToneForMinimumRatio(avgMealTypes, 3, 1, 0.66)
		structureValue = fmt.Sprintf("%.1f приёма", avgMealTypes)
		structureDetail = fmt.Sprintf("З %d · О %d · У %d · %d полных дней", mealPresence["breakfast"], mealPresence["lunch"], mealPresence["dinner"], fullMealDays)
	}

	return NutritionGoldenMetrics{
		Cards: []NutritionGoldenCard{
			{
				Key:    "consistency",
				Title:  "Режим",
				Value:  fmt.Sprintf("%d/%d дн", loggedDays, windowDays),
				Detail: fmt.Sprintf("%d дней с полным логом · гидратация %d/%d", fullMealDays, hydrationTrackedDays, windowDays),
				Tone:   regimeTone,
			},
			{
				Key:    "calories",
				Title:  "Калории",
				Value:  calorieValue,
				Detail: calorieDetail,
				Tone:   calorieTone,
			},
			{
				Key:    "protein",
				Title:  "Белок",
				Value:  proteinValue,
				Detail: proteinDetail,
				Tone:   proteinTone,
			},
			{
				Key:    "hydration",
				Title:  "Гидратация",
				Value:  waterValue,
				Detail: waterDetail,
				Tone:   waterTone,
			},
			{
				Key:    "structure",
				Title:  "Приёмы",
				Value:  structureValue,
				Detail: structureDetail,
				Tone:   structureTone,
			},
		},
	}
}

func averageNutritionField(days []nutritionGoldenDay, pick func(nutritionGoldenDay) float64) float64 {
	if len(days) == 0 {
		return 0
	}
	total := 0.0
	for _, day := range days {
		total += pick(day)
	}
	return total / float64(len(days))
}

func nutritionTargetValue(targets *NutritionTargets, pick func(*NutritionTargets) *float64) *float64 {
	if targets == nil {
		return nil
	}
	return pick(targets)
}

func nutritionWithinBand(value, target, band float64) bool {
	if target <= 0 {
		return false
	}
	delta := value - target
	if delta < 0 {
		delta = -delta
	}
	return delta/target <= band
}

func nutritionToneForMinimumRatio(value, target, successRatio, warningRatio float64) string {
	if target <= 0 {
		return "muted"
	}
	ratio := value / target
	switch {
	case ratio >= successRatio:
		return "success"
	case ratio >= warningRatio:
		return "warning"
	default:
		return "danger"
	}
}

func nutritionToneForDistance(delta, target, successRatio, warningRatio float64) string {
	if target <= 0 {
		return "muted"
	}
	absDelta := delta
	if absDelta < 0 {
		absDelta = -absDelta
	}
	ratio := absDelta / target
	switch {
	case ratio <= successRatio:
		return "success"
	case ratio <= warningRatio:
		return "warning"
	default:
		return "danger"
	}
}

func formatNutritionSignedDelta(value float64, unit string) string {
	if value > -5 && value < 5 {
		return "в цель"
	}
	if value > 0 {
		return fmt.Sprintf("+%.0f %s", value, unit)
	}
	return fmt.Sprintf("−%.0f %s", -value, unit)
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
		req.TargetWaterML == nil &&
		req.HydrationMode == nil
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
	if req.HydrationMode != nil {
		if err := validateHydrationMode(*req.HydrationMode); err != nil {
			return err
		}
	}
	return nil
}

func normalizedHydrationMode(value *string) *string {
	if value == nil {
		return nil
	}
	mode := normalizeHydrationMode(*value)
	return &mode
}
