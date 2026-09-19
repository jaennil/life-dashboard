package handlers

import (
	"testing"
	"time"
)

func mskTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02 15:04", value, aiDisplayLocation)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return parsed
}

func TestCheckupScheduleDueDaily(t *testing.T) {
	schedule := CheckupSchedule{Period: checkupPeriodToday, Hour: 21, Minute: 0}

	if _, ok := checkupScheduleDue(schedule, mskTime(t, "2026-09-07 20:59")); ok {
		t.Fatalf("a minute early must not fire")
	}

	scheduled, ok := checkupScheduleDue(schedule, mskTime(t, "2026-09-07 21:03"))
	if !ok {
		t.Fatalf("expected the daily schedule to fire just after its time")
	}
	if !scheduled.Equal(mskTime(t, "2026-09-07 21:00")) {
		t.Fatalf("unexpected scheduled instant %s", scheduled)
	}

	// Once it has run for that instant it must not fire again on the next tick.
	ran := scheduled
	schedule.LastRunAt = &ran
	if _, ok := checkupScheduleDue(schedule, mskTime(t, "2026-09-07 21:08")); ok {
		t.Fatalf("a schedule already served must not fire twice")
	}

	// Yesterday's run does not cover today.
	yesterday := mskTime(t, "2026-09-06 21:00")
	schedule.LastRunAt = &yesterday
	if _, ok := checkupScheduleDue(schedule, mskTime(t, "2026-09-07 21:03")); !ok {
		t.Fatalf("expected today's run after yesterday's")
	}
}

func TestCheckupScheduleSkipsStaleWindow(t *testing.T) {
	schedule := CheckupSchedule{Period: checkupPeriodToday, Hour: 21, Minute: 0}

	// Down for an hour: still worth sending.
	if _, ok := checkupScheduleDue(schedule, mskTime(t, "2026-09-07 22:00")); !ok {
		t.Fatalf("expected a late but useful report to fire")
	}
	// Down all night: yesterday's report is not what anyone wants at breakfast.
	if _, ok := checkupScheduleDue(schedule, mskTime(t, "2026-09-08 09:00")); ok {
		t.Fatalf("expected a stale schedule to be skipped")
	}
}

func TestCheckupScheduleWeeklyAndMonthly(t *testing.T) {
	sunday := int(time.Sunday)
	weekly := CheckupSchedule{Period: checkupPeriodWeek, Hour: 21, Weekday: &sunday}

	// 2026-09-07 is a Monday, 2026-09-06 a Sunday.
	if _, ok := checkupScheduleDue(weekly, mskTime(t, "2026-09-07 21:05")); ok {
		t.Fatalf("weekly schedule fired on the wrong weekday")
	}
	if _, ok := checkupScheduleDue(weekly, mskTime(t, "2026-09-06 21:05")); !ok {
		t.Fatalf("weekly schedule did not fire on its weekday")
	}

	third := 3
	monthly := CheckupSchedule{Period: checkupPeriodMonth, Hour: 10, DayOfMonth: &third}
	if _, ok := checkupScheduleDue(monthly, mskTime(t, "2026-09-02 10:05")); ok {
		t.Fatalf("monthly schedule fired on the wrong day")
	}
	if _, ok := checkupScheduleDue(monthly, mskTime(t, "2026-09-03 10:05")); !ok {
		t.Fatalf("monthly schedule did not fire on its day")
	}
}

func TestNormalizeCheckupSchedule(t *testing.T) {
	weekday, day := 3, 15

	daily := CheckupSchedule{Period: checkupPeriodToday, Hour: 8, Weekday: &weekday, DayOfMonth: &day}
	if err := normalizeCheckupSchedule(&daily); err != nil {
		t.Fatalf("normalize daily: %v", err)
	}
	if daily.Weekday != nil || daily.DayOfMonth != nil {
		t.Fatalf("daily schedule kept fields it does not use: %+v", daily)
	}

	weekly := CheckupSchedule{Period: checkupPeriodWeek, Hour: 21}
	if err := normalizeCheckupSchedule(&weekly); err != nil {
		t.Fatalf("normalize weekly: %v", err)
	}
	if weekly.Weekday == nil || *weekly.Weekday != 0 {
		t.Fatalf("weekly schedule did not default to Sunday: %+v", weekly)
	}

	monthly := CheckupSchedule{Period: checkupPeriodMonth, Hour: 10}
	if err := normalizeCheckupSchedule(&monthly); err != nil {
		t.Fatalf("normalize monthly: %v", err)
	}
	if monthly.DayOfMonth == nil || *monthly.DayOfMonth != 1 {
		t.Fatalf("monthly schedule did not default to the first: %+v", monthly)
	}

	tooLate := 30
	for _, invalid := range []CheckupSchedule{
		{Period: "quarter", Hour: 10},
		{Period: checkupPeriodToday, Hour: 24},
		{Period: checkupPeriodToday, Hour: 10, Minute: 60},
		{Period: checkupPeriodMonth, Hour: 10, DayOfMonth: &tooLate},
	} {
		schedule := invalid
		if err := normalizeCheckupSchedule(&schedule); err == nil {
			t.Fatalf("expected %+v to be rejected", invalid)
		}
	}
}

