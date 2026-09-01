package connectors

import (
	"context"
	"fmt"
	"time"
)

const (
	// fatSecretBackfillDaysPerRun bounds how much history one sync re-walks. The
	// point is to finish on its own over a few runs rather than in one burst: the
	// account has a generous daily budget - around 5000 calls - but throttles on
	// rapid bursts, which is what error 12 is.
	fatSecretBackfillDaysPerRun = 10
	// fatSecretBackfillPause spaces the requests out. Walking 57 days back to back
	// is what earned "User is performing too many actions" in the first place, and
	// the API publishes no Retry-After to aim at, so the only defence is not to
	// burst.
	fatSecretBackfillPause = 400 * time.Millisecond
)

// backfillMissingFoodIDs re-syncs days whose items predate food_id being stored.
//
// There is no cursor to keep: the days that still need work are exactly the days
// whose items have no food_id, which the database can answer directly. That makes
// the backfill resumable by construction - it cannot drift out of step with
// reality, and an interrupted run simply leaves the remaining days for the next
// one.
func (c *FatSecretConnector) backfillMissingFoodIDs(ctx context.Context, userID, token, secret string) error {
	days, err := c.daysMissingFoodIDs(ctx, userID, fatSecretBackfillDaysPerRun)
	if err != nil {
		return fmt.Errorf("find days missing food ids: %w", err)
	}
	if len(days) == 0 {
		return nil
	}

	done := 0
	for i, date := range days {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(fatSecretBackfillPause):
			}
		}

		if err := c.syncDay(ctx, userID, token, secret, date); err != nil {
			if isFatSecretRateLimitError(err) {
				// Throttling clears within minutes, and the scheduler comes back
				// every fifteen, so stopping here costs nothing but a wait.
				c.logger.Info().Int("days_done", done).
					Msg("backfill paused by throttling, resuming on the next sync")
				return nil
			}
			c.logger.Warn().Err(err).Str("date", date.Format("2006-01-02")).
				Msg("backfill day failed")
			continue
		}
		done++
	}

	c.logger.Info().Int("days", done).Int("requested", len(days)).Msg("food id backfill progressed")
	return nil
}

// daysMissingFoodIDs lists the oldest days that still hold items without a
// provider food id. Oldest first, because the recent days are the ones the
// regular sync already keeps fresh.
func (c *FatSecretConnector) daysMissingFoodIDs(ctx context.Context, userID string, limit int) ([]time.Time, error) {
	rows, err := c.db.Query(ctx, `
		SELECT d.date
		FROM nutrition_daily d
		JOIN nutrition_items i ON i.daily_id = d.id
		WHERE d.user_id = $1 AND i.food_id IS NULL
		GROUP BY d.date
		ORDER BY d.date
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	days := make([]time.Time, 0, limit)
	for rows.Next() {
		var day time.Time
		if err := rows.Scan(&day); err != nil {
			return nil, err
		}
		days = append(days, day)
	}
	return days, rows.Err()
}
