package connectors

import (
	"sort"
	"testing"

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
