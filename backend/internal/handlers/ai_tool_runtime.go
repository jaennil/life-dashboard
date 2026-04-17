package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type aiToolExecution struct {
	Name    aiToolName
	Section string
	Run     func(*strings.Builder) error
}

type aiToolResult struct {
	Name     aiToolName
	Section  string
	Context  string
	Duration time.Duration
}

type aiToolRun struct {
	Results  []aiToolResult
	Sections []string
}

func (r aiToolRun) render() string {
	if len(r.Results) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Ниже результаты внутренних data-tools. Каждый блок — фактическая выборка из конкретного домена.\n")

	for _, result := range r.Results {
		sb.WriteString(fmt.Sprintf("\n--- TOOL %s | SECTION %s | %dms ---\n",
			result.Name,
			emptyFallback(result.Section, "unknown"),
			result.Duration.Milliseconds(),
		))
		sb.WriteString(strings.TrimSpace(result.Context))
		sb.WriteString("\n")
	}

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
			Name:     execution.Name,
			Section:  execution.Section,
			Context:  contextText,
			Duration: duration,
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
