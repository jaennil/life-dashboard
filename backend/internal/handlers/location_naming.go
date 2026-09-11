package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	// How far around a place to look for something that could be its name. A
	// visit is reported with about a hundred metres of error, and the centroid of
	// several visits is better than that, so this is deliberately tighter than
	// the raw accuracy: a candidate two hundred metres away is not this place.
	placeCandidateRadiusM = 80
	// A single candidate this close is taken as the name without asking. Beyond
	// it there is usually more than one thing at the same spot and guessing would
	// put a name on the map that nobody chose.
	placeConfidentRadiusM = 45
	placeCandidateLimit   = 8
	// A fix worse than this says no more than the visit already did, so it is not
	// worth preferring over the centroid.
	placeFixAccuracyM = 100
	// OpenStreetMap asks for no more than one request a second and no bulk work.
	// Naming is a handful of places a week, so the pace is set by courtesy rather
	// than by need.
	placeNamingPause     = 1500 * time.Millisecond
	placeNamingPerRun    = 5
	placeNamingTimeout   = 20 * time.Second
	placeNamingUserAgent = "life-dashboard/1.0 (personal dashboard; https://lifedash.dubrovskih.ru)"
)

// placeCandidate is something that might be the name of a place.
type placeCandidate struct {
	Name     string  `json:"name"`
	Kind     string  `json:"kind,omitempty"`
	Distance float64 `json:"distance_m"`
}

// placeNaming is what a lookup could learn about one point.
type placeNaming struct {
	Address    string
	Candidates []placeCandidate
}

// placeNamer turns coordinates into something readable. It is an interface
// because the choice of provider is a quality decision that will be revisited:
// OpenStreetMap needs no key, a commercial map knows more small businesses.
type placeNamer interface {
	Lookup(ctx context.Context, latitude, longitude float64) (placeNaming, error)
}

type PlaceNamingHandler struct {
	db     *pgxpool.Pool
	namer  placeNamer
	logger zerolog.Logger
}

func NewPlaceNaming(db *pgxpool.Pool, namer placeNamer, logger zerolog.Logger) *PlaceNamingHandler {
	return &PlaceNamingHandler{
		db:     db,
		namer:  namer,
		logger: logger.With().Str("handler", "place_naming").Logger(),
	}
}

// NameNewPlaces looks up the places that have never been looked at.
//
// It runs on a schedule rather than inside ingestion: an outbound call to a
// public map service has no place in the request the phone is waiting on, and a
// service that is down should delay a name, not a batch of visits.
func (h *PlaceNamingHandler) NameNewPlaces(ctx context.Context) error {
	// The coordinate to look up is not the place centroid but the median of the
	// fixes recorded while actually standing there. A visit's own coordinate is
	// the system's rough guess at the centre of an area; a fix taken during the
	// stay is a real measurement, and its accuracy is whatever the phone was set
	// to. Places with no fixes fall back to the centroid.
	rows, err := h.db.Query(ctx, `
		SELECT p.id,
		       COALESCE(fixes.latitude, p.latitude),
		       COALESCE(fixes.longitude, p.longitude)
		FROM location_places p
		LEFT JOIN LATERAL (
			SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY lp.latitude) AS latitude,
			       percentile_cont(0.5) WITHIN GROUP (ORDER BY lp.longitude) AS longitude
			FROM location_visits v
			JOIN location_points lp
			  ON lp.user_id = v.user_id
			 AND lp.recorded_at >= v.arrived_at
			 AND lp.recorded_at <= COALESCE(v.departed_at, v.arrived_at)
			 AND COALESCE(lp.accuracy_m, 9999) <= $2
			WHERE v.place_id = p.id
		) AS fixes ON TRUE
		WHERE p.named_at IS NULL
		ORDER BY p.created_at
		LIMIT $1
	`, placeNamingPerRun, placeFixAccuracyM)
	if err != nil {
		return err
	}

	type target struct {
		id        string
		latitude  float64
		longitude float64
	}
	targets := make([]target, 0, placeNamingPerRun)
	for rows.Next() {
		var item target
		if err := rows.Scan(&item.id, &item.latitude, &item.longitude); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for i, item := range targets {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(placeNamingPause):
			}
		}

		naming, err := h.namer.Lookup(ctx, item.latitude, item.longitude)
		if err != nil {
			// Leave named_at unset so the next run tries again: a lookup that
			// failed is not a place without a name.
			h.logger.Warn().Err(err).Str("place_id", item.id).Msg("look up place name")
			continue
		}
		if err := h.storeNaming(ctx, item.id, naming); err != nil {
			h.logger.Error().Err(err).Str("place_id", item.id).Msg("store place name")
		}
	}
	return nil
}

