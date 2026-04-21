package handlers

import (
	"testing"
	"time"
)

func TestResolveCheckupWindowYesterday(t *testing.T) {
	now := time.Date(2026, 4, 21, 15, 4, 5, 0, time.UTC)

	window, err := resolveCheckupWindow(now, checkupPeriodYesterday, nil)
	if err != nil {
		t.Fatalf("resolveCheckupWindow returned error: %v", err)
	}

	wantStart := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 4, 20, 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC)

	if window.RequestedPeriod != checkupPeriodYesterday {
		t.Fatalf("requested period = %q, want %q", window.RequestedPeriod, checkupPeriodYesterday)
	}
	if window.Title != "Checkup за вчера" {
		t.Fatalf("title = %q, want %q", window.Title, "Checkup за вчера")
	}
	if window.UserLabel != "за вчера" {
		t.Fatalf("user label = %q, want %q", window.UserLabel, "за вчера")
	}
	if !window.Start.Equal(wantStart) {
		t.Fatalf("start = %s, want %s", window.Start, wantStart)
	}
	if !window.End.Equal(wantEnd) {
		t.Fatalf("end = %s, want %s", window.End, wantEnd)
	}
}

func TestCheckupPeriodLabelYesterday(t *testing.T) {
	if got := checkupPeriodLabel(checkupPeriodYesterday); got != "за вчера" {
		t.Fatalf("label = %q, want %q", got, "за вчера")
	}
}