func TestCheckupScheduleAtMidnightStillFires(t *testing.T) {
	// The report was moved to 23:59 and stopped arriving: the sweep runs every
	// five minutes, so nothing looks at the clock during that one minute, and by
	// 00:00 the slot has already become tomorrow's.
	schedule := CheckupSchedule{Period: checkupPeriodToday, Hour: 23, Minute: 59, Enabled: true}

	atMidnight := time.Date(2026, 9, 19, 0, 0, 0, 0, aiDisplayLocation)
	scheduled, ok := checkupScheduleDue(schedule, atMidnight)
	if !ok {
		t.Fatal("a 23:59 report never comes due")
	}
	if scheduled.Day() != 18 || scheduled.Hour() != 23 || scheduled.Minute() != 59 {
		t.Errorf("fired for %s, want the previous evening's slot", scheduled.Format("02.01 15:04"))
	}

	// And it is yesterday's slot only until the catch-up window closes.
	lateMorning := time.Date(2026, 9, 19, 9, 0, 0, 0, aiDisplayLocation)
	if _, ok := checkupScheduleDue(schedule, lateMorning); ok {
		t.Error("last night's report would arrive over breakfast")
	}
}

func TestWeeklyCheckupKeepsItsDayAcrossMidnight(t *testing.T) {
	// Sunday 23:59, noticed at ten past midnight on Monday. The slot is still
	// Sunday's, and checking the weekday against the clock would drop it.
	sunday := 0
	schedule := CheckupSchedule{Period: checkupPeriodWeek, Hour: 23, Minute: 59, Weekday: &sunday, Enabled: true}

	monday := time.Date(2026, 9, 14, 0, 10, 0, 0, aiDisplayLocation) // 13.09.2026 is a Sunday
	scheduled, ok := checkupScheduleDue(schedule, monday)
	if !ok {
		t.Fatal("the weekly report never comes due")
	}
	if scheduled.Weekday() != time.Sunday {
		t.Errorf("fired for %s", scheduled.Weekday())
	}
}

func TestCheckupScheduleRunsOnlyOncePerSlot(t *testing.T) {
	// Yesterday's slot must not be re-run after it already has.
	ran := time.Date(2026, 9, 18, 23, 59, 30, 0, aiDisplayLocation)
	schedule := CheckupSchedule{Period: checkupPeriodToday, Hour: 23, Minute: 59, Enabled: true, LastRunAt: &ran}

	atMidnight := time.Date(2026, 9, 19, 0, 5, 0, 0, aiDisplayLocation)
	if _, ok := checkupScheduleDue(schedule, atMidnight); ok {
		t.Error("the same slot fired twice")
	}
}

func TestSlotDecidesWhichDayTheReportIsAbout(t *testing.T) {
	// The 23:59 report is collected at 00:00. Asking for "today" then produces a
	// report about a day four minutes old - which is exactly what arrived: zero
	// transactions and "день только начался".
	slot := time.Date(2026, 9, 19, 23, 59, 0, 0, aiDisplayLocation)
	atMidnight := time.Date(2026, 9, 20, 0, 0, 0, 0, aiDisplayLocation)
	if got := checkupPeriodForSlot(checkupPeriodToday, slot, atMidnight); got != checkupPeriodYesterday {
		t.Errorf("period = %q, want the day the slot belongs to", got)
	}

	// An evening slot collected the same evening is still about today.
	evening := time.Date(2026, 9, 19, 22, 0, 0, 0, aiDisplayLocation)
	if got := checkupPeriodForSlot(checkupPeriodToday, evening, evening.Add(2*time.Minute)); got != checkupPeriodToday {
		t.Errorf("period = %q, want today", got)
	}

	// Weekly and monthly reports have their own windows and are left alone.
	if got := checkupPeriodForSlot(checkupPeriodWeek, slot, atMidnight); got != checkupPeriodWeek {
		t.Errorf("the weekly report was rewritten to %q", got)
	}
}