// storeNaming writes what the lookup found. The name is only taken from a
// candidate when there is exactly one nearby; otherwise the address is the name
// and the candidates wait for a person to choose between them.
func (h *PlaceNamingHandler) storeNaming(ctx context.Context, placeID string, naming placeNaming) error {
	name, source := pickPlaceName(naming)
	// A nil slice marshals to JSON null, and a null is not an empty array: every
	// later query that treats the column as a list would fail on it.
	encoded, err := json.Marshal(emptyCandidateList(naming.Candidates))
	if err != nil {
		return err
	}

	_, err = h.db.Exec(ctx, `
		UPDATE location_places
		SET name = NULLIF($2, ''),
		    name_source = NULLIF($3, ''),
		    candidates = $4::jsonb,
		    named_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`, placeID, name, source, encoded)
	return err
}

// emptyCandidateList turns a nil slice into an empty one, because a nil slice
// marshals to JSON null and a null is not an empty array: every later query that
// reads the column as a list would fail on it.
func emptyCandidateList(candidates []placeCandidate) []placeCandidate {
	if candidates == nil {
		return []placeCandidate{}
	}
	return candidates
}

// pickPlaceName decides what to call a place.
//
// One candidate close by is almost certainly the place itself. Several mean a
// mall, a courtyard or a busy street, where the nearest one is a coin toss - and
// a wrong shop name is worse than a street address, because it reads as a fact.
func pickPlaceName(naming placeNaming) (name, source string) {
	near := make([]placeCandidate, 0, len(naming.Candidates))
	for _, candidate := range naming.Candidates {
		if candidate.Distance <= placeConfidentRadiusM {
			near = append(near, candidate)
		}
	}

	if len(near) == 1 {
		return near[0].Name, "osm_poi"
	}
	if strings.TrimSpace(naming.Address) != "" {
		return naming.Address, "osm_address"
	}
	return "", ""
}

// osmPlaceNamer reads OpenStreetMap: Overpass for what stands at a point,
// Nominatim for the address it stands at. Neither needs a key, and both are
// asked at a pace their usage policy allows.
type osmPlaceNamer struct {
	http         *http.Client
	overpassURL  string
	nominatimURL string
}

func NewOSMPlaceNamer(overpassURL, nominatimURL string) *osmPlaceNamer {
	return &osmPlaceNamer{
		http:         &http.Client{Timeout: placeNamingTimeout},
		overpassURL:  emptyFallback(overpassURL, "https://overpass-api.de/api/interpreter"),
		nominatimURL: emptyFallback(nominatimURL, "https://nominatim.openstreetmap.org/reverse"),
	}
}

func (n *osmPlaceNamer) Lookup(ctx context.Context, latitude, longitude float64) (placeNaming, error) {
	naming := placeNaming{}

	candidates, err := n.nearbyPOI(ctx, latitude, longitude)
	if err != nil {
		// An address alone still names a place, so a failing POI lookup is not
		// the end of the attempt.
		return naming, err
	}
	naming.Candidates = candidates

	address, err := n.address(ctx, latitude, longitude)
	if err != nil {
		return naming, err
	}
	naming.Address = address
	return naming, nil
}

// overpassResponse is the part of the answer worth reading.
type overpassResponse struct {
	Elements []struct {
		Lat    float64 `json:"lat"`
		Lon    float64 `json:"lon"`
		Center *struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
		} `json:"center"`
		Tags map[string]string `json:"tags"`
	} `json:"elements"`
}

