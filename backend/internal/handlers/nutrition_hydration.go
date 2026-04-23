package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	hydrationModeStrict   = "strict"
	hydrationModeFlexible = "flexible"

	hydrationBeverageTea       = "tea"
	hydrationBeverageCoffee    = "coffee"
	hydrationBeverageEnergy    = "energy"
	hydrationBeverageMilkshake = "milkshake"
	hydrationBeverageOther     = "other"
)

var hydrationBeverageOrder = []string{
	hydrationBeverageTea,
	hydrationBeverageCoffee,
	hydrationBeverageEnergy,
	hydrationBeverageMilkshake,
	hydrationBeverageOther,
}

type NutritionHydrationBeverage struct {
	BeverageType     string  `json:"beverage_type"`
	AmountML         float64 `json:"amount_ml"`
	CountsTowardGoal bool    `json:"counts_toward_goal"`
}

type NutritionHydrationState struct {
	Date            string                       `json:"date"`
	WaterML         float64                      `json:"water_ml"`
	HydrationML     float64                      `json:"hydration_ml"`
	CountedDrinksML float64                      `json:"counted_drinks_ml"`
	OtherDrinksML   float64                      `json:"other_drinks_ml"`
	HydrationMode   string                       `json:"hydration_mode"`
	Beverages       []NutritionHydrationBeverage `json:"beverages,omitempty"`
}

type nutritionHydrationAggregate struct {
	WaterML         float64
	CountedDrinksML float64
	OtherDrinksML   float64
	HydrationML     float64
	Beverages       []NutritionHydrationBeverage
}

func normalizeHydrationMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case hydrationModeFlexible:
		return hydrationModeFlexible
	default:
		return hydrationModeStrict
	}
}

func hydrationModeDescription(mode string) string {
	switch normalizeHydrationMode(mode) {
	case hydrationModeFlexible:
		return "гибкий: в цель идут вода, чай и кофе; сладкие напитки и энергетики считаются отдельно"
	default:
		return "строгий: в цель идёт только вода"
	}
}

func validateHydrationMode(mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case hydrationModeStrict, hydrationModeFlexible:
		return nil
	default:
		return fmt.Errorf("invalid hydration_mode")
	}
}

func normalizeHydrationBeverageType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validateHydrationBeverageType(value string) error {
	switch normalizeHydrationBeverageType(value) {
	case hydrationBeverageTea, hydrationBeverageCoffee, hydrationBeverageEnergy, hydrationBeverageMilkshake, hydrationBeverageOther:
		return nil
	default:
		return fmt.Errorf("invalid beverage_type")
	}
}

func hydrationCountsTowardGoal(mode, beverageType string) bool {
	switch normalizeHydrationMode(mode) {
	case hydrationModeFlexible:
		switch normalizeHydrationBeverageType(beverageType) {
		case hydrationBeverageTea, hydrationBeverageCoffee:
			return true
		}
	}
	return false
}

func loadHydrationRange(ctx context.Context, db *pgxpool.Pool, userID string, startDate, endDate time.Time, mode string) (map[string]*nutritionHydrationAggregate, error) {
	result := make(map[string]*nutritionHydrationAggregate)

	waterRows, err := db.Query(ctx, `
		SELECT TO_CHAR(date, 'YYYY-MM-DD'), COALESCE(water_ml, 0)
		FROM nutrition_daily
		WHERE user_id = $1
			AND date >= $2::date
			AND date <= $3::date
			AND COALESCE(water_ml, 0) > 0
	`, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer waterRows.Close()

	for waterRows.Next() {
		var day string
		var waterML float64
		if err := waterRows.Scan(&day, &waterML); err != nil {
			return nil, err
		}
		agg := ensureHydrationAggregate(result, day)
		agg.WaterML = waterML
	}
	if err := waterRows.Err(); err != nil {
		return nil, err
	}

	beverageRows, err := db.Query(ctx, `
		SELECT TO_CHAR(date, 'YYYY-MM-DD'), beverage_type, COALESCE(amount_ml, 0)
		FROM nutrition_hydration_entries
		WHERE user_id = $1
			AND date >= $2::date
			AND date <= $3::date
			AND COALESCE(amount_ml, 0) > 0
		ORDER BY date DESC,
			CASE beverage_type
				WHEN 'tea' THEN 1
				WHEN 'coffee' THEN 2
				WHEN 'energy' THEN 3
				WHEN 'milkshake' THEN 4
				ELSE 5
			END
	`, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer beverageRows.Close()

	for beverageRows.Next() {
		var day string
		var beverageType string
		var amountML float64
		if err := beverageRows.Scan(&day, &beverageType, &amountML); err != nil {
			return nil, err
		}
		agg := ensureHydrationAggregate(result, day)
		countsTowardGoal := hydrationCountsTowardGoal(mode, beverageType)
		agg.Beverages = append(agg.Beverages, NutritionHydrationBeverage{
			BeverageType:     beverageType,
			AmountML:         amountML,
			CountsTowardGoal: countsTowardGoal,
		})
		if countsTowardGoal {
			agg.CountedDrinksML += amountML
		} else {
			agg.OtherDrinksML += amountML
		}
	}
	if err := beverageRows.Err(); err != nil {
		return nil, err
	}

	for _, agg := range result {
		agg.HydrationML = agg.WaterML + agg.CountedDrinksML
	}

	return result, nil
}

func loadHydrationStateForDate(ctx context.Context, db *pgxpool.Pool, userID string, targetDate time.Time, mode string) (NutritionHydrationState, error) {
	aggregates, err := loadHydrationRange(ctx, db, userID, targetDate, targetDate, mode)
	if err != nil {
		return NutritionHydrationState{}, err
	}
	state := NutritionHydrationState{
		Date:          targetDate.Format("2006-01-02"),
		HydrationMode: normalizeHydrationMode(mode),
	}
	if agg := aggregates[state.Date]; agg != nil {
		state.WaterML = agg.WaterML
		state.HydrationML = agg.HydrationML
		state.CountedDrinksML = agg.CountedDrinksML
		state.OtherDrinksML = agg.OtherDrinksML
		state.Beverages = agg.Beverages
	}
	return state, nil
}

func ensureHydrationAggregate(values map[string]*nutritionHydrationAggregate, day string) *nutritionHydrationAggregate {
	if agg, ok := values[day]; ok {
		return agg
	}
	agg := &nutritionHydrationAggregate{}
	values[day] = agg
	return agg
}
