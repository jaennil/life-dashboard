package handlers

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"life-dashboard/internal/connectors"
)

const (
	// Data pulled within this window is fresh enough. The scheduler already runs
	// every source every 15 minutes, and re-pulling on every question would hammer
	// providers that have rate-limited this project before.
	aiPrefetchFreshness = 10 * time.Minute
	// Measured over 24h in production: zenmoney 0.4s, todoist 0.8s, xiaomi 0.8s,
	// calendar 1.4s, vikunja 2.6s, fatsecret 3.1s, notion 3.8s, zepp 6.1s. Hevy
	// averages 79s because its routine and template endpoints stall, so it is the
	// one source that regularly hits this cap - and its workouts, the part an
	// answer needs, are fetched first.
	aiPrefetchSourceTimeout = 15 * time.Second
	aiPrefetchTotalBudget   = 20 * time.Second
)

// prefetchPace is how long a refresh may take. A question is answered while
// someone waits, so it gets seconds; a report is written in the background and
// can afford to wait for a slow provider.
type prefetchPace struct {
	total     time.Duration
	perSource time.Duration
}

var (
	answerPrefetchPace  = prefetchPace{total: aiPrefetchTotalBudget, perSource: aiPrefetchSourceTimeout}
	checkupPrefetchPace = prefetchPace{total: 90 * time.Second, perSource: 60 * time.Second}
)

// aiConnectorSyncer pulls one source on demand. The AI side takes the interface
// rather than the sync handler so it stays out of the connector wiring.
type aiConnectorSyncer interface {
	SyncNow(ctx context.Context, source, userID string, trigger connectors.SyncTrigger) error
}

// UseSyncer wires the connectors in after construction, the way the Telegram bot
// is wired: the AI handler needs them only to refresh data before answering.
func (h *AIHandler) UseSyncer(syncer aiConnectorSyncer) {
	h.syncer = syncer
}

// aiToolSyncSources says which provider stands behind each tool. Tools with no
// entry read data that is pushed to us - Apple Health, Screen Time - or that is
// not ours to refresh, like the weather.
var aiToolSyncSources = map[aiToolName][]string{
	aiToolFinanceOverview:      {"zenmoney"},
	aiToolRecentTransactions:   {"zenmoney"},
	aiToolProductivityOverview: {"vikunja", "todoist"},
	aiToolActivityOverview:     {"strava"},
	aiToolRecentActivities:     {"strava"},
	aiToolWorkoutOverview:      {"hevy"},
	aiToolRecentWorkouts:       {"hevy"},
	aiToolRoutineOverview:      {"hevy"},
	aiToolHabitOverview:        {"habitify"},
	aiToolNutritionOverview:    {"fatsecret"},
	aiToolJournalOverview:      {"notion"},
	aiToolCalendarOverview:     {"google_calendar"},
	aiToolHealthOverview:       {"zepp", "xiaomi_scale"},
}

// prefetchToolSources refreshes the providers the planned tools read from, so a
// question about the balance is answered with the balance as it is now rather
// than as it was at the last cron tick.
//
// Everything here is best effort. A provider that is slow, down or not connected
// costs the answer nothing but the wait it was allowed, and the data already in
// the database still answers the question.
func (h *AIHandler) prefetchToolSources(ctx context.Context, userID string, tools []aiToolCall, progress func(aiProgressUpdate) error, pace prefetchPace) {
	if h.syncer == nil {
		return
	}
	sources := h.staleSourcesFor(ctx, userID, tools)
	if len(sources) == 0 {
		return
	}

	if progress != nil {
		_ = progress(aiProgressUpdate{
			Stage:   "loading",
			Message: "Обновляю данные: " + strings.Join(sources, ", "),
		})
	}

	ctx, cancel := context.WithTimeout(ctx, pace.total)
	defer cancel()

	var wg sync.WaitGroup
	for _, source := range sources {
		wg.Add(1)
		go func(source string) {
			defer wg.Done()

			sourceCtx, cancelSource := context.WithTimeout(ctx, pace.perSource)
			defer cancelSource()

			startedAt := time.Now()
			if err := h.syncer.SyncNow(sourceCtx, source, userID, connectors.SyncTriggerPrefetch); err != nil {
				h.logger.Warn().Err(err).Str("user_id", userID).Str("source", source).
					Dur("duration", time.Since(startedAt)).Msg("prefetch sync failed")
				return
			}
			h.logger.Info().Str("user_id", userID).Str("source", source).
				Dur("duration", time.Since(startedAt)).Msg("prefetch sync done")
		}(source)
	}
	wg.Wait()
}

// staleSourcesFor keeps only the sources that are actually behind. An enabled
// integration with no sync_state row has never run, which counts as stale.
func (h *AIHandler) staleSourcesFor(ctx context.Context, userID string, tools []aiToolCall) []string {
	wanted := plannedSyncSources(tools)
	if len(wanted) == 0 {
		return nil
	}

	rows, err := h.db.Query(ctx, `
		SELECT wanted.source
		FROM unnest($2::text[]) AS wanted(source)
		LEFT JOIN sync_state ss ON ss.source = wanted.source AND ss.user_id = $1
		WHERE COALESCE(ss.enabled, TRUE)
		  AND (ss.last_synced_at IS NULL OR ss.last_synced_at < NOW() - $3::interval)
	`, userID, wanted, aiPrefetchFreshness.String())
	if err != nil {
		h.logger.Warn().Err(err).Str("user_id", userID).Msg("check source freshness")
		return nil
	}
	defer rows.Close()

	stale := make([]string, 0, len(wanted))
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			return nil
		}
		stale = append(stale, source)
	}
	if rows.Err() != nil {
		return nil
	}
	sort.Strings(stale)
	return stale
}

// plannedSyncSources maps the planned tools onto providers, without repeats: two
// tools reading the same provider are one refresh.
func plannedSyncSources(tools []aiToolCall) []string {
	seen := make(map[string]bool, len(tools))
	sources := make([]string, 0, len(tools))
	for _, tool := range tools {
		for _, source := range aiToolSyncSources[tool.Name] {
			if seen[source] {
				continue
			}
			seen[source] = true
			sources = append(sources, source)
		}
	}
	sort.Strings(sources)
	return sources
}

// checkupPrefetchTools is every tool a report reads, so a checkup refreshes all
// of its sources rather than the handful a single question would have planned.
func checkupPrefetchTools() []aiToolCall {
	tools := make([]aiToolCall, 0, len(aiToolSyncSources))
	for name := range aiToolSyncSources {
		tools = append(tools, aiToolCall{Name: name})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools
}