func (n *osmPlaceNamer) nearbyPOI(ctx context.Context, latitude, longitude float64) ([]placeCandidate, error) {
	query := fmt.Sprintf(`[out:json][timeout:15];
(
  nwr(around:%d,%f,%f)["name"]["amenity"];
  nwr(around:%d,%f,%f)["name"]["shop"];
  nwr(around:%d,%f,%f)["name"]["leisure"];
  nwr(around:%d,%f,%f)["name"]["office"];
);
out center tags %d;`,
		placeCandidateRadiusM, latitude, longitude,
		placeCandidateRadiusM, latitude, longitude,
		placeCandidateRadiusM, latitude, longitude,
		placeCandidateRadiusM, latitude, longitude,
		placeCandidateLimit*3)

	body, err := n.post(ctx, n.overpassURL, url.Values{"data": {query}})
	if err != nil {
		return nil, fmt.Errorf("overpass: %w", err)
	}

	var decoded overpassResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode overpass: %w", err)
	}

	candidates := make([]placeCandidate, 0, len(decoded.Elements))
	for _, element := range decoded.Elements {
		lat, lon := element.Lat, element.Lon
		if element.Center != nil {
			lat, lon = element.Center.Lat, element.Center.Lon
		}
		name := strings.TrimSpace(element.Tags["name"])
		if name == "" || lat == 0 || lon == 0 {
			continue
		}
		candidates = append(candidates, placeCandidate{
			Name:     name,
			Kind:     osmCandidateKind(element.Tags),
			Distance: roundTo(metersBetween(latitude, longitude, lat, lon), 0),
		})
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Distance < candidates[j].Distance })
	candidates = dedupeCandidates(candidates)
	if len(candidates) > placeCandidateLimit {
		candidates = candidates[:placeCandidateLimit]
	}
	return candidates, nil
}

// osmCandidateKind reduces the tag soup to the one word that says what a place
// is, in the order of how specific the tag usually is.
func osmCandidateKind(tags map[string]string) string {
	for _, key := range []string{"shop", "amenity", "leisure", "office"} {
		if value := strings.TrimSpace(tags[key]); value != "" && value != "yes" {
			return value
		}
	}
	return ""
}

// dedupeCandidates keeps the nearest of each name: a shop mapped as both a node
// and a building is one shop.
func dedupeCandidates(candidates []placeCandidate) []placeCandidate {
	seen := make(map[string]bool, len(candidates))
	unique := make([]placeCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		key := strings.ToLower(candidate.Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, candidate)
	}
	return unique
}

type nominatimResponse struct {
	Address struct {
		Road        string `json:"road"`
		HouseNumber string `json:"house_number"`
		Suburb      string `json:"suburb"`
		City        string `json:"city"`
		Town        string `json:"town"`
	} `json:"address"`
	DisplayName string `json:"display_name"`
}

func (n *osmPlaceNamer) address(ctx context.Context, latitude, longitude float64) (string, error) {
	query := url.Values{
		"lat":             {fmt.Sprintf("%f", latitude)},
		"lon":             {fmt.Sprintf("%f", longitude)},
		"format":          {"jsonv2"},
		"zoom":            {"18"},
		"accept-language": {"ru"},
	}
	body, err := n.get(ctx, n.nominatimURL+"?"+query.Encode())
	if err != nil {
		return "", fmt.Errorf("nominatim: %w", err)
	}

	var decoded nominatimResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("decode nominatim: %w", err)
	}
	return formatOSMAddress(decoded), nil
}

// formatOSMAddress keeps the part of an address a person would say out loud.
// The full display name carries the postcode, the district and the country,
// none of which help to recognise where you were.
func formatOSMAddress(response nominatimResponse) string {
	road := strings.TrimSpace(response.Address.Road)
	house := strings.TrimSpace(response.Address.HouseNumber)

	switch {
	case road != "" && house != "":
		return road + ", " + house
	case road != "":
		return road
	}

	for _, fallback := range []string{response.Address.Suburb, response.Address.City, response.Address.Town} {
		if value := strings.TrimSpace(fallback); value != "" {
			return value
		}
	}
	if display := strings.TrimSpace(response.DisplayName); display != "" {
		parts := strings.Split(display, ",")
		return strings.TrimSpace(parts[0])
	}
	return ""
}

func (n *osmPlaceNamer) post(ctx context.Context, endpoint string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return n.do(req)
}

func (n *osmPlaceNamer) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	return n.do(req)
}

// do sends the request with the identification OpenStreetMap asks every client
// for: an anonymous script is the one thing their usage policy refuses.
func (n *osmPlaceNamer) do(req *http.Request) ([]byte, error) {
	req.Header.Set("User-Agent", placeNamingUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := n.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
