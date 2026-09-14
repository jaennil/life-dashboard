package handlers

import (
	"testing"
	"time"
)

func atHour(hour int) time.Time {
	return time.Date(2026, 9, 14, hour, 30, 0, 0, time.UTC)
}

func TestDueMealsFollowTheClock(t *testing.T) {
	for _, test := range []struct {
		hour int
		want []string
	}{
		{9, nil},
		{12, []string{mealBreakfast}},
		// Half past two: breakfast is still within the window it is worth
		// mentioning in, lunch is not late yet.
		{14, []string{mealBreakfast}},
		// Half past three: breakfast has been missed for too long to bring up.
		{15, nil},
		{17, []string{mealLunch}},
		{22, []string{mealDinner}},
	} {
		due := dueMeals(atHour(test.hour))
		if len(due) != len(test.want) {
			t.Fatalf("at %d:30 got %d meals, want %d", test.hour, len(due), len(test.want))
		}
		// The order matters: the latest meal is the one still worth saying.
		for i, meal := range test.want {
			if due[i].Meal != meal {
				t.Errorf("at %d:30 position %d is %s, want %s", test.hour, i, due[i].Meal, meal)
			}
		}
	}
}

func TestSnacksAreNeverLate(t *testing.T) {
	for _, due := range dueMeals(atHour(23)) {
		if due.Meal == mealOther {
			t.Error("reminded about a snack, which has no time to be late by")
		}
	}
}

func TestMealIsHabitualNeedsBothHistoryAndShare(t *testing.T) {
	for _, test := range []struct {
		name                      string
		daysWithMeal, trackedDays int
		want                      bool
	}{
		// The real numbers from the diary: breakfast every tracked day, dinner on
		// about half, snacks rarely.
		{"завтрак: 15 из 15", 15, 15, true},
		{"обед: 14 из 15", 14, 15, true},
		{"ужин: 8 из 15", 8, 15, true},
		{"перекус: 3 из 15", 3, 15, false},
		{"новый дневник", 2, 2, false},
	} {
		if got := mealIsHabitual(test.daysWithMeal, test.trackedDays); got != test.want {
			t.Errorf("%s: habitual = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestMealReminderTitleIsInRussian(t *testing.T) {
	if got := mealReminderTitle(mealLunch); got != "обед не записан" {
		t.Errorf("title = %q", got)
	}
}
