package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NutritionTargets struct {
	Source               string     `json:"source"`
	CurrentWeightKg      *float64   `json:"current_weight_kg,omitempty"`
	CurrentWeightDate    *string    `json:"current_weight_date,omitempty"`
	CurrentWeightComment string     `json:"current_weight_comment,omitempty"`
	TargetWeightKg       *float64   `json:"target_weight_kg,omitempty"`
	HeightCm             *float64   `json:"height_cm,omitempty"`
	TargetCalories       *float64   `json:"target_calories,omitempty"`
	TargetProteinG       *float64   `json:"target_protein_g,omitempty"`
	TargetCarbsG         *float64   `json:"target_carbs_g,omitempty"`
	TargetFatG           *float64   `json:"target_fat_g,omitempty"`
	WeightMeasure        string     `json:"weight_measure,omitempty"`
	HeightMeasure        string     `json:"height_measure,omitempty"`
	APINotes             []string   `json:"api_notes,omitempty"`
	SyncedAt             *time.Time `json:"synced_at,omitempty"`
}

func loadNutritionTargets(ctx context.Context, db *pgxpool.Pool, userID string) (*NutritionTargets, error) {
	var source string
	var currentWeight, targetWeight, height, targetCalories, targetProtein, targetCarbs, targetFat sql.NullFloat64
	var currentWeightDate, currentWeightComment, weightMeasure, heightMeasure sql.NullString
	var syncedAt time.Time

	err := db.QueryRow(ctx, `
		SELECT
			source,
			current_weight_kg,
			TO_CHAR(current_weight_date, 'YYYY-MM-DD'),
			current_weight_comment,
			target_weight_kg,
			height_cm,
			target_calories,
			target_protein_g,
			target_carbs_g,
			target_fat_g,
			weight_measure,
			height_measure,
			synced_at
		FROM nutrition_targets
		WHERE user_id = $1
		ORDER BY CASE WHEN source = 'fatsecret' THEN 0 ELSE 1 END, synced_at DESC
		LIMIT 1
	`, userID).Scan(
		&source,
		&currentWeight,
		&currentWeightDate,
		&currentWeightComment,
		&targetWeight,
		&height,
		&targetCalories,
		&targetProtein,
		&targetCarbs,
		&targetFat,
		&weightMeasure,
		&heightMeasure,
		&syncedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	targets := &NutritionTargets{
		Source:               source,
		CurrentWeightKg:      floatPtr(currentWeight),
		CurrentWeightDate:    stringPtr(currentWeightDate),
		CurrentWeightComment: stringValueFromNull(currentWeightComment),
		TargetWeightKg:       floatPtr(targetWeight),
		HeightCm:             floatPtr(height),
		TargetCalories:       floatPtr(targetCalories),
		TargetProteinG:       floatPtr(targetProtein),
		TargetCarbsG:         floatPtr(targetCarbs),
		TargetFatG:           floatPtr(targetFat),
		WeightMeasure:        stringValueFromNull(weightMeasure),
		HeightMeasure:        stringValueFromNull(heightMeasure),
		SyncedAt:             &syncedAt,
	}
	if !targets.hasMacroTargets() {
		targets.APINotes = append(targets.APINotes, "FatSecret Platform API profile.get отдает целевой вес, текущий вес и рост, но не отдает целевые калории и БЖУ.")
	}

	return targets, nil
}

func renderNutritionTargetsForAI(targets *NutritionTargets) string {
	if targets == nil {
		return "Цели питания/веса: нет данных профиля из FatSecret.\n"
	}

	var lines []string
	source := targets.Source
	if source == "" {
		source = "unknown"
	}
	lines = append(lines, fmt.Sprintf("Цели питания/веса (source: %s):", source))
	if targets.CurrentWeightKg != nil {
		line := fmt.Sprintf("- текущий вес: %.1f кг", *targets.CurrentWeightKg)
		if targets.CurrentWeightDate != nil && *targets.CurrentWeightDate != "" {
			line += " на " + *targets.CurrentWeightDate
		}
		lines = append(lines, line)
	}
	if targets.TargetWeightKg != nil {
		lines = append(lines, fmt.Sprintf("- целевой вес: %.1f кг", *targets.TargetWeightKg))
	}
	if targets.HeightCm != nil {
		lines = append(lines, fmt.Sprintf("- рост: %.0f см", *targets.HeightCm))
	}
	if targets.hasMacroTargets() {
		lines = append(lines, "- целевые калории/БЖУ:")
		if targets.TargetCalories != nil {
			lines = append(lines, fmt.Sprintf("  калории: %.0f ккал", *targets.TargetCalories))
		}
		if targets.TargetProteinG != nil {
			lines = append(lines, fmt.Sprintf("  белки: %.0f г", *targets.TargetProteinG))
		}
		if targets.TargetFatG != nil {
			lines = append(lines, fmt.Sprintf("  жиры: %.0f г", *targets.TargetFatG))
		}
		if targets.TargetCarbsG != nil {
			lines = append(lines, fmt.Sprintf("  углеводы: %.0f г", *targets.TargetCarbsG))
		}
	} else {
		lines = append(lines, "- целевые калории/БЖУ: нет данных в API; не придумывай их и сравнивай только с фактическими средними.")
	}

	return strings.Join(lines, "\n") + "\n"
}

func (t *NutritionTargets) hasMacroTargets() bool {
	if t == nil {
		return false
	}
	return t.TargetCalories != nil || t.TargetProteinG != nil || t.TargetCarbsG != nil || t.TargetFatG != nil
}

func floatPtr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

func stringPtr(value sql.NullString) *string {
	if !value.Valid || value.String == "" {
		return nil
	}
	return &value.String
}

func stringValueFromNull(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
