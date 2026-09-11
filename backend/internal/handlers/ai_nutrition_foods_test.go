package handlers

import (
	"strings"
	"testing"
)

func TestRenderNutritionFoodsTextWithoutItems(t *testing.T) {
	rendered := renderNutritionFoodsText(AINutritionFoods{})
	if !strings.Contains(rendered, "нет отдельных продуктов") {
		t.Fatalf("unexpected render: %q", rendered)
	}
	// An empty diary must not grow headings that imply a list follows.
	if strings.Contains(rendered, "Чаще всего в рационе") {
		t.Fatalf("empty data rendered a food list: %q", rendered)
	}
}

func TestRenderNutritionFoodsText(t *testing.T) {
	data := AINutritionFoods{
		TopByCalories: []AINutritionFood{
			{Name: "Куриная грудка", Calories: 4200, Entries: 14, Days: 12, Share: 21},
			{Name: "Хлеб бородинский", Calories: 1900, Entries: 9, Days: 8},
		},
		TopByFrequency: []AINutritionFood{
			{Name: "Кофе с молоком", Calories: 800, Entries: 26, Days: 13},
		},
		MealPatterns: []AINutritionMealPattern{
			{Meal: "breakfast", Foods: []AINutritionFood{{Name: "Овсянка", Calories: 1500, Entries: 10, Days: 10}}},
			{Meal: "dinner", Foods: []AINutritionFood{{Name: "Пельмени", Calories: 2400, Entries: 4, Days: 4}}},
		},
		RecentDays: []AINutritionDayMeals{
			{
				Date: "2026-09-10",
				Items: []AINutritionLoggedItem{
					{Meal: "lunch", Name: "Борщ", Calories: 320, Serving: "1 тарелка"},
					{Meal: "snack", Name: "Банан"},
				},
			},
		},
	}

	rendered := renderNutritionFoodsText(data)

	for _, want := range []string{
		"Больше всего калорий дали:",
		"  - Куриная грудка: 4200 ккал, 21% залогированных калорий, записей 14 за 12 дн.",
		"  - Хлеб бородинский: 1900 ккал, записей 9 за 8 дн.",
		"Чаще всего в рационе:",
		"  - Кофе с молоком: 13 дн. из логов, записей 26, всего 800 ккал",
		"Обычно на завтрак: Овсянка (10 дн., 1500 ккал)",
		"Обычно на ужин: Пельмени (4 дн., 2400 ккал)",
		"Съедено 10.09:",
		"  - Борщ (обед), 1 тарелка, 320 ккал",
		"  - Банан (перекус)",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("render is missing %q:\n%s", want, rendered)
		}
	}

	// A zero share is unknown, not "0% of calories".
	if strings.Contains(rendered, "0% залогированных") {
		t.Fatalf("rendered a share that was never computed:\n%s", rendered)
	}
}

func TestFormatServingSize(t *testing.T) {
	// Every input here is a real FatSecret serving string from the diary.
	cases := map[string]string{
		"3.2 :custom:320s  g Милти Филе Индейки В Беконе":         "320 г",
		"2 1/2 :custom:250s  g Папа Может  Молочные Сосиски":      "250 г",
		"3 :custom:3s  x 100г, 300 g ВкусВилл Стрипсы Куриные":    "300 г",
		"1.2 :custom:1.2s  x 100г (120 g) Ашан Маффин":            "120 г",
		"0.2 :custom:0.2 x 100г, 20 g MacChocolate Какао-Напиток": "20 г",
		"1 :custom:1 порция (330 ml) Zizzi Zizzi C Vitamin":       "330 мл",
		"1 serving Shaurmeals Шаурма Классическая":                "1 порц.",
		"0.8 serving ВкусВилл Чипсы из Куриного Филе":             "0.8 порц.",
		"":      "",
		"кусок": "",
	}

	for input, want := range cases {
		if got := formatServingSize(input); got != want {
			t.Errorf("formatServingSize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMealPatternHeading(t *testing.T) {
	if got := mealPatternHeading("lunch"); got != "Обычно на обед" {
		t.Fatalf("unexpected heading: %q", got)
	}
	// "Обычно на другое" is not a sentence.
	if got := mealPatternHeading("other"); got != "Прочие записи" {
		t.Fatalf("unexpected heading: %q", got)
	}
}

func TestMealTypeRankFollowsTheDay(t *testing.T) {
	day := []string{"breakfast", "morning snack", "lunch", "afternoon snack", "snack", "dinner", "evening snack"}
	for i := 1; i < len(day); i++ {
		if mealTypeRank(day[i-1]) >= mealTypeRank(day[i]) {
			t.Fatalf("%s must come before %s", day[i-1], day[i])
		}
	}
	// Anything unrecognised still beats the catch-all bucket.
	if mealTypeRank("полдник") >= mealTypeRank("other") {
		t.Fatalf("the other bucket must sort last")
	}
}
