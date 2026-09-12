package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	// A stay shorter than this is a traffic light, not a place worth reporting.
	aiLocationMinVisitMinutes = 5
	aiLocationTopPlaceLimit   = 8
	aiLocationRecentDayLimit  = 5
)

type AILocationPlace struct {
	// ID distinguishes two places that have no name yet: without it a shop and a
	// gym both collapse into one nameless line.
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Kind   string  `json:"kind"`
	Hours  float64 `json:"hours"`
	Visits int     `json:"visits"`
	Days   int     `json:"days"`
}

type AILocationVisit struct {
	PlaceID   string  `json:"place_id,omitempty"`
	Place     string  `json:"place"`
	Kind      string  `json:"kind"`
	From      string  `json:"from"`
	To        string  `json:"to,omitempty"`
	Minutes   float64 `json:"minutes,omitempty"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type AILocationDay struct {
	Date   string            `json:"date"`
	Visits []AILocationVisit `json:"visits"`
}

type AILocationOverviewData struct {
	Days int `json:"days"`
	// TrackedDays counts the days with any visit at all: a gap means the phone
	// reported nothing, not that the day was spent motionless.
	TrackedDays int               `json:"tracked_days"`
	TotalVisits int               `json:"total_visits"`
	HomeHours   float64           `json:"home_hours"`
	WorkHours   float64           `json:"work_hours"`
	OtherHours  float64           `json:"other_hours"`
	Places      []AILocationPlace `json:"places,omitempty"`
	RecentDays  []AILocationDay   `json:"recent_days,omitempty"`
}

func (h *AIHandler) buildLocationOverviewData(ctx context.Context, userID string, days int) (AILocationOverviewData, error) {
	return h.buildLocationOverviewInRange(ctx, userID, time.Now().AddDate(0, 0, -days), time.Now())
}

func (h *AIHandler) buildLocationOverviewInRange(ctx context.Context, userID string, start, end time.Time) (AILocationOverviewData, error) {
	days := int(end.Sub(start).Hours()/24) + 1
	data := AILocationOverviewData{Days: days}
	since := start

	places, err := h.loadLocationPlaces(ctx, userID, since)
	if err != nil {
		return data, err
	}
	data.Places = places
	for _, place := range places {
		switch place.Kind {
		case placeKindHome:
			data.HomeHours += place.Hours
		case placeKindWork:
			data.WorkHours += place.Hours
		default:
			data.OtherHours += place.Hours
		}
	}

	data.RecentDays, data.TrackedDays, data.TotalVisits, err = h.loadLocationDays(ctx, userID, since)
	if err != nil {
		return data, err
	}
	return data, nil
}

func (h *AIHandler) loadLocationPlaces(ctx context.Context, userID string, since time.Time) ([]AILocationPlace, error) {
	rows, err := h.db.Query(ctx, `
		SELECT p.id::text,
		       COALESCE(NULLIF(p.custom_name, ''), NULLIF(p.name, ''), '') AS name,
		       COALESCE(p.kind, 'other') AS kind,
		       SUM(EXTRACT(EPOCH FROM (v.departed_at - v.arrived_at))) / 3600.0 AS hours,
		       COUNT(*) AS visits,
		       COUNT(DISTINCT (v.arrived_at AT TIME ZONE 'Europe/Moscow')::date) AS days
		FROM location_visits v
		JOIN location_places p ON p.id = v.place_id
		WHERE v.user_id = $1 AND v.arrived_at >= $2 AND v.departed_at IS NOT NULL
		GROUP BY p.id
		ORDER BY hours DESC
		LIMIT $3
	`, userID, since, aiLocationTopPlaceLimit)
	if err != nil {
		return nil, fmt.Errorf("query places: %w", err)
	}
	defer rows.Close()

	places := make([]AILocationPlace, 0, aiLocationTopPlaceLimit)
	for rows.Next() {
		var place AILocationPlace
		if err := rows.Scan(&place.ID, &place.Name, &place.Kind, &place.Hours, &place.Visits, &place.Days); err != nil {
			return nil, err
		}
		places = append(places, place)
	}
	return places, rows.Err()
}

// loadLocationDays returns the last few days visit by visit, which is what makes
// an answer about yesterday specific instead of statistical.
func (h *AIHandler) loadLocationDays(ctx context.Context, userID string, since time.Time) ([]AILocationDay, int, int, error) {
	rows, err := h.db.Query(ctx, `
		SELECT (v.arrived_at AT TIME ZONE 'Europe/Moscow')::date AS day,
		       to_char(v.arrived_at AT TIME ZONE 'Europe/Moscow', 'HH24:MI') AS from_time,
		       CASE WHEN v.departed_at IS NULL THEN ''
		            ELSE to_char(v.departed_at AT TIME ZONE 'Europe/Moscow', 'HH24:MI') END AS to_time,
		       COALESCE(EXTRACT(EPOCH FROM (v.departed_at - v.arrived_at)) / 60.0, 0) AS minutes,
		       COALESCE(p.id::text, '') AS place_id,
		       COALESCE(NULLIF(p.custom_name, ''), NULLIF(p.name, ''), '') AS name,
		       COALESCE(p.kind, 'other') AS kind,
		       v.latitude, v.longitude
		FROM location_visits v
		LEFT JOIN location_places p ON p.id = v.place_id
		WHERE v.user_id = $1 AND v.arrived_at >= $2
		ORDER BY v.arrived_at DESC
	`, userID, since)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("query visits: %w", err)
	}
	defer rows.Close()

	days := make([]AILocationDay, 0, aiLocationRecentDayLimit)
	index := map[string]int{}
	total := 0
	trackedDays := map[string]bool{}

	for rows.Next() {
		var day time.Time
		var visit AILocationVisit
		if err := rows.Scan(&day, &visit.From, &visit.To, &visit.Minutes,
			&visit.PlaceID, &visit.Place, &visit.Kind, &visit.Latitude, &visit.Longitude); err != nil {
			return nil, 0, 0, err
		}

		key := day.Format("2006-01-02")
		trackedDays[key] = true
		total++

		// A stop of a couple of minutes is noise, but it still counts as a day
		// with tracking, so it is dropped only from the timeline.
		if visit.Minutes > 0 && visit.Minutes < aiLocationMinVisitMinutes {
			continue
		}
		position, ok := index[key]
		if !ok {
			if len(days) >= aiLocationRecentDayLimit {
				continue
			}
			days = append(days, AILocationDay{Date: key})
			position = len(days) - 1
			index[key] = position
		}
		days[position].Visits = append(days[position].Visits, visit)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}

	// The query walks backwards so that only the newest days are kept; each day
	// then reads forwards, the way a day is lived.
	for _, day := range days {
		reverseVisits(day.Visits)
	}
	return days, len(trackedDays), total, nil
}

func reverseVisits(visits []AILocationVisit) {
	for i, j := 0, len(visits)-1; i < j; i, j = i+1, j-1 {
		visits[i], visits[j] = visits[j], visits[i]
	}
}

// aiPlaceLabel names a place for the model. An unnamed place is described by
// what it is rather than by coordinates nobody can read.
func aiPlaceLabel(name, kind string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	switch kind {
	case placeKindHome:
		return "дом"
	case placeKindWork:
		return "работа"
	default:
		return "место без названия"
	}
}

// numberUnnamedPlaces gives every still nameless place a number, so that two of
// them read as two places rather than as the same one visited twice. The number
// is the same in the summary and in the daily timeline.
func numberUnnamedPlaces(places []AILocationPlace) map[string]string {
	labels := make(map[string]string, len(places))
	unnamed := 0
	for _, place := range places {
		label := aiPlaceLabel(place.Name, place.Kind)
		if label == "место без названия" {
			unnamed++
			label = fmt.Sprintf("место #%d", unnamed)
		}
		if place.ID != "" {
			labels[place.ID] = label
		}
	}
	return labels
}

// labelFor prefers the numbered label and falls back for a visit whose place did
// not make the summary.
func labelFor(labels map[string]string, placeID, name, kind string) string {
	if label, ok := labels[placeID]; ok {
		return label
	}
	return aiPlaceLabel(name, kind)
}

func renderLocationOverviewText(title string, data AILocationOverviewData) string {
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(title))
	sb.WriteString("\n")
	sb.WriteString("Это визиты, которые определил сам телефон: он отмечает приход и уход, когда ты где-то задержался. Короткие заходы и поездки между местами сюда не попадают, а пропуск в днях означает, что телефон ничего не прислал, а не что ты никуда не выходил.\n")

	if data.TotalVisits == 0 {
		sb.WriteString("Нет данных о местоположении за период\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("Дней с данными: %d, визитов: %d\n", data.TrackedDays, data.TotalVisits))
	sb.WriteString(fmt.Sprintf("Время по типам мест: дом %.0f ч, работа %.0f ч, прочее %.0f ч\n",
		data.HomeHours, data.WorkHours, data.OtherHours))

	labels := numberUnnamedPlaces(data.Places)
	if len(data.Places) > 0 {
		sb.WriteString("Места за период:\n")
		for _, place := range data.Places {
			sb.WriteString(fmt.Sprintf("  - %s: %.0f ч, визитов %d за %d дн.\n",
				labelFor(labels, place.ID, place.Name, place.Kind), place.Hours, place.Visits, place.Days))
		}
	}

	for _, day := range data.RecentDays {
		if len(day.Visits) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s:\n", formatISODateShort(day.Date)))
		for _, visit := range day.Visits {
			label := labelFor(labels, visit.PlaceID, visit.Place, visit.Kind)
			if visit.To == "" {
				sb.WriteString(fmt.Sprintf("  - %s с %s, ещё там\n", label, visit.From))
				continue
			}
			if visit.Minutes == 0 {
				// Arrival and departure at the same second is how a visit that had
				// already started when tracking began is stored. Saying "12:36-12:36"
				// would report a stay of no length instead of an unknown one.
				sb.WriteString(fmt.Sprintf("  - %s: ушёл в %s, время прихода неизвестно\n", label, visit.To))
				continue
			}
			sb.WriteString(fmt.Sprintf("  - %s: %s-%s (%s)\n", label, visit.From, visit.To, formatAIDuration(visit.Minutes)))
		}
	}
	return sb.String()
}

// formatAIDuration writes a length the way it would be said out loud.
func formatAIDuration(minutes float64) string {
	if minutes < 60 {
		return fmt.Sprintf("%.0f мин", minutes)
	}
	hours := int(minutes) / 60
	rest := int(minutes) % 60
	if rest == 0 {
		return fmt.Sprintf("%d ч", hours)
	}
	return fmt.Sprintf("%d ч %d мин", hours, rest)
}
