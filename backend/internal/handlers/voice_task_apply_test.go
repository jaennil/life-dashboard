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

func TestResolveTaskProject(t *testing.T) {
	projects := []taskProject{
		{ExternalID: "1", Name: "Inbox", Path: "Inbox", IsDefault: true},
		{ExternalID: "4", Name: "citroen", Path: "citroen"},
		{ExternalID: "9", Name: "Ремонт", Path: "Дом / Ремонт"},
		{ExternalID: "11", Name: "Кухня", Path: "Дом / Кухня"},
	}

	for _, tc := range []struct {
		answer string
		want   string
	}{
		{"citroen", "4"},
		{"CITROEN", "4"},
		{"Дом / Ремонт", "9"},
		{"Ремонт", "9"},
		{" ремонт ", "9"},
	} {
		project, ok := resolveTaskProject(tc.answer, projects)
		if !ok || project.ExternalID != tc.want {
			t.Fatalf("resolveTaskProject(%q) = %+v, %v; want id %s", tc.answer, project, ok, tc.want)
		}
	}

	// "Дом" matches two projects and neither is what was asked for, so the task
	// belongs in the default project rather than in a coin-flip.
	for _, answer := range []string{"", "   ", "работа", "Дом"} {
		if project, ok := resolveTaskProject(answer, projects); ok {
			t.Fatalf("resolveTaskProject(%q) unexpectedly matched %+v", answer, project)
		}
	}
}

func TestTaskRepeatSeconds(t *testing.T) {
	cases := []struct {
		repeat  *voiceParsedRepeat
		seconds int64
		monthly bool
		ok      bool
	}{
		{nil, 0, false, false},
		{&voiceParsedRepeat{Every: 1, Unit: "day"}, 86400, false, true},
		{&voiceParsedRepeat{Every: 2, Unit: "week"}, 2 * 7 * 86400, false, true},
		{&voiceParsedRepeat{Every: 0, Unit: "day"}, 86400, false, true},
		{&voiceParsedRepeat{Every: 1, Unit: "месяц"}, 0, true, true},
		// Vikunja repeats by calendar month only one month at a time.
		{&voiceParsedRepeat{Every: 3, Unit: "month"}, 0, false, false},
		{&voiceParsedRepeat{Every: 1, Unit: "квартал"}, 0, false, false},
	}

	for _, tc := range cases {
		seconds, monthly, ok := taskRepeatSeconds(tc.repeat)
		if seconds != tc.seconds || monthly != tc.monthly || ok != tc.ok {
			t.Fatalf("taskRepeatSeconds(%+v) = %d, %v, %v; want %d, %v, %v",
				tc.repeat, seconds, monthly, ok, tc.seconds, tc.monthly, tc.ok)
		}
	}
}

func TestApplyTaskCarriesDescriptionAndRepeat(t *testing.T) {
	writer := &fakeTaskWriter{}
	handler := newTaskTestHandler(writer)
	response := voiceWorkoutResponse{}

	handler.applyTask(context.Background(), "user-1", "", voiceInterpretation{
		Domain: voiceDomainTask,
		Task: &voiceParsedTask{
			Title:       "вынести мусор",
			Description: "оба пакета из кухни",
			Repeat:      &voiceParsedRepeat{Every: 1, Unit: "week"},
		},
	}, &response)

	if len(writer.drafts) != 1 {
		t.Fatalf("expected one task, got %d", len(writer.drafts))
	}
	draft := writer.drafts[0]
	if draft.Description != "оба пакета из кухни" {
		t.Fatalf("unexpected description %q", draft.Description)
	}
	if draft.RepeatEverySeconds != 7*24*3600 || draft.RepeatMonthly {
		t.Fatalf("unexpected repeat %d monthly=%v", draft.RepeatEverySeconds, draft.RepeatMonthly)
	}
}

func TestApplyTaskReportsAnUnusableRepeat(t *testing.T) {
	writer := &fakeTaskWriter{}
	handler := newTaskTestHandler(writer)
	response := voiceWorkoutResponse{}

	handler.applyTask(context.Background(), "user-1", "", voiceInterpretation{
		Domain: voiceDomainTask,
		Task:   &voiceParsedTask{Title: "оплатить квартиру", Repeat: &voiceParsedRepeat{Every: 3, Unit: "month"}},
	}, &response)

	if len(writer.drafts) != 1 || writer.drafts[0].RepeatEverySeconds != 0 {
		t.Fatalf("expected the task to be created without a repeat: %+v", writer.drafts)
	}
	if !strings.Contains(strings.Join(response.Unmatched, "; "), "повтор") {
		t.Fatalf("expected the dropped repeat to be reported: %#v", response.Unmatched)
	}
}

func TestVoiceParsePromptListsProjects(t *testing.T) {
	projects := []taskProject{
		{ExternalID: "1", Name: "Inbox", Path: "Inbox", IsDefault: true},
		{ExternalID: "9", Name: "Ремонт", Path: "Дом / Ремонт"},
	}

	prompt := buildVoiceParsePrompt(voiceTestCandidates, nil, projects, nil, false, time.Now())

	for _, want := range []string{"Проекты пользователя:", "- Inbox (по умолчанию)", "- Дом / Ремонт", "\"project\""} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt is missing %q", want)
		}
	}

	// With no projects mirrored yet the model must not be asked to pick one.
	if empty := buildVoiceParsePrompt(voiceTestCandidates, nil, nil, nil, false, time.Now()); strings.Contains(empty, "Проекты пользователя") {
		t.Fatalf("empty project list still produced a project section")
	}
}

func TestTaskRecurrenceRu(t *testing.T) {
	cases := map[string]string{
		"every day":                 "каждый день",
		"every week":                "каждую неделю",
		"every month":               "каждый месяц",
		"every 2 weeks":             "каждые 2 недели",
		"every 3 days":              "каждые 3 дня",
		"every 5 weeks":             "каждые 5 недель",
		"every 11 days":             "каждые 11 дней",
		"every 21 days":             "каждые 21 день",
		"every day from completion": "каждый день от выполнения",
		"":                          "",
		"every 5 seconds":           "every 5 seconds",
	}
	for given, want := range cases {
		if got := taskRecurrenceRu(given); got != want {
			t.Fatalf("taskRecurrenceRu(%q) = %q, want %q", given, got, want)
		}
	}
}
