package syncstate

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const activityTouchWindow = 15 * time.Minute

// A priority user is never slowed down by inactivity. Their interval is capped so
// even the laziest source is retried within the hour, and a failure costs one
// scheduler tick rather than the escalating hours everyone else waits.
//
// The cap is deliberately not zero: every source here talks to a third-party API
// with its own limits, and this project has already been rate-limited by one of
// them. An hour keeps the data fresh without turning the scheduler into a hammer.
const (
	priorityMaxInterval    = time.Hour
	priorityFailureBackoff = 15 * time.Minute
)

type DueSync struct {
	UserID              string
	LastSyncedAt        *time.Time
	LastFailedAt        *time.Time
	LastActiveAt        *time.Time
	ConsecutiveFailures int
	Priority            bool
}

func TouchUserActivity(ctx context.Context, db *pgxpool.Pool, userID string) error {
	return touchUserActivity(ctx, db, userID, false)
}

func ForceUserActivity(ctx context.Context, db *pgxpool.Pool, userID string) error {
	return touchUserActivity(ctx, db, userID, true)
}

func touchUserActivity(ctx context.Context, db *pgxpool.Pool, userID string, force bool) error {
	query := `
		UPDATE users
		SET last_active_at = NOW()
		WHERE id = $1
	`
	if !force {
		query += ` AND (last_active_at IS NULL OR last_active_at < NOW() - INTERVAL '15 minutes')`
	}
	_, err := db.Exec(ctx, query, userID)
	return err
}

