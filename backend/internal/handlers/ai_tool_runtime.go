package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

type aiToolExecution struct {
	Name            aiToolName
	Section         string
	Days            int
	Limit           int
	PastDays        int
	FutureDays      int
	RequestedPeriod string
	Start           *time.Time
	End             *time.Time
	Run             func(*strings.Builder) error
}

type aiToolWindow struct {
	Days            int    `json:"days,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	PastDays        int    `json:"past_days,omitempty"`
	FutureDays      int    `json:"future_days,omitempty"`
	RequestedPeriod string `json:"requested_period,omitempty"`
	Start           string `json:"start,omitempty"`
	End             string `json:"end,omitempty"`
}

type aiToolResult struct {
	Name          aiToolName    `json:"tool"`
	Section       string        `json:"section"`
	Window        *aiToolWindow `json:"window,omitempty"`
	ContextFormat string        `json:"context_format"`
	ContextText   string        `json:"context_text,omitempty"`
	DurationMs    int64         `json:"duration_ms"`
}

type aiToolRenderEnvelope struct {
	ToolResults []aiToolResult `json:"tool_results"`
}

type aiToolRun struct {
	Results  []aiToolResult
	Sections []string
}

func (r aiToolRun) render() string {
	if len(r.Results) == 0 {
		return ""
	}

	body, err := json.MarshalIndent(aiToolRenderEnvelope{ToolResults: r.Results}, "", "  ")
	if err != nil {
		var fallback strings.Builder
		fallback.WriteString("Результаты внутренних data-tools:\n")
		for _, result := range r.Results {
			fallback.WriteString("\n")
			fallback.WriteString(string(result.Name))
			fallback.WriteString(": ")
			fallback.WriteString(strings.TrimSpace(result.ContextText))
			fallback.WriteString("\n")
		}
		return strings.TrimSpace(fallback.String())
	}

	var sb strings.Builder
	sb.WriteString("Ниже результаты внутренних data-tools в JSON. Используй поля tool/section/window/context_text и не выдумывай отсутствующие поля.\n")
	sb.Write(body)
	return strings.TrimSpace(sb.String())
}

func (h *AIHandler) runAITools(ctx context.Context, userID string, executions []aiToolExecution) (aiToolRun, error) {
	run := aiToolRun{
		Results:  make([]aiToolResult, 0, len(executions)),
		Sections: make([]string, 0, len(executions)),
	}
	seenSections := make(map[string]bool, len(executions))

	for _, execution := range executions {
		if execution.Run == nil {
			continue
		}

		startedAt := time.Now()
		var sb strings.Builder
		err := execution.Run(&sb)
		duration := time.Since(startedAt)
		if err != nil {
			h.logger.Warn().
				Err(err).
				Str("user_id", userID).
				Str("tool", string(execution.Name)).
				Str("section", execution.Section).
				Dur("duration", duration).
				Msg("ai tool failed")
			return aiToolRun{}, err
		}

		contextText := strings.TrimSpace(sb.String())
		if contextText == "" {
			h.logger.Debug().
				Str("user_id", userID).
				Str("tool", string(execution.Name)).
				Str("section", execution.Section).
				Dur("duration", duration).
				Msg("ai tool returned empty context")
			continue
		}

		if execution.Section != "" && !seenSections[execution.Section] {
			seenSections[execution.Section] = true
			run.Sections = append(run.Sections, execution.Section)
		}

		run.Results = append(run.Results, aiToolResult{
			Name:          execution.Name,
			Section:       execution.Section,
			Window:        execution.window(),
			ContextFormat: "plain_text_v1",
			ContextText:   contextText,
			DurationMs:    duration.Milliseconds(),
		})

		h.logger.Info().
			Str("user_id", userID).
			Str("tool", string(execution.Name)).
			Str("section", execution.Section).
			Dur("duration", duration).
			Int("bytes", len(contextText)).
			Msg("ai tool executed")
	}

	return run, nil
}

func (e aiToolExecution) window() *aiToolWindow {
	window := &aiToolWindow{
		Days:            e.Days,
		Limit:           e.Limit,
		PastDays:        e.PastDays,
		FutureDays:      e.FutureDays,
		RequestedPeriod: e.RequestedPeriod,
	}
	if e.Start != nil {
		window.Start = e.Start.Format(time.RFC3339)
	}
	if e.End != nil {
		window.End = e.End.Format(time.RFC3339)
	}
	if window.Days == 0 &&
		window.Limit == 0 &&
		window.PastDays == 0 &&
		window.FutureDays == 0 &&
		window.RequestedPeriod == "" &&
		window.Start == "" &&
		window.End == "" {
		return nil
	}
	return window
}
