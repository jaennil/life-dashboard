package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	authmw "life-dashboard/internal/middleware"
)

// LocationPlace is a place as the settings screen shows it: what it is called,
// what it turned out to be, and what else it might have been.
type LocationPlace struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	CustomName string           `json:"custom_name,omitempty"`
	NameSource string           `json:"name_source,omitempty"`
	Kind       string           `json:"kind"`
	Latitude   float64          `json:"latitude"`
	Longitude  float64          `json:"longitude"`
	Visits     int              `json:"visits"`
	Hours      float64          `json:"hours"`
	LastSeen   *string          `json:"last_seen,omitempty"`
	Candidates []placeCandidate `json:"candidates"`
}

// GetLocationPlaces lists the places visits have been grouped into.
//
// GET /api/v1/location/places
func (h *LocationWebhookHandler) GetLocationPlaces(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(authmw.UserIDKey).(string)

	rows, err := h.db.Query(r.Context(), `
		SELECT p.id::text,
		       COALESCE(p.name, ''), COALESCE(p.custom_name, ''), COALESCE(p.name_source, ''),
		       COALESCE(p.kind, 'other'), p.latitude, p.longitude,
		       COALESCE(stats.visits, 0),
		       COALESCE(stats.hours, 0),
		       to_char(stats.last_seen AT TIME ZONE 'Europe/Moscow', 'DD.MM.YYYY HH24:MI'),
		       p.candidates
		FROM location_places p
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS visits,
			       SUM(EXTRACT(EPOCH FROM (v.departed_at - v.arrived_at))) / 3600.0 AS hours,
			       MAX(v.arrived_at) AS last_seen
			FROM location_visits v
			WHERE v.place_id = p.id
		) AS stats ON TRUE
		WHERE p.user_id = $1
		ORDER BY COALESCE(stats.hours, 0) DESC, p.created_at
	`, userID)
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", userID).Msg("list places")
		http.Error(w, "cannot load places", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	places := make([]LocationPlace, 0, 16)
	for rows.Next() {
		var place LocationPlace
		var candidates []byte
		if err := rows.Scan(&place.ID, &place.Name, &place.CustomName, &place.NameSource,
			&place.Kind, &place.Latitude, &place.Longitude,
			&place.Visits, &place.Hours, &place.LastSeen, &candidates); err != nil {
			h.logger.Error().Err(err).Msg("scan place")
			http.Error(w, "cannot load places", http.StatusInternalServerError)
			return
		}
		place.Candidates = []placeCandidate{}
		if len(candidates) > 0 {
			_ = json.Unmarshal(candidates, &place.Candidates)
		}
		place.Hours = roundTo(place.Hours, 1)
		places = append(places, place)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "cannot load places", http.StatusInternalServerError)
		return
	}

	writeJSONStatus(w, http.StatusOK, places)
}

type updateLocationPlaceRequest struct {
	// CustomName is the user's own word for a place. An empty string clears it
	// and hands the place back to whatever the map said.
	CustomName string `json:"custom_name"`
}

// UpdateLocationPlace renames one place.
//
// PUT /api/v1/location/places/{placeID}
//
// A name chosen here always wins over the looked-up one: a hundred metres of
// accuracy cannot tell a gym from the shop below it, and the person who was
// there can.
func (h *LocationWebhookHandler) UpdateLocationPlace(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(authmw.UserIDKey).(string)
	placeID := strings.TrimSpace(chi.URLParam(r, "placeID"))
	if placeID == "" {
		http.Error(w, "missing place id", http.StatusBadRequest)
		return
	}

	var req updateLocationPlaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(req.CustomName)
	if len([]rune(name)) > 120 {
		http.Error(w, "name is too long", http.StatusBadRequest)
		return
	}

	tag, err := h.db.Exec(r.Context(), `
		UPDATE location_places
		SET custom_name = NULLIF($3, ''), updated_at = NOW()
		WHERE id = $1 AND user_id = $2
	`, placeID, userID, name)
	if err != nil {
		h.logger.Error().Err(err).Str("place_id", placeID).Msg("rename place")
		http.Error(w, "cannot rename place", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "place not found", http.StatusNotFound)
		return
	}

	h.logger.Info().Str("user_id", userID).Str("place_id", placeID).Msg("place renamed")
	writeJSONStatus(w, http.StatusOK, map[string]string{"status": "ok"})
}
