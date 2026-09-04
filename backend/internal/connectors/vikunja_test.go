package connectors

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestVikunjaAPIURL(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"https://vikunja.example.com", "https://vikunja.example.com/api/v1"},
		{"https://vikunja.example.com/", "https://vikunja.example.com/api/v1"},
		{"vikunja.example.com", "https://vikunja.example.com/api/v1"},
		{"  https://vikunja.example.com/api/v1/  ", "https://vikunja.example.com/api/v1"},
		{"http://localhost:3456", "http://localhost:3456/api/v1"},
		{"https://example.com/vikunja", "https://example.com/vikunja/api/v1"},
	}

	for _, tc := range cases {
		got, err := vikunjaAPIURL(tc.raw)
		if err != nil {
			t.Fatalf("vikunjaAPIURL(%q): %v", tc.raw, err)
		}
		if got != tc.want {
			t.Fatalf("vikunjaAPIURL(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}

	for _, raw := range []string{"", "   ", "ftp://vikunja.example.com"} {
		if _, err := vikunjaAPIURL(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestVikunjaTaskFilterAsksForOpenAndRecentlyDone(t *testing.T) {
	since := time.Date(2026, 8, 20, 15, 4, 5, 0, time.UTC)
	want := "done = false || done_at > '2026-08-20T15:04:05Z'"
	if got := vikunjaTaskFilter(since); got != want {
		t.Fatalf("vikunjaTaskFilter = %q, want %q", got, want)
	}
}

func TestVikunjaLastPage(t *testing.T) {
	header := http.Header{}
	header.Set(vikunjaTotalPagesHeader, "3")
	if vikunjaLastPage(header, 2, vikunjaPageSize) {
		t.Fatalf("expected page 2 of 3 to continue")
	}
	if !vikunjaLastPage(header, 3, vikunjaPageSize) {
		t.Fatalf("expected page 3 of 3 to stop")
	}

	// Without the header a full page means there may be more, a short one not.
	if vikunjaLastPage(http.Header{}, 1, vikunjaPageSize) {
		t.Fatalf("expected a full page to continue when the header is missing")
	}
	if !vikunjaLastPage(http.Header{}, 1, vikunjaPageSize-1) {
		t.Fatalf("expected a short page to stop when the header is missing")
	}
}

func TestVikunjaPriorityNormalizesToTodoistScale(t *testing.T) {
	if got := vikunjaPriority(0); got != nil {
		t.Fatalf("expected unset priority to stay unset, got %#v", got)
	}
	if got := vikunjaPriority(2); got != 2 {
		t.Fatalf("expected priority 2 to pass through, got %#v", got)
	}
	// Vikunja's "DO NOW" has no Todoist counterpart above urgent.
	if got := vikunjaPriority(5); got != 4 {
		t.Fatalf("expected DO NOW to collapse onto urgent, got %#v", got)
	}
}

func TestVikunjaRecurrence(t *testing.T) {
	cases := []struct {
		task vikunjaTask
		want string
	}{
		{vikunjaTask{}, ""},
		{vikunjaTask{RepeatAfter: 86400}, "every day"},
		{vikunjaTask{RepeatAfter: 2 * 7 * 24 * 3600}, "every 2 weeks"},
		{vikunjaTask{RepeatAfter: 3600}, "every hour"},
		{vikunjaTask{RepeatAfter: 86400, RepeatMode: 2}, "every day from completion"},
		{vikunjaTask{RepeatMode: 1}, "every month"},
		{vikunjaTask{RepeatAfter: 90000}, "every 25 hours"},
	}

	for _, tc := range cases {
		if got := vikunjaRecurrence(tc.task); got != tc.want {
			t.Fatalf("vikunjaRecurrence(%+v) = %q, want %q", tc.task, got, tc.want)
		}
	}
}

func TestParseVikunjaTimeTreatsZeroValueAsUnset(t *testing.T) {
	if ts := parseVikunjaTime("0001-01-01T00:00:00Z"); !ts.IsZero() {
		t.Fatalf("expected the Go zero time to read as unset, got %s", ts)
	}
	if ts := parseVikunjaTime(""); !ts.IsZero() {
		t.Fatalf("expected an empty timestamp to read as unset")
	}

	ts := parseVikunjaTime("2026-09-04T18:17:12.87454168+03:00")
	if ts.IsZero() {
		t.Fatalf("expected a fractional offset timestamp to parse")
	}
	if got := ts.Format("15:04"); got != "18:17" {
		t.Fatalf("expected the sent offset to be kept, got %s", got)
	}
}

func TestVikunjaProjectPathWalksParents(t *testing.T) {
	projects := map[int64]vikunjaProject{
		1: {ID: 1, Title: "Дом"},
		2: {ID: 2, Title: "Ремонт", ParentProjectID: 1},
		3: {ID: 3, Title: "Кухня", ParentProjectID: 2},
	}

	if got := vikunjaProjectPath(3, projects); got != "Дом / Ремонт / Кухня" {
		t.Fatalf("unexpected project path %q", got)
	}
	if got := vikunjaProjectPath(1, projects); got != "Дом" {
		t.Fatalf("unexpected root project path %q", got)
	}
	if got := vikunjaProjectPath(99, projects); got != "" {
		t.Fatalf("expected an unknown project to name nothing, got %q", got)
	}

	// A parent chain that loops back must not spin forever.
	projects[1] = vikunjaProject{ID: 1, Title: "Дом", ParentProjectID: 3}
	if got := vikunjaProjectPath(3, projects); got != "Дом / Ремонт / Кухня" {
		t.Fatalf("unexpected cyclic project path %q", got)
	}
}

func TestVikunjaSyncStart(t *testing.T) {
	connector := NewVikunja(nil, zerolog.Nop())
	now := time.Date(2026, 9, 4, 18, 0, 0, 0, time.UTC)

	initial := connector.syncStart(time.Time{}, now)
	if want := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC); !initial.Equal(want) {
		t.Fatalf("initial backfill starts at %s, want %s", initial, want)
	}

	incremental := connector.syncStart(now.AddDate(0, 0, -1), now)
	if want := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC); !incremental.Equal(want) {
		t.Fatalf("incremental window starts at %s, want %s", incremental, want)
	}
}

func TestVikunjaFetchProjectsSkipsSavedFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/projects" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("is_archived"); got != "true" {
			t.Errorf("expected archived projects to be requested, got %q", got)
		}
		w.Header().Set(vikunjaTotalPagesHeader, "1")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"title":"Inbox"},{"id":-2,"title":"My Open Tasks"},{"id":4,"title":"citroen","is_archived":true}]`)
	}))
	defer server.Close()

	connector := NewVikunja(nil, zerolog.Nop())
	apiURL, err := vikunjaAPIURL(server.URL)
	if err != nil {
		t.Fatalf("api url: %v", err)
	}

	projects, err := connector.fetchProjects(context.Background(), vikunjaCredentials{apiURL: apiURL, token: "tk_test"})
	if err != nil {
		t.Fatalf("fetch projects: %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("expected the saved filter to be dropped, got %d projects", len(projects))
	}
	if _, ok := projects[-2]; ok {
		t.Fatalf("saved filter reached the project map")
	}
	if !projects[4].IsArchived {
		t.Fatalf("expected an archived project to be kept and marked archived")
	}
}

func TestVikunjaFetchTasksFollowsPagination(t *testing.T) {
	var gotFilters []string
	var gotPages []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tasks" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tk_test" {
			t.Errorf("unexpected authorization header %q", got)
		}
		gotFilters = append(gotFilters, r.URL.Query().Get("filter"))
		page := r.URL.Query().Get("page")
		gotPages = append(gotPages, page)

		w.Header().Set(vikunjaTotalPagesHeader, "2")
		w.Header().Set("Content-Type", "application/json")
		if page == "1" {
			fmt.Fprint(w, `[{"id":1,"title":"open","done":false,"done_at":"0001-01-01T00:00:00Z"}]`)
			return
		}
		fmt.Fprint(w, `[{"id":2,"title":"done","done":true,"done_at":"2026-09-02T17:23:51+03:00"}]`)
	}))
	defer server.Close()

	connector := NewVikunja(nil, zerolog.Nop())
	apiURL, err := vikunjaAPIURL(server.URL)
	if err != nil {
		t.Fatalf("api url: %v", err)
	}

	since := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	tasks, err := connector.fetchTasks(context.Background(), vikunjaCredentials{apiURL: apiURL, token: "tk_test"}, since)
	if err != nil {
		t.Fatalf("fetch tasks: %v", err)
	}

	if len(tasks) != 2 {
		t.Fatalf("expected both pages to be collected, got %d tasks", len(tasks))
	}
	if len(gotPages) != 2 || gotPages[0] != "1" || gotPages[1] != "2" {
		t.Fatalf("unexpected pages requested: %v", gotPages)
	}
	for _, filter := range gotFilters {
		if filter != vikunjaTaskFilter(since) {
			t.Fatalf("unexpected filter %q", filter)
		}
	}
	if tasks[1].DoneAt == "" || parseVikunjaTime(tasks[1].DoneAt).IsZero() {
		t.Fatalf("expected the completed task to keep its done_at")
	}
}

func TestVikunjaFetchTasksSurfacesAPIErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"code":403,"message":"missing permission"}`)
	}))
	defer server.Close()

	connector := NewVikunja(nil, zerolog.Nop())
	apiURL, err := vikunjaAPIURL(server.URL)
	if err != nil {
		t.Fatalf("api url: %v", err)
	}

	_, err = connector.fetchTasks(context.Background(), vikunjaCredentials{apiURL: apiURL, token: "tk_test"}, time.Now())
	if err == nil {
		t.Fatalf("expected a 403 to fail the sync rather than look like an empty task list")
	}
}