func LoadDueSyncs(ctx context.Context, db *pgxpool.Pool, source string, now time.Time) ([]DueSync, error) {
	rows, err := db.Query(ctx, `
		SELECT ss.user_id, ss.last_synced_at, ss.last_failed_at, ss.consecutive_failures,
		       u.last_active_at, u.sync_priority
		FROM sync_state ss
		INNER JOIN users u ON u.id = ss.user_id
		WHERE ss.source = $1
			AND ss.enabled = TRUE
	`, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	due := make([]DueSync, 0)
	for rows.Next() {
		var item DueSync
		if err := rows.Scan(&item.UserID, &item.LastSyncedAt, &item.LastFailedAt, &item.ConsecutiveFailures,
			&item.LastActiveAt, &item.Priority); err != nil {
			return nil, err
		}
		if IsDueForUser(now, source, item.LastActiveAt, item.LastSyncedAt, item.LastFailedAt, item.ConsecutiveFailures, item.Priority) {
			due = append(due, item)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return due, nil
}

func RecordSyncSuccess(ctx context.Context, db *pgxpool.Pool, source, userID string, at time.Time) error {
	_, err := db.Exec(ctx, `
		INSERT INTO sync_state (
			source, user_id, last_synced_at, updated_at, enabled, consecutive_failures, last_failed_at
		)
		VALUES ($1, $2, $3, $3, TRUE, 0, NULL)
		ON CONFLICT (source, user_id) DO UPDATE SET
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at = EXCLUDED.updated_at,
			consecutive_failures = 0,
			last_failed_at = NULL
	`, source, userID, at)
	return err
}

func RecordSyncFailure(ctx context.Context, db *pgxpool.Pool, source, userID string, at time.Time) error {
	_, err := db.Exec(ctx, `
		INSERT INTO sync_state (
			source, user_id, updated_at, enabled, consecutive_failures, last_failed_at
		)
		VALUES ($1, $2, $3, TRUE, 1, $3)
		ON CONFLICT (source, user_id) DO UPDATE SET
			updated_at = EXCLUDED.updated_at,
			consecutive_failures = sync_state.consecutive_failures + 1,
			last_failed_at = EXCLUDED.last_failed_at
	`, source, userID, at)
	return err
}

// IsDue keeps the original signature for callers that have no priority flag to
// pass; it behaves exactly as before.
func IsDue(now time.Time, source string, lastActiveAt, lastSyncedAt, lastFailedAt *time.Time, consecutiveFailures int) bool {
	return IsDueForUser(now, source, lastActiveAt, lastSyncedAt, lastFailedAt, consecutiveFailures, false)
}

func IsDueForUser(now time.Time, source string, lastActiveAt, lastSyncedAt, lastFailedAt *time.Time, consecutiveFailures int, priority bool) bool {
	baseInterval, ok := sourceIntervalForActivity(source, now, lastActiveAt)
	if priority {
		// Always treated as active, so dormancy can never stop the sync, and the
		// slowest source still comes round within the cap.
		baseInterval, _ = sourceInterval(source, "hot")
		if baseInterval > priorityMaxInterval {
			baseInterval = priorityMaxInterval
		}
		ok = true
	}
	if !ok {
		return false
	}

	nextEligibleAt := now
	if lastSyncedAt != nil {
		nextEligibleAt = lastSyncedAt.Add(baseInterval)
	}

	backoff := failureBackoff(consecutiveFailures)
	if priority && backoff > priorityFailureBackoff {
		backoff = priorityFailureBackoff
	}
	if backoff > 0 && lastFailedAt != nil {
		failureReadyAt := lastFailedAt.Add(backoff)
		if failureReadyAt.After(nextEligibleAt) {
			nextEligibleAt = failureReadyAt
		}
	}

	return !nextEligibleAt.After(now)
}

func sourceIntervalForActivity(source string, now time.Time, lastActiveAt *time.Time) (time.Duration, bool) {
	return sourceInterval(source, activityTier(now, lastActiveAt))
}

func sourceInterval(source string, tier string) (time.Duration, bool) {
	switch source {
	case "todoist", "google_calendar":
		switch tier {
		case "hot":
			return time.Hour, true
		case "warm":
			return 6 * time.Hour, true
		case "cold":
			return 24 * time.Hour, true
		default:
			return 0, false
		}
	case "strava", "hevy":
		switch tier {
		case "hot":
			return 2 * time.Hour, true
		case "warm":
			return 8 * time.Hour, true
		case "cold":
			return 36 * time.Hour, true
		default:
			return 0, false
		}
	case "zenmoney":
		switch tier {
		case "hot":
			return 3 * time.Hour, true
		case "warm":
			return 12 * time.Hour, true
		case "cold":
			return 72 * time.Hour, true
		default:
			return 0, false
		}
	case "fatsecret", "myfitnesspal":
		switch tier {
		case "hot":
			return 6 * time.Hour, true
		case "warm":
			return 12 * time.Hour, true
		case "cold":
			return 72 * time.Hour, true
		default:
			return 0, false
		}
	case "notion":
		switch tier {
		case "hot":
			return 12 * time.Hour, true
		case "warm":
			return 24 * time.Hour, true
		case "cold":
			return 72 * time.Hour, true
		default:
			return 0, false
		}
	case "habitify":
		switch tier {
		case "hot":
			return 6 * time.Hour, true
		case "warm":
			return 12 * time.Hour, true
		case "cold":
			return 48 * time.Hour, true
		default:
			return 0, false
		}
	default:
		switch tier {
		case "hot":
			return 6 * time.Hour, true
		case "warm":
			return 24 * time.Hour, true
		case "cold":
			return 72 * time.Hour, true
		default:
			return 0, false
		}
	}
}

func activityTier(now time.Time, lastActiveAt *time.Time) string {
	if lastActiveAt == nil {
		return "dormant"
	}
	age := now.Sub(*lastActiveAt)
	switch {
	case age <= 24*time.Hour:
		return "hot"
	case age <= 7*24*time.Hour:
		return "warm"
	case age <= 30*24*time.Hour:
		return "cold"
	default:
		return "dormant"
	}
}

func failureBackoff(consecutiveFailures int) time.Duration {
	switch {
	case consecutiveFailures <= 0:
		return 0
	case consecutiveFailures == 1:
		return 30 * time.Minute
	case consecutiveFailures == 2:
		return 2 * time.Hour
	case consecutiveFailures == 3:
		return 6 * time.Hour
	default:
		return 12 * time.Hour
	}
}
