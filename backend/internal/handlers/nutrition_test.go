package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestParseNutritionWindowDays(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		fallback int
		want     int
	}{
		{name: "missing uses fallback", query: "", fallback: 14, want: 14},
		{name: "valid days", query: "days=30", fallback: 14, want: 30},
		{name: "invalid uses fallback", query: "days=abc", fallback: 14, want: 14},
		{name: "zero uses fallback", query: "days=0", fallback: 14, want: 14},
		{name: "negative uses fallback", query: "days=-5", fallback: 14, want: 14},
		{name: "caps to max", query: "days=365", fallback: 14, want: 90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/nutrition/daily?"+tt.query, nil)
			got := parseNutritionWindowDays(req, tt.fallback)
			if got != tt.want {
				t.Fatalf("parseNutritionWindowDays(%q) = %d, want %d", tt.query, got, tt.want)
			}
		})
	}
}

func TestBuildNutritionGoldenMetricsUsesTargets(t *testing.T) {
	targetCalories := 3000.0
	targetProtein := 180.0
	targetWater := 2500.0

	resp := buildNutritionGoldenMetrics(7, []nutritionGoldenDay{
		{Calories: 2900, Protein: 185, WaterML: 2600, MealTypeCount: 3},
		{Calories: 3100, Protein: 170, WaterML: 2400, MealTypeCount: 4},
		{Calories: 1500, Protein: 90, WaterML: 0, MealTypeCount: 2},
	}, &NutritionTargets{
		TargetCalories: &targetCalories,
		TargetProteinG: &targetProtein,
		TargetWaterML:  &targetWater,
	}, map[string]int{
		"breakfast": 2,
		"lunch":     3,
		"dinner":    2,
	})

	if len(resp.Cards) != 5 {
		t.Fatalf("expected 5 golden cards, got %d", len(resp.Cards))
	}
	if resp.Cards[0].Value != "3/7 дн" {
		t.Fatalf("unexpected consistency card: %+v", resp.Cards[0])
	}
	if resp.Cards[1].Value != "−500 ккал" {
		t.Fatalf("unexpected calories card: %+v", resp.Cards[1])
	}
	if resp.Cards[2].Value != "82% цели" {
		t.Fatalf("unexpected protein card: %+v", resp.Cards[2])
	}
	if resp.Cards[3].Value != "100% цели" {
		t.Fatalf("unexpected hydration card: %+v", resp.Cards[3])
	}
	if resp.Cards[4].Value != "3.0 приёма" {
		t.Fatalf("unexpected structure card: %+v", resp.Cards[4])
	}
}
