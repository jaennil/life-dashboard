package handlers

import (
	"context"
	"time"
)

const (
	// Two visits belong to the same place when their reported coordinates are
	// within this distance. iOS reports a visit as the centre of an area it is
	// not precise about - the same flat comes back tens of metres apart between
	// stays - so the radius has to absorb that drift without swallowing the shop
	// across the road.
	placeMergeMeters = 120
	// Below this there is not enough history to call anything home or work.
	placeMinHours = 3.0
	// Home is where the nights are, and a night or two somewhere is a hotel.
	placeMinNightHours = 3.0
	// The hours that give a place away: nobody is anywhere but home at four in
	// the morning, and a weekday afternoon is spent wherever the work is.
	placeNightFromHour = 0
	placeNightToHour   = 6
	placeWorkFromHour  = 10
	placeWorkToHour    = 18
	// How far back the labelling looks. Long enough to see a working week, short
	// enough that a new flat becomes home within days of moving.
	placeLabelWindowDays = 30
)

// attachVisitsToPlaces groups every visit that has no place yet.
//
// It runs after ingestion rather than inside it: a visit is worth storing even
// when grouping fails, and a failed grouping can simply be retried on the next
// batch because the work is driven by what is still unassigned.
func (h *LocationWebhookHandler) attachVisitsToPlaces(ctx context.Context, userID string) error {
	rows, err := h.db.Query(ctx, `
		SELECT id, latitude, longitude
		FROM location_visits
		WHERE user_id = $1 AND place_id IS NULL
		ORDER BY arrived_at
	`, userID)
	if err != nil {
		return err
	}

	type pendingVisit struct {
		id        string
		latitude  float64
		longitude float64
	}
	pending := make([]pendingVisit, 0, 16)
	for rows.Next() {
		var visit pendingVisit
		if err := rows.Scan(&visit.id, &visit.latitude, &visit.longitude); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, visit)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, visit := range pending {
		placeID, err := h.placeForCoordinates(ctx, userID, visit.latitude, visit.longitude)
		if err != nil {
			return err
		}
		if _, err := h.db.Exec(ctx,
			`UPDATE location_visits SET place_id = $2, updated_at = NOW() WHERE id = $1`,
			visit.id, placeID); err != nil {
			return err
		}
	}
	return nil
}

