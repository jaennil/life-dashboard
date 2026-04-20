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
