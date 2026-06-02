package handlers

import (
	"net/http"
	"time"
)

type queryDateRange struct {
	Start        time.Time
	End          time.Time
	EndExclusive time.Time
	Days         int
	HasExplicit  bool
}

func parseQueryDateRange(r *http.Request, fallbackStart, fallbackEnd time.Time) queryDateRange {
	loc := aiDisplayLocation
	start := startOfQueryDay(fallbackStart, loc)
	end := startOfQueryDay(fallbackEnd, loc)
	hasExplicit := false

	if parsed, ok := parseQueryDate(r.URL.Query().Get("from"), loc); ok {
		start = parsed
		hasExplicit = true
	}
	if parsed, ok := parseQueryDate(r.URL.Query().Get("to"), loc); ok {
		end = parsed
		hasExplicit = true
	}
	if start.After(end) {
		start = end
	}

	return queryDateRange{
		Start:        start,
		End:          end,
		EndExclusive: end.AddDate(0, 0, 1),
		Days:         daysInclusiveTime(start, end),
		HasExplicit:  hasExplicit,
	}
}

func parseQueryDate(value string, loc *time.Location) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, loc)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func startOfQueryDay(value time.Time, loc *time.Location) time.Time {
	local := value.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

func daysInclusiveTime(start, end time.Time) int {
	days := int(end.Sub(start).Hours()/24) + 1
	if days < 1 {
		return 1
	}
	return days
}
