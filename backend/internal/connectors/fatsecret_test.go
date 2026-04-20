package connectors

import "testing"

func TestNormalizeFatSecretMealType(t *testing.T) {
	tests := []struct {
		name   string
		meal   string
		mealID string
		want   string
	}{
		{name: "meal string breakfast", meal: "Breakfast", want: "breakfast"},
		{name: "meal string lunch", meal: "lunch", want: "lunch"},
		{name: "meal string dinner", meal: "DINNER", want: "dinner"},
		{name: "meal string snack", meal: "Snack", want: "snacks"},
		{name: "meal string other", meal: "Other", want: "other"},
		{name: "falls back to meal id", mealID: "2", want: "dinner"},
		{name: "unknown falls back to other", meal: "Brunch", mealID: "9", want: "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeFatSecretMealType(tt.meal, tt.mealID)
			if got != tt.want {
				t.Fatalf("normalizeFatSecretMealType(%q, %q) = %q, want %q", tt.meal, tt.mealID, got, tt.want)
			}
		})
	}
}
