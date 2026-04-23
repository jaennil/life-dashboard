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

type NutritionManualTargets struct {
	TargetWeightKg *float64   `json:"target_weight_kg,omitempty"`
	TargetCalories *float64   `json:"target_calories,omitempty"`
	TargetProteinG *float64   `json:"target_protein_g,omitempty"`
	TargetCarbsG   *float64   `json:"target_carbs_g,omitempty"`
	TargetFatG     *float64   `json:"target_fat_g,omitempty"`
	TargetWaterML  *float64   `json:"target_water_ml,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
}

type NutritionTargets struct {
	Source               string                  `json:"source"`
	CurrentWeightKg      *float64                `json:"current_weight_kg,omitempty"`
	CurrentWeightDate    *string                 `json:"current_weight_date,omitempty"`
	CurrentWeightComment string                  `json:"current_weight_comment,omitempty"`
	TargetWeightKg       *float64                `json:"target_weight_kg,omitempty"`
	HeightCm             *float64                `json:"height_cm,omitempty"`
	TargetCalories       *float64                `json:"target_calories,omitempty"`
	TargetProteinG       *float64                `json:"target_protein_g,omitempty"`
	TargetCarbsG         *float64                `json:"target_carbs_g,omitempty"`
	TargetFatG           *float64                `json:"target_fat_g,omitempty"`
	TargetWaterML        *float64                `json:"target_water_ml,omitempty"`
	WeightMeasure        string                  `json:"weight_measure,omitempty"`
	HeightMeasure        string                  `json:"height_measure,omitempty"`
	APINotes             []string                `json:"api_notes,omitempty"`
	SyncedAt             *time.Time              `json:"synced_at,omitempty"`
	Manual               *NutritionManualTargets `json:"manual,omitempty"`
}

type nutritionTargetsRow struct {
	Source               string
	CurrentWeightKg      sql.NullFloat64
	CurrentWeightDate    sql.NullString
	CurrentWeightComment sql.NullString
	TargetWeightKg       sql.NullFloat64
	HeightCm             sql.NullFloat64
	TargetCalories       sql.NullFloat64
	TargetProteinG       sql.NullFloat64
	TargetCarbsG         sql.NullFloat64
	TargetFatG           sql.NullFloat64
	TargetWaterML        sql.NullFloat64
	WeightMeasure        sql.NullString
	HeightMeasure        sql.NullString
	SyncedAt             time.Time
}

func loadNutritionTargets(ctx context.Context, db *pgxpool.Pool, userID string) (*NutritionTargets, error) {
	rows, err := db.Query(ctx, `
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
			target_water_ml,
			weight_measure,
			height_measure,
			synced_at
		FROM nutrition_targets
		WHERE user_id = $1
			AND source IN ('fatsecret', 'manual')
		ORDER BY CASE WHEN source = 'fatsecret' THEN 0 ELSE 1 END, synced_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fatsecretRow *nutritionTargetsRow
	var manualRow *nutritionTargetsRow
	for rows.Next() {
		row, scanErr := scanNutritionTargetsRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		switch row.Source {
		case "fatsecret":
			if fatsecretRow == nil {
				fatsecretRow = row
			}
		case "manual":
			if manualRow == nil {
				manualRow = row
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return mergeNutritionTargets(fatsecretRow, manualRow), nil
}

func scanNutritionTargetsRow(row pgx.Row) (*nutritionTargetsRow, error) {
	result := &nutritionTargetsRow{}
	if err := row.Scan(
		&result.Source,
		&result.CurrentWeightKg,
		&result.CurrentWeightDate,
		&result.CurrentWeightComment,
		&result.TargetWeightKg,
		&result.HeightCm,
		&result.TargetCalories,
		&result.TargetProteinG,
		&result.TargetCarbsG,
		&result.TargetFatG,
		&result.TargetWaterML,
		&result.WeightMeasure,
		&result.HeightMeasure,
		&result.SyncedAt,
	); err != nil {
		return nil, err
	}
	return result, nil
}

func mergeNutritionTargets(fatsecretRow, manualRow *nutritionTargetsRow) *NutritionTargets {
	if fatsecretRow == nil && manualRow == nil {
		return nil
	}

	result := &NutritionTargets{}
	if fatsecretRow != nil {
		result.Source = "fatsecret"
		result.CurrentWeightKg = floatPtr(fatsecretRow.CurrentWeightKg)
		result.CurrentWeightDate = stringPtr(fatsecretRow.CurrentWeightDate)
		result.CurrentWeightComment = stringValueFromNull(fatsecretRow.CurrentWeightComment)
		result.TargetWeightKg = floatPtr(fatsecretRow.TargetWeightKg)
		result.HeightCm = floatPtr(fatsecretRow.HeightCm)
		result.TargetCalories = floatPtr(fatsecretRow.TargetCalories)
		result.TargetProteinG = floatPtr(fatsecretRow.TargetProteinG)
		result.TargetCarbsG = floatPtr(fatsecretRow.TargetCarbsG)
		result.TargetFatG = floatPtr(fatsecretRow.TargetFatG)
		result.TargetWaterML = floatPtr(fatsecretRow.TargetWaterML)
		result.WeightMeasure = stringValueFromNull(fatsecretRow.WeightMeasure)
		result.HeightMeasure = stringValueFromNull(fatsecretRow.HeightMeasure)
		result.SyncedAt = &fatsecretRow.SyncedAt
	}

	if manualRow != nil {
		if result.Source == "" {
			result.Source = "manual"
		} else {
			result.Source += "+manual"
		}
		result.Manual = &NutritionManualTargets{
			TargetWeightKg: floatPtr(manualRow.TargetWeightKg),
			TargetCalories: floatPtr(manualRow.TargetCalories),
			TargetProteinG: floatPtr(manualRow.TargetProteinG),
			TargetCarbsG:   floatPtr(manualRow.TargetCarbsG),
			TargetFatG:     floatPtr(manualRow.TargetFatG),
			TargetWaterML:  floatPtr(manualRow.TargetWaterML),
			UpdatedAt:      &manualRow.SyncedAt,
		}
		if result.SyncedAt == nil || manualRow.SyncedAt.After(*result.SyncedAt) {
			result.SyncedAt = &manualRow.SyncedAt
		}
		if manualRow.TargetWeightKg.Valid {
			result.TargetWeightKg = floatPtr(manualRow.TargetWeightKg)
		}
		if manualRow.TargetCalories.Valid {
			result.TargetCalories = floatPtr(manualRow.TargetCalories)
		}
		if manualRow.TargetProteinG.Valid {
			result.TargetProteinG = floatPtr(manualRow.TargetProteinG)
		}
		if manualRow.TargetCarbsG.Valid {
			result.TargetCarbsG = floatPtr(manualRow.TargetCarbsG)
		}
		if manualRow.TargetFatG.Valid {
			result.TargetFatG = floatPtr(manualRow.TargetFatG)
		}
		if manualRow.TargetWaterML.Valid {
			result.TargetWaterML = floatPtr(manualRow.TargetWaterML)
		}
	}

	if result.Manual != nil && result.Manual.hasAny() {
		result.APINotes = append(result.APINotes, "Часть целей задана вручную и имеет приоритет над данными FatSecret.")
	}
	if !result.hasMacroTargets() {
		result.APINotes = append(result.APINotes, "FatSecret Platform API profile.get отдает целевой вес, текущий вес и рост, но не отдает целевые калории и БЖУ.")
	}

	return result
}

func renderNutritionTargetsForAI(targets *NutritionTargets) string {
	if targets == nil {
		return "Цели питания/веса: нет данных профиля или ручных целей.\n"
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
	if targets.Manual != nil && targets.Manual.hasAny() {
		lines = append(lines, "- часть nutrition goals задана вручную и имеет приоритет над данными API")
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
	if targets.TargetWaterML != nil {
		lines = append(lines, fmt.Sprintf("- вода: цель %.0f мл в день", *targets.TargetWaterML))
	}

	return strings.Join(lines, "\n") + "\n"
}

func (t *NutritionTargets) hasMacroTargets() bool {
	if t == nil {
		return false
	}
	return t.TargetCalories != nil || t.TargetProteinG != nil || t.TargetCarbsG != nil || t.TargetFatG != nil
}

func (t *NutritionManualTargets) hasAny() bool {
	if t == nil {
		return false
	}
	return t.TargetWeightKg != nil || t.TargetCalories != nil || t.TargetProteinG != nil || t.TargetCarbsG != nil || t.TargetFatG != nil || t.TargetWaterML != nil
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
