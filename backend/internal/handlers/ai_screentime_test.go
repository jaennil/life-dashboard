package handlers

import (
	"strings"
	"testing"
)

func TestRenderScreenTimeOverviewTextWithoutData(t *testing.T) {
	rendered := renderScreenTimeOverviewText("=== ЭКРАННОЕ ВРЕМЯ ===", AIScreenTimeOverviewData{})
	if !strings.Contains(rendered, "Нет данных экранного времени") {
		t.Fatalf("unexpected render: %q", rendered)
	}
	// Without data the caveats would be noise, and worse, would imply numbers.
	if strings.Contains(rendered, "Приложения:") {
		t.Fatalf("empty data rendered an app list: %q", rendered)
	}
}

func TestRenderScreenTimeOverviewText(t *testing.T) {
	previous := 5.0
	data := AIScreenTimeOverviewData{
		DaysWithData:  14,
		PartialDays:   1,
		AppHours:      84.0,
		WebsiteHours:  9.5,
		DailyAvgHours: 6.0,
		PrevDailyAvg:  &previous,
		BusiestDay:    &AIScreenTimeDay{Date: "2026-09-02", Hours: 9.4},
		QuietestDay:   &AIScreenTimeDay{Date: "2026-08-28", Hours: 2.1},
		TopApps: []AIScreenTimeItem{
			{Name: "Instagram", Hours: 30.4, Share: 36, Days: 14},
			{Name: "Safari", Hours: 10.2, Share: 12, Days: 12},
		},
		TopWebsites: []AIScreenTimeItem{
			{Name: "2ch.su", Hours: 1.4, Share: 15, Days: 6},
		},
	}

	rendered := renderScreenTimeOverviewText("=== ЭКРАННОЕ ВРЕМЯ (14 дней) ===", data)

	for _, want := range []string{
		"=== ЭКРАННОЕ ВРЕМЯ (14 дней) ===",
		"Дней с данными: 14",
		"всего в приложениях: 84.0 ч",
		"в среднем 6.0 ч/день",
		"пред. период 5.0 ч/день (+1.0)",
		"Из них в сайтах через браузер: 9.5 ч",
		"Незакрытых дней",
		"Максимум: 02.09 - 9.4 ч",
		"минимум: 28.08 - 2.1 ч",
		"  - Instagram: 30.4 ч, 36%, дней с использованием 14",
		"Сайты:",
		"  - 2ch.su: 1.4 ч",
		// The two ways to misread this data have to travel with it.
		"уже входит в время браузера",
		"домашний экран",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("render is missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderScreenTimeOverviewTextShowsADrop(t *testing.T) {
	previous := 7.5
	rendered := renderScreenTimeOverviewText("=== ЭКРАННОЕ ВРЕМЯ ===", AIScreenTimeOverviewData{
		DaysWithData:  7,
		AppHours:      35,
		DailyAvgHours: 5.0,
		PrevDailyAvg:  &previous,
	})
	if !strings.Contains(rendered, "(-2.5)") {
		t.Fatalf("expected the drop to be spelled out:\n%s", rendered)
	}
}

func TestScreenTimeScopeKeywords(t *testing.T) {
	for _, message := range []string{
		"сколько я залипаю в телефоне?",
		"какие сайты я чаще всего открываю",
		"что там с экранным временем за неделю",
		"я много сижу в инстаграме?",
	} {
		if scope := selectAIContextScope(message, nil); !scope.screentime {
			t.Fatalf("expected %q to pull screen time", message)
		}
	}

	// A question about training must not drag phone use into the context.
	if scope := selectAIContextScope("какой у меня рабочий вес в жиме?", nil); scope.screentime {
		t.Fatalf("workout question pulled screen time")
	}
}

func TestHoursFromSeconds(t *testing.T) {
	cases := map[int]float64{0: 0, 1800: 0.5, 3600: 1, 5400: 1.5, 109440: 30.4}
	for seconds, want := range cases {
		if got := hoursFromSeconds(seconds); got != want {
			t.Fatalf("hoursFromSeconds(%d) = %v, want %v", seconds, got, want)
		}
	}
}
