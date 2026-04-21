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

	wantStart := "2026-04-20T00:00:00+03:00"
	wantEnd := "2026-04-20T23:59:59+03:00"

	if window.RequestedPeriod != checkupPeriodYesterday {
		t.Fatalf("requested period = %q, want %q", window.RequestedPeriod, checkupPeriodYesterday)
	}
	if window.Title != "Checkup за вчера" {
		t.Fatalf("title = %q, want %q", window.Title, "Checkup за вчера")
	}
	if window.UserLabel != "за вчера" {
		t.Fatalf("user label = %q, want %q", window.UserLabel, "за вчера")
	}
	if got := window.Start.Format("2006-01-02T15:04:05Z07:00"); got != wantStart {
		t.Fatalf("start = %s, want %s", got, wantStart)
	}
	if got := window.End.Format("2006-01-02T15:04:05Z07:00"); got != wantEnd {
		t.Fatalf("end = %s, want %s", got, wantEnd)
	}
}

func TestCheckupPeriodLabelYesterday(t *testing.T) {
	if got := checkupPeriodLabel(checkupPeriodYesterday); got != "за вчера" {
		t.Fatalf("label = %q, want %q", got, "за вчера")
	}
}
