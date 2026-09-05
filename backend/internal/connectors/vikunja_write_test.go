package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestVikunjaAPIPriorityStaysInsideTheScale(t *testing.T) {
	cases := map[int]int{0: 0, -3: 0, 1: 1, 4: 4, 9: 4}
	for given, want := range cases {
		if got := vikunjaAPIPriority(given); got != want {
			t.Fatalf("vikunjaAPIPriority(%d) = %d, want %d", given, got, want)
		}
	}
}

func TestVikunjaTaskRefLinksToTheInstance(t *testing.T) {
	connector := NewVikunja(nil, zerolog.Nop())
	creds := vikunjaCredentials{apiURL: "https://vikunja.example.com/api/v1", webURL: "https://vikunja.example.com"}
	projects := map[int64]vikunjaProject{
		1: {ID: 1, Title: "Дом"},
		2: {ID: 2, Title: "Ремонт", ParentProjectID: 1},
	}

	ref := connector.taskRef(vikunjaTask{
		ID:        7,
		Title:     "  забрать запчасти  ",
		ProjectID: 2,
		DueDate:   "2026-09-05T12:00:00+03:00",
		DoneAt:    "0001-01-01T00:00:00Z",
	}, creds, projects)

	if ref.URL != "https://vikunja.example.com/tasks/7" {
		t.Fatalf("unexpected task url %q", ref.URL)
	}
	if ref.ExternalID != "7" {
		t.Fatalf("unexpected external id %q", ref.ExternalID)
	}
	if ref.Title != "забрать запчасти" {
		t.Fatalf("expected the title to be trimmed, got %q", ref.Title)
	}
	if ref.ProjectName != "Дом / Ремонт" {
		t.Fatalf("unexpected project name %q", ref.ProjectName)
	}
	if ref.DueAt == nil {
		t.Fatalf("expected the due date to be reported")
	}
	if ref.CompletedAt != nil {
		t.Fatalf("expected an unset done_at to stay empty, got %s", ref.CompletedAt)
	}

	ref = connector.taskRef(vikunjaTask{ID: 8, DoneAt: "2026-09-04T18:17:12+03:00"}, creds, projects)
	if ref.CompletedAt == nil {
		t.Fatalf("expected a completed task to report when it was completed")
	}
}

func TestVikunjaProjectRefOrdering(t *testing.T) {
	refs := []VikunjaProjectRef{
		{ID: 4, Path: "Ремонт", Archived: true},
		{ID: 3, Path: "Работа"},
		{ID: 1, Path: "Inbox", IsDefault: true},
		{ID: 2, Path: "Дом"},
	}

	sort.Slice(refs, func(i, j int) bool { return vikunjaProjectRefLess(refs[i], refs[j]) })

	var order []int64
	for _, ref := range refs {
		order = append(order, ref.ID)
	}
	// Default first, then live projects by name, archived last.
	want := []int64{1, 2, 3, 4}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("unexpected project order %v, want %v", order, want)
		}
	}
}

func TestVikunjaCreatePayload(t *testing.T) {
	due := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	payload := vikunjaCreatePayload(VikunjaTaskDraft{
		Title:              "  забрать запчасти  ",
		Description:        "у сервиса на Марьино",
		DueAt:              due,
		Priority:           3,
		RepeatEverySeconds: 7 * 24 * 3600,
	})

	if payload["title"] != "забрать запчасти" {
		t.Fatalf("unexpected title %v", payload["title"])
	}
	if payload["due_date"] != "2026-09-06T09:00:00Z" {
		t.Fatalf("unexpected due_date %v", payload["due_date"])
	}
	if payload["priority"] != 3 {
		t.Fatalf("unexpected priority %v", payload["priority"])
	}
	if payload["repeat_after"] != int64(7*24*3600) {
		t.Fatalf("unexpected repeat_after %v", payload["repeat_after"])
	}
	if _, ok := payload["repeat_mode"]; ok {
		t.Fatalf("an interval repeat must not also send repeat_mode: %v", payload)
	}

	// A monthly repeat is a mode, and Vikunja ignores repeat_after in that mode.
	monthly := vikunjaCreatePayload(VikunjaTaskDraft{Title: "оплатить", RepeatMonthly: true, RepeatEverySeconds: 999})
	if monthly["repeat_mode"] != vikunjaRepeatModeMonthly {
		t.Fatalf("unexpected repeat_mode %v", monthly["repeat_mode"])
	}
	if _, ok := monthly["repeat_after"]; ok {
		t.Fatalf("monthly repeat must not send an interval: %v", monthly)
	}

	// Nothing optional set: only the title travels.
	bare := vikunjaCreatePayload(VikunjaTaskDraft{Title: "постричься"})
	if len(bare) != 1 {
		t.Fatalf("unexpected bare payload %v", bare)
	}
}

