package handlers

import (
	"context"
	"strings"
)

// aiToolPushedSources names the sources behind tools whose data is pushed to us
// rather than pulled, so they have no entry in aiToolSyncSources but can still
// be switched off in the settings.
var aiToolPushedSources = map[aiToolName][]string{
	aiToolHealthOverview:     {"apple_health"},
	aiToolScreenTimeOverview: {"ios_screentime"},
	aiToolLocationOverview:   {"overland"},
}

func aiToolSources(name aiToolName) []string {
	return append(append([]string(nil), aiToolSyncSources[name]...), aiToolPushedSources[name]...)
}

// disabledSources lists the integrations the user switched off.
//
// Switching one off stops the sync but keeps what was already loaded, and for a
// calendar that is weeks of recurring events stretching into the future. Read
// as current, it put a schedule the user had abandoned into the reports.
func (h *AIHandler) disabledSources(ctx context.Context, userID string) map[string]bool {
	disabled := make(map[string]bool)
	if h.db == nil {
		return disabled
	}
	rows, err := h.db.Query(ctx, `SELECT source FROM sync_state WHERE user_id = $1 AND enabled = FALSE`, userID)
	if err != nil {
		h.logger.Warn().Err(err).Str("user_id", userID).Msg("load disabled sources")
		return disabled
	}
	defer rows.Close()
	for rows.Next() {
		var source string
		if rows.Scan(&source) == nil {
			disabled[source] = true
		}
	}
	return disabled
}

// toolSourcesDisabled is true when every source behind the tool is switched
// off. A tool with one live source still runs: productivity reads Vikunja even
// with Todoist off.
func toolSourcesDisabled(name aiToolName, disabled map[string]bool) bool {
	sources := aiToolSources(name)
	if len(sources) == 0 {
		return false
	}
	for _, source := range sources {
		if !disabled[source] {
			return false
		}
	}
	return true
}

func disabledSourceNames(name aiToolName) string {
	return strings.Join(aiToolSources(name), ", ")
}
