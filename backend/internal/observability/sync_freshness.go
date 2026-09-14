package observability

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

// syncFreshnessCollector publishes when each source last synced, read from the
// database at scrape time.
//
// The counter of successful runs cannot answer this. A counter that appears in a
// fresh process starts at one, and Prometheus never saw the zero before it, so
// increase() over any window reads zero - which is indistinguishable from a
// source that has not synced at all. Six deploys in one day therefore produced
// five "not syncing" alerts for connectors that were syncing every hour.
//
// The database has the answer already and keeps it across restarts, so the
// metric is built from it rather than from anything held in memory.
type syncFreshnessCollector struct {
	db     *pgxpool.Pool
	polled map[string]bool
	logger zerolog.Logger
	desc   *prometheus.Desc
}

// RegisterSyncFreshness publishes life_dashboard_sync_last_success_timestamp_seconds.
//
// polled lists the sources this process goes out and fetches on a timer. The
// rest arrive when the phone decides to send them - location, screen time, the
// health export - and for those a long silence is a quiet evening, not a
// failure. The mode label is what lets an alert tell the two apart instead of
// carrying a list of names that would rot.
func RegisterSyncFreshness(db *pgxpool.Pool, polled []string, logger zerolog.Logger) {
	set := make(map[string]bool, len(polled))
	for _, source := range polled {
		set[source] = true
	}
	collector := &syncFreshnessCollector{
		db:     db,
		polled: set,
		logger: logger,
		desc: prometheus.NewDesc(
			"life_dashboard_sync_last_success_timestamp_seconds",
			"Unix time of the last successful sync of a source, across users.",
			[]string{"source", "mode"},
			nil,
		),
	}
	prometheus.MustRegister(collector)
}

func (c *syncFreshnessCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *syncFreshnessCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// The newest success across users. One account's abandoned row must not make
	// a source look stale for the account that uses it.
	rows, err := c.db.Query(ctx, `
		SELECT source, EXTRACT(EPOCH FROM MAX(last_synced_at))
		FROM sync_state
		WHERE enabled = TRUE AND last_synced_at IS NOT NULL
		GROUP BY source
	`)
	if err != nil {
		c.logger.Warn().Err(err).Msg("collect sync freshness")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var source string
		var epoch float64
		if err := rows.Scan(&source, &epoch); err != nil {
			c.logger.Warn().Err(err).Msg("scan sync freshness")
			return
		}
		mode := "pushed"
		if c.polled[source] {
			mode = "polled"
		}
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, epoch, source, mode)
	}
	if err := rows.Err(); err != nil {
		c.logger.Warn().Err(err).Msg("read sync freshness")
	}
}