func TestApplyTaskLabelsMatchesExistingAndSkipsUnknown(t *testing.T) {
	var bulk map[string]any
	created := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/labels":
			w.Header().Set(vikunjaTotalPagesHeader, "1")
			fmt.Fprint(w, `[{"id":7,"title":"Дом"},{"id":9,"title":"работа"}]`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/labels":
			created++
			fmt.Fprint(w, `{"id":42,"title":"новая"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks/5/labels/bulk":
			_ = json.NewDecoder(r.Body).Decode(&bulk)
			fmt.Fprint(w, `{}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	connector := NewVikunja(nil, zerolog.Nop())
	apiURL, err := vikunjaAPIURL(server.URL)
	if err != nil {
		t.Fatalf("api url: %v", err)
	}
	creds := vikunjaCredentials{apiURL: apiURL, token: "tk_test"}

	// A dictated label is only ever matched, never invented.
	attached, skipped, err := connector.applyTaskLabels(context.Background(), creds, 5, []string{"дом", "новая"}, false)
	if err != nil {
		t.Fatalf("apply labels: %v", err)
	}
	if len(attached) != 1 || attached[0] != "дом" {
		t.Fatalf("unexpected attached labels %#v", attached)
	}
	if len(skipped) != 1 || skipped[0] != "новая" {
		t.Fatalf("unexpected skipped labels %#v", skipped)
	}
	if created != 0 {
		t.Fatalf("a label was created without permission")
	}

	labels, ok := bulk["labels"].([]any)
	if !ok || len(labels) != 1 {
		t.Fatalf("unexpected bulk payload %#v", bulk)
	}
	if id := labels[0].(map[string]any)["id"]; id != float64(7) {
		t.Fatalf("bulk payload carried the wrong label id: %#v", id)
	}
}

func TestApplyTaskLabelsCreatesWhenAllowed(t *testing.T) {
	createdTitles := make([]string, 0, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/labels":
			w.Header().Set(vikunjaTotalPagesHeader, "1")
			fmt.Fprint(w, `[]`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/labels":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			createdTitles = append(createdTitles, fmt.Sprint(body["title"]))
			fmt.Fprint(w, `{"id":11,"title":"ремонт"}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()

	connector := NewVikunja(nil, zerolog.Nop())
	apiURL, _ := vikunjaAPIURL(server.URL)

	attached, skipped, err := connector.applyTaskLabels(context.Background(),
		vikunjaCredentials{apiURL: apiURL, token: "tk_test"}, 5, []string{"  ремонт  ", ""}, true)
	if err != nil {
		t.Fatalf("apply labels: %v", err)
	}
	if len(attached) != 1 || attached[0] != "ремонт" {
		t.Fatalf("unexpected attached labels %#v", attached)
	}
	if len(skipped) != 0 {
		t.Fatalf("nothing should have been skipped: %#v", skipped)
	}
	if len(createdTitles) != 1 || createdTitles[0] != "ремонт" {
		t.Fatalf("unexpected created labels %#v", createdTitles)
	}
}
