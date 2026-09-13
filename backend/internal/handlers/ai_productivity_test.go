package handlers

import (
	"strings"
	"testing"
)

func TestProductivityLineCarriesNoPriorityCode(t *testing.T) {
	// "Разморозить p1-задачи по «bitrix отзывы»" is what the report said when the
	// provider's priority number went into the line. It is a code the person
	// reading the report does not use and cannot decipher.
	data := AIProductivityOverviewData{
		Source: "vikunja",
		KeyTasks: []ProductivityTask{
			{Content: "bitrix отзывы", ProjectName: "работа", Priority: 1, DueBucket: "later"},
		},
	}

	text := renderProductivityOverviewText("Задачи", data)

	if strings.Contains(text, "p1") {
		t.Errorf("the priority code is still in the line: %s", text)
	}
	if !strings.Contains(text, "bitrix отзывы") {
		t.Errorf("the task itself went missing: %s", text)
	}
}
