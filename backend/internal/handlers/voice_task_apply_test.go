package handlers

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"life-dashboard/internal/connectors"
)

type fakeTaskWriter struct {
	drafts []connectors.VikunjaTaskDraft
	err    error
}

func (f *fakeTaskWriter) CreateTask(_ context.Context, _ string, draft connectors.VikunjaTaskDraft) (connectors.VikunjaTaskRef, error) {
	f.drafts = append(f.drafts, draft)
	if f.err != nil {
		return connectors.VikunjaTaskRef{}, f.err
	}
	due := draft.DueAt
	ref := connectors.VikunjaTaskRef{
		ExternalID:  "42",
		Title:       draft.Title,
		ProjectName: "Inbox",
	}
	if !due.IsZero() {
		ref.DueAt = &due
	}
	return ref, nil
}

func newTaskTestHandler(writer taskWriter) *VoiceWorkoutHandler {
	return &VoiceWorkoutHandler{task: writer, logger: zerolog.Nop()}
}

func TestApplyTaskWritesTheDictatedTask(t *testing.T) {
	writer := &fakeTaskWriter{}
	handler := newTaskTestHandler(writer)
	response := voiceWorkoutResponse{Heard: "напомни забрать запчасти в пятницу"}

	handler.applyTask(context.Background(), "user-1", "", voiceInterpretation{
		Domain: voiceDomainTask,
		Task:   &voiceParsedTask{Title: "забрать запчасти", DueAt: "2026-09-05T12:00:00+03:00", Priority: 3},
	}, &response)

	if len(writer.drafts) != 1 {
		t.Fatalf("expected exactly one task to be created, got %d", len(writer.drafts))
	}
	draft := writer.drafts[0]
	if draft.Title != "забрать запчасти" {
		t.Fatalf("unexpected title %q", draft.Title)
	}
	if draft.Priority != 3 {
		t.Fatalf("unexpected priority %d", draft.Priority)
	}
	if draft.DueAt.IsZero() {
		t.Fatalf("expected the resolved deadline to reach the connector")
	}
	if !strings.Contains(response.Task, "забрать запчасти") {
		t.Fatalf("response does not report the task: %q", response.Task)
	}
	if response.Message == "" {
		t.Fatalf("expected a confirmation message")
	}
}

func TestApplyTaskRefusesToInventTheAction(t *testing.T) {
	writer := &fakeTaskWriter{}
	handler := newTaskTestHandler(writer)

	for _, interpreted := range []voiceInterpretation{
		{Domain: voiceDomainTask},
		{Domain: voiceDomainTask, Task: &voiceParsedTask{Title: "   "}},
	} {
		response := voiceWorkoutResponse{}
		handler.applyTask(context.Background(), "user-1", "", interpreted, &response)

		if len(writer.drafts) != 0 {
			t.Fatalf("expected no task to be created, got %d", len(writer.drafts))
		}
		if response.Task != "" {
			t.Fatalf("expected no task summary, got %q", response.Task)
		}
		if !strings.Contains(response.Message, "не понял") {
			t.Fatalf("unexpected message %q", response.Message)
		}
	}
}

func TestApplyTaskKeepsTheTaskWhenTheDeadlineIsUnusable(t *testing.T) {
	writer := &fakeTaskWriter{}
	handler := newTaskTestHandler(writer)
	response := voiceWorkoutResponse{}

	handler.applyTask(context.Background(), "user-1", "", voiceInterpretation{
		Domain: voiceDomainTask,
		Task:   &voiceParsedTask{Title: "постричься", DueAt: "в пятницу"},
	}, &response)

	if len(writer.drafts) != 1 {
		t.Fatalf("expected the task to be created without a deadline, got %d writes", len(writer.drafts))
	}
	if !writer.drafts[0].DueAt.IsZero() {
		t.Fatalf("expected an unparsable deadline to be dropped, got %s", writer.drafts[0].DueAt)
	}
	if len(response.Unmatched) == 0 || !strings.Contains(strings.Join(response.Unmatched, "; "), "срок") {
		t.Fatalf("expected the dropped deadline to be reported: %#v", response.Unmatched)
	}
}

func TestApplyTaskSaysWhenVikunjaIsNotWired(t *testing.T) {
	handler := newTaskTestHandler(nil)
	response := voiceWorkoutResponse{}

	handler.applyTask(context.Background(), "user-1", "", voiceInterpretation{
		Domain: voiceDomainTask,
		Task:   &voiceParsedTask{Title: "постричься"},
	}, &response)

	if !strings.Contains(response.Message, "Vikunja") {
		t.Fatalf("unexpected message %q", response.Message)
	}
}

func TestSummarizeCreatedTaskShowsWhatWasFiled(t *testing.T) {
	due := time.Date(2026, 9, 5, 12, 0, 0, 0, time.Local)
	got := summarizeCreatedTask(connectors.VikunjaTaskRef{
		Title:       "забрать запчасти",
		ProjectName: "Дом / ТО",
		DueAt:       &due,
	})

	for _, want := range []string{"забрать запчасти", "Дом / ТО", "до 05.09 12:00"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary %q is missing %q", got, want)
		}
	}
}
