package handlers

import (
	"database/sql"
	"testing"
	"time"
)

func TestMergeNutritionTargetsPrefersManualGoals(t *testing.T) {
	now := time.Now()
	fatsecretRow := &nutritionTargetsRow{
		Source:          "fatsecret",
		CurrentWeightKg: sql.NullFloat64{Float64: 84.2, Valid: true},
		TargetWeightKg:  sql.NullFloat64{Float64: 80, Valid: true},
		HeightCm:        sql.NullFloat64{Float64: 183, Valid: true},
		SyncedAt:        now.Add(-time.Hour),
	}
	manualRow := &nutritionTargetsRow{
		Source:         "manual",
		TargetWeightKg: sql.NullFloat64{Float64: 78, Valid: true},
		TargetCalories: sql.NullFloat64{Float64: 2400, Valid: true},
		TargetProteinG: sql.NullFloat64{Float64: 170, Valid: true},
		TargetCarbsG:   sql.NullFloat64{Float64: 250, Valid: true},
		TargetFatG:     sql.NullFloat64{Float64: 70, Valid: true},
		TargetWaterML:  sql.NullFloat64{Float64: 2600, Valid: true},
		HydrationMode:  sql.NullString{String: hydrationModeFlexible, Valid: true},
		SyncedAt:       now,
	}

	targets := mergeNutritionTargets(fatsecretRow, manualRow)
	if targets == nil {
		t.Fatal("expected merged targets")
	}
	if targets.Source != "fatsecret+manual" {
		t.Fatalf("unexpected source: %s", targets.Source)
	}
	if targets.TargetWeightKg == nil || *targets.TargetWeightKg != 78 {
		t.Fatalf("expected manual target weight 78, got %+v", targets.TargetWeightKg)
	}
	if targets.TargetCalories == nil || *targets.TargetCalories != 2400 {
		t.Fatalf("expected manual calories 2400, got %+v", targets.TargetCalories)
	}
	if targets.CurrentWeightKg == nil || *targets.CurrentWeightKg != 84.2 {
		t.Fatalf("expected fatsecret current weight 84.2, got %+v", targets.CurrentWeightKg)
	}
	if targets.Manual == nil || !targets.Manual.hasAny() {
		t.Fatal("expected manual overrides in result")
	}
	if targets.TargetWaterML == nil || *targets.TargetWaterML != 2600 {
		t.Fatalf("expected manual water target 2600, got %+v", targets.TargetWaterML)
	}
	if targets.HydrationMode != hydrationModeFlexible {
		t.Fatalf("expected hydration mode %q, got %q", hydrationModeFlexible, targets.HydrationMode)
	}
}

func TestMergeNutritionTargetsWithoutFatSecretStillReturnsManual(t *testing.T) {
	now := time.Now()
	manualRow := &nutritionTargetsRow{
		Source:         "manual",
		TargetCalories: sql.NullFloat64{Float64: 2200, Valid: true},
		SyncedAt:       now,
	}

	targets := mergeNutritionTargets(nil, manualRow)
	if targets == nil {
		t.Fatal("expected manual-only targets")
	}
	if targets.Source != "manual" {
		t.Fatalf("unexpected source: %s", targets.Source)
	}
	if targets.TargetCalories == nil || *targets.TargetCalories != 2200 {
		t.Fatalf("expected target calories 2200, got %+v", targets.TargetCalories)
	}
	if targets.Manual == nil || targets.Manual.UpdatedAt == nil {
		t.Fatal("expected manual metadata")
	}
	if targets.HydrationMode != hydrationModeStrict {
		t.Fatalf("expected default hydration mode %q, got %q", hydrationModeStrict, targets.HydrationMode)
	}
}
