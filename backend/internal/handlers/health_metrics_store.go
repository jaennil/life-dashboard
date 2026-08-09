package handlers

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// healthMetricBatchSize keeps a full Health Auto Export backfill, which can carry
// thousands of points, to a handful of round trips instead of one per row.
const healthMetricBatchSize = 500

// saveHealthMetrics upserts metric entries into biometrics. It is shared by every
// ingestion shape the webhook accepts: the typed arrays, the flat fields, and the
// Health Auto Export payload.
func (h *HealthWebhookHandler) saveHealthMetrics(ctx context.Context, userID, source string, entries []healthEntry) (saved, skipped int) {
	const upsert = `
		INSERT INTO biometrics (timestamp, source, metric_type, value, unit, metadata, user_id)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
		ON CONFLICT (user_id, timestamp, source, metric_type) DO UPDATE SET
			value = EXCLUDED.value,
			unit = EXCLUDED.unit,
			metadata = EXCLUDED.metadata`

	batch := &pgx.Batch{}
	flush := func() {
		if batch.Len() == 0 {
			return
		}
		results := h.db.SendBatch(ctx, batch)
		for i := 0; i < batch.Len(); i++ {
			if _, err := results.Exec(); err != nil {
				h.logger.Warn().Err(err).Msg("insert health metric failed")
				skipped++
				continue
			}
			saved++
		}
		if err := results.Close(); err != nil {
			h.logger.Warn().Err(err).Msg("close health metric batch failed")
		}
		batch = &pgx.Batch{}
	}

	for _, entry := range entries {
		metricType := normalizeHealthMetricType(firstNonEmpty(entry.Type, entry.Metric))
		if metricType == "" {
			skipped++
			continue
		}

		ts, err := parseHealthEntryTime(entry)
		if err != nil {
			h.logger.Warn().Str("metric_type", metricType).Err(err).Msg("skip health metric with bad timestamp")
			skipped++
			continue
		}

		metadata, _ := json.Marshal(entry.Metadata)
		if len(metadata) == 0 || string(metadata) == "null" {
			metadata = []byte("{}")
		}

		batch.Queue(upsert, ts, source, metricType, entry.Value,
			normalizeHealthUnit(metricType, entry.Unit), metadata, userID)
		if batch.Len() >= healthMetricBatchSize {
			flush()
		}
	}
	flush()

	return saved, skipped
}
