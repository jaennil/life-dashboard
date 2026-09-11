package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPickPlaceNameTakesALonePOI(t *testing.T) {
	name, source := pickPlaceName(placeNaming{
		Address:    "Гарибальди, 23",
		Candidates: []placeCandidate{{Name: "Пятёрочка", Kind: "supermarket", Distance: 18}},
	})
	if name != "Пятёрочка" || source != "osm_poi" {
		t.Fatalf("got %q from %q, want the shop", name, source)
	}
}

func TestPickPlaceNamePrefersAnAddressToAGuess(t *testing.T) {
	// A mall: several things at the same spot. The nearest one is a coin toss,
	// and a wrong shop name reads as a fact while an address never lies.
	name, source := pickPlaceName(placeNaming{
		Address: "Профсоюзная, 61А",
		Candidates: []placeCandidate{
			{Name: "Кофемания", Distance: 12},
			{Name: "Аптека", Distance: 20},
			{Name: "Спортмастер", Distance: 31},
		},
	})
	if name != "Профсоюзная, 61А" || source != "osm_address" {
		t.Fatalf("got %q from %q, want the address", name, source)
	}
}

func TestPickPlaceNameIgnoresDistantCandidates(t *testing.T) {
	// One candidate, but across the road: the address is the honest answer.
	name, source := pickPlaceName(placeNaming{
		Address:    "Гарибальди, 23",
		Candidates: []placeCandidate{{Name: "Магнит", Distance: 70}},
	})
	if name != "Гарибальди, 23" || source != "osm_address" {
		t.Fatalf("got %q from %q", name, source)
	}
}

func TestPickPlaceNameFindsNothing(t *testing.T) {
	if name, source := pickPlaceName(placeNaming{}); name != "" || source != "" {
		t.Fatalf("invented %q from nothing", name)
	}
}

func TestFormatOSMAddressKeepsWhatAPersonWouldSay(t *testing.T) {
	full := nominatimResponse{DisplayName: "23, улица Гарибальди, Москва, 117335, Россия"}
	full.Address.Road = "улица Гарибальди"
	full.Address.HouseNumber = "23"
	if got := formatOSMAddress(full); got != "улица Гарибальди, 23" {
		t.Errorf("got %q", got)
	}

	roadOnly := nominatimResponse{}
	roadOnly.Address.Road = "Ленинский проспект"
	if got := formatOSMAddress(roadOnly); got != "Ленинский проспект" {
		t.Errorf("got %q", got)
	}

	suburbOnly := nominatimResponse{}
	suburbOnly.Address.Suburb = "Черёмушки"
	if got := formatOSMAddress(suburbOnly); got != "Черёмушки" {
		t.Errorf("got %q", got)
	}

	// Nothing structured: the first part of the display name still beats nothing.
	if got := formatOSMAddress(nominatimResponse{DisplayName: "Парк Горького, Москва"}); got != "Парк Горького" {
		t.Errorf("got %q", got)
	}
	if got := formatOSMAddress(nominatimResponse{}); got != "" {
		t.Errorf("invented %q", got)
	}
}

func TestOSMPlaceNamerReadsBothServices(t *testing.T) {
	overpass := `{"elements":[
		{"lat":55.708400,"lon":37.534400,"tags":{"name":"Пятёрочка","shop":"supermarket"}},
		{"type":"way","center":{"lat":55.708600,"lon":37.534900},"tags":{"name":"Пятёрочка","building":"yes","shop":"yes"}},
		{"lat":55.709200,"lon":37.535600,"tags":{"name":"Аптека","amenity":"pharmacy"}},
		{"lat":55.708300,"lon":37.534300,"tags":{"amenity":"bench"}}
	]}`
	nominatim := `{"display_name":"23, улица Гарибальди, Москва","address":{"road":"улица Гарибальди","house_number":"23"}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Both services demand a real client identity; a naming job that does not
		// send one gets blocked, so the stub insists on it too.
		if !strings.Contains(r.Header.Get("User-Agent"), "life-dashboard") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if strings.Contains(r.URL.Path, "reverse") {
			_, _ = w.Write([]byte(nominatim))
			return
		}
		_, _ = w.Write([]byte(overpass))
	}))
	defer server.Close()

	namer := NewOSMPlaceNamer(server.URL+"/interpreter", server.URL+"/reverse")
	naming, err := namer.Lookup(context.Background(), 55.708392, 37.534392)
	if err != nil {
		t.Fatal(err)
	}

	if naming.Address != "улица Гарибальди, 23" {
		t.Errorf("address = %q", naming.Address)
	}
	// The shop is mapped twice, as a node and as a building: one shop.
	if len(naming.Candidates) != 2 {
		t.Fatalf("candidates = %+v, want the shop and the pharmacy", naming.Candidates)
	}
	if naming.Candidates[0].Name != "Пятёрочка" || naming.Candidates[0].Kind != "supermarket" {
		t.Errorf("nearest candidate = %+v", naming.Candidates[0])
	}
	if naming.Candidates[0].Distance > naming.Candidates[1].Distance {
		t.Error("candidates are not ordered by distance")
	}
	// The unnamed bench is not a place.
	for _, candidate := range naming.Candidates {
		if candidate.Name == "" {
			t.Error("a nameless object became a candidate")
		}
	}
}

func TestOSMPlaceNamerReportsAFailingService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	namer := NewOSMPlaceNamer(server.URL+"/interpreter", server.URL+"/reverse")
	if _, err := namer.Lookup(context.Background(), 55.7, 37.6); err == nil {
		t.Fatal("a rate limited service was reported as a successful lookup")
	}
}

func TestStoreNamingKeepsCandidatesAList(t *testing.T) {
	// The empty case has to be an empty array: a JSON null in that column breaks
	// every query that reads it as a list.
	encoded, err := json.Marshal(emptyCandidateList(placeNaming{}.Candidates))
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "[]" {
		t.Fatalf("no candidates encoded as %s, want []", encoded)
	}
}
