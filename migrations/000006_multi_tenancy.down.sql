DROP VIEW IF EXISTS timeline;

DROP INDEX IF EXISTS idx_zenmoney_tags_user_id;
DROP INDEX IF EXISTS idx_raw_events_user_id;
DROP INDEX IF EXISTS idx_nutrition_daily_user_id;
DROP INDEX IF EXISTS idx_accounts_user_id;
DROP INDEX IF EXISTS idx_transactions_user_id;
DROP INDEX IF EXISTS idx_workouts_user_id;
DROP INDEX IF EXISTS idx_activities_user_id;

-- Restore original constraints
ALTER TABLE journal_entries DROP CONSTRAINT IF EXISTS journal_entries_user_source_external_id_key;
ALTER TABLE journal_entries ADD CONSTRAINT journal_entries_source_external_id_key UNIQUE (source, external_id);

ALTER TABLE sleep_sessions DROP CONSTRAINT IF EXISTS sleep_sessions_user_source_date_key;
ALTER TABLE sleep_sessions ADD CONSTRAINT sleep_sessions_source_date_key UNIQUE (source, date);

DROP INDEX IF EXISTS idx_biometrics_unique;
CREATE UNIQUE INDEX idx_biometrics_unique ON biometrics(timestamp, source, metric_type);

ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_user_external_id_key;
ALTER TABLE transactions ADD CONSTRAINT transactions_external_id_key UNIQUE (external_id);

ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_user_external_id_key;
ALTER TABLE accounts ADD CONSTRAINT accounts_external_id_key UNIQUE (external_id);

ALTER TABLE nutrition_daily DROP CONSTRAINT IF EXISTS nutrition_daily_user_date_key;
ALTER TABLE nutrition_daily ADD CONSTRAINT nutrition_daily_date_key UNIQUE (date);

ALTER TABLE activities DROP CONSTRAINT IF EXISTS activities_user_external_id_key;
ALTER TABLE activities ADD CONSTRAINT activities_external_id_key UNIQUE (external_id);

ALTER TABLE workouts DROP CONSTRAINT IF EXISTS workouts_user_external_id_key;
ALTER TABLE workouts ADD CONSTRAINT workouts_external_id_key UNIQUE (external_id);

ALTER TABLE oauth_tokens DROP CONSTRAINT oauth_tokens_pkey;
ALTER TABLE oauth_tokens ADD PRIMARY KEY (source);

ALTER TABLE sync_state DROP CONSTRAINT sync_state_pkey;
ALTER TABLE sync_state ADD PRIMARY KEY (source);

-- Drop user_id columns
ALTER TABLE zenmoney_tags DROP COLUMN user_id;
ALTER TABLE journal_entries DROP COLUMN user_id;
ALTER TABLE sleep_sessions DROP COLUMN user_id;
ALTER TABLE biometrics DROP COLUMN user_id;
ALTER TABLE transactions DROP COLUMN user_id;
ALTER TABLE accounts DROP COLUMN user_id;
ALTER TABLE nutrition_daily DROP COLUMN user_id;
ALTER TABLE activities DROP COLUMN user_id;
ALTER TABLE workouts DROP COLUMN user_id;
ALTER TABLE raw_events DROP COLUMN user_id;
ALTER TABLE oauth_tokens DROP COLUMN user_id;
ALTER TABLE sync_state DROP COLUMN user_id;

CREATE VIEW timeline AS
    SELECT 'workout'::text AS event_type, id, started_at AS occurred_at, title AS summary FROM workouts
    UNION ALL
    SELECT 'activity', id, started_at, name FROM activities
    UNION ALL
    SELECT 'journal', id, date::timestamptz, title FROM journal_entries WHERE date IS NOT NULL
    UNION ALL
    SELECT 'transaction', id, occurred_at, COALESCE(category, 'uncategorized') || ': ' || amount::text FROM transactions
    ORDER BY occurred_at DESC;