// placeForCoordinates finds the place a visit belongs to, creating one when the
// visit is somewhere new. The centroid then moves to the average of everything
// assigned to it, so a place settles on its real centre as visits accumulate.
func (h *LocationWebhookHandler) placeForCoordinates(ctx context.Context, userID string, latitude, longitude float64) (string, error) {
	rows, err := h.db.Query(ctx,
		`SELECT id, latitude, longitude FROM location_places WHERE user_id = $1`, userID)
	if err != nil {
		return "", err
	}

	nearestID := ""
	nearest := float64(placeMergeMeters)
	for rows.Next() {
		var id string
		var placeLat, placeLon float64
		if err := rows.Scan(&id, &placeLat, &placeLon); err != nil {
			rows.Close()
			return "", err
		}
		if distance := metersBetween(placeLat, placeLon, latitude, longitude); distance <= nearest {
			nearest = distance
			nearestID = id
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}

	if nearestID == "" {
		var id string
		err := h.db.QueryRow(ctx, `
			INSERT INTO location_places (user_id, latitude, longitude)
			VALUES ($1, $2, $3)
			RETURNING id
		`, userID, latitude, longitude).Scan(&id)
		return id, err
	}

	// Recentre on everything that belongs here, including the visit being added.
	if _, err := h.db.Exec(ctx, `
		UPDATE location_places p
		SET latitude = centre.latitude, longitude = centre.longitude, updated_at = NOW()
		FROM (
			SELECT AVG(latitude) AS latitude, AVG(longitude) AS longitude
			FROM (
				SELECT latitude, longitude FROM location_visits WHERE place_id = $1
				UNION ALL
				SELECT $2::double precision, $3::double precision
			) AS members
		) AS centre
		WHERE p.id = $1
	`, nearestID, latitude, longitude); err != nil {
		return "", err
	}
	return nearestID, nil
}

// placeUsage is how a place is used across the window: total time, and how much
// of it falls in the hours that give a place away.
type placeUsage struct {
	PlaceID      string
	TotalHours   float64
	NightHours   float64
	WorkdayHours float64
}

// classifyPlaces decides which place is home and which is work.
//
// It reads the clock rather than a map: the place you sleep at is home, and the
// place you spend weekday working hours at - that is not home - is work. This
// needs no geocoder, no API key and no configuration, and it is right for the
// two places that matter most in any question about a day.
func classifyPlaces(usage []placeUsage) map[string]string {
	kinds := make(map[string]string, len(usage))

	// Most nights, not the largest share of nights: someone who also works from
	// home spends most of their hours there, which would sink any share test and
	// hand the label to the office instead.
	homeID := ""
	homeNight := placeMinNightHours
	for _, place := range usage {
		if place.TotalHours < placeMinHours {
			continue
		}
		if place.NightHours >= homeNight {
			homeNight = place.NightHours
			homeID = place.PlaceID
		}
	}

	// Work is the weekday daytime place that is not home. When there is no such
	// place - working from home, or a week off - nothing is called work rather
	// than the nearest cafe being promoted to it.
	workID := ""
	workHours := placeMinHours
	for _, place := range usage {
		if place.PlaceID == homeID || place.TotalHours < placeMinHours {
			continue
		}
		if place.WorkdayHours >= workHours {
			workHours = place.WorkdayHours
			workID = place.PlaceID
		}
	}

	for _, place := range usage {
		switch place.PlaceID {
		case homeID:
			kinds[place.PlaceID] = placeKindHome
		case workID:
			kinds[place.PlaceID] = placeKindWork
		default:
			kinds[place.PlaceID] = placeKindOther
		}
	}
	return kinds
}

const (
	placeKindHome  = "home"
	placeKindWork  = "work"
	placeKindOther = "other"
)

// placeVisitSpan is one stay, reduced to what the labeller needs.
type placeVisitSpan struct {
	PlaceID  string
	Arrived  time.Time
	Departed time.Time
}

// summarizePlaceUsage measures each place against the clock.
//
// The hours are counted by overlap, not by the hour a visit started: a stay from
// nine in the evening to nine in the morning is a night at home, and reading only
// its arrival hour would say it was no night at all.
func summarizePlaceUsage(spans []placeVisitSpan, location *time.Location) []placeUsage {
	byPlace := map[string]*placeUsage{}
	order := make([]string, 0, 8)

	for _, span := range spans {
		if !span.Departed.After(span.Arrived) {
			continue
		}
		usage, seen := byPlace[span.PlaceID]
		if !seen {
			usage = &placeUsage{PlaceID: span.PlaceID}
			byPlace[span.PlaceID] = usage
			order = append(order, span.PlaceID)
		}
		usage.TotalHours += span.Departed.Sub(span.Arrived).Hours()
		usage.NightHours += overlapHours(span, location, placeNightFromHour, placeNightToHour, false)
		usage.WorkdayHours += overlapHours(span, location, placeWorkFromHour, placeWorkToHour, true)
	}

	summary := make([]placeUsage, 0, len(order))
	for _, placeID := range order {
		summary = append(summary, *byPlace[placeID])
	}
	return summary
}

// overlapHours sums how much of a stay falls inside the given hours of the local
// day, optionally counting weekdays only.
func overlapHours(span placeVisitSpan, location *time.Location, fromHour, toHour int, weekdaysOnly bool) float64 {
	start := span.Arrived.In(location)
	end := span.Departed.In(location)

	total := 0.0
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, location)
	for !day.After(end) {
		if weekdaysOnly && (day.Weekday() == time.Saturday || day.Weekday() == time.Sunday) {
			day = day.AddDate(0, 0, 1)
			continue
		}

		windowStart := day.Add(time.Duration(fromHour) * time.Hour)
		windowEnd := day.Add(time.Duration(toHour) * time.Hour)
		if overlapStart, overlapEnd := laterOf(start, windowStart), earlierOf(end, windowEnd); overlapEnd.After(overlapStart) {
			total += overlapEnd.Sub(overlapStart).Hours()
		}
		day = day.AddDate(0, 0, 1)
	}
	return total
}

func laterOf(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earlierOf(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// relabelPlaces recomputes home and work over the recent past, so a move to a
// new flat or a new office corrects itself instead of being remembered forever.
func (h *LocationWebhookHandler) relabelPlaces(ctx context.Context, userID string, since time.Time) error {
	rows, err := h.db.Query(ctx, `
		SELECT place_id, arrived_at, departed_at
		FROM location_visits
		WHERE user_id = $1 AND place_id IS NOT NULL
		  AND departed_at IS NOT NULL AND arrived_at >= $2
	`, userID, since)
	if err != nil {
		return err
	}

	spans := make([]placeVisitSpan, 0, 64)
	for rows.Next() {
		var span placeVisitSpan
		if err := rows.Scan(&span.PlaceID, &span.Arrived, &span.Departed); err != nil {
			rows.Close()
			return err
		}
		spans = append(spans, span)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for placeID, kind := range classifyPlaces(summarizePlaceUsage(spans, aiDisplayLocation)) {
		if _, err := h.db.Exec(ctx,
			`UPDATE location_places SET kind = $2, updated_at = NOW() WHERE id = $1 AND kind IS DISTINCT FROM $2`,
			placeID, kind); err != nil {
			return err
		}
	}
	return nil
}
