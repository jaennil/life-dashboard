-- Step 1: Add nullable user_id to all data tables
ALTER TABLE sync_state ADD COLUMN user_id UUID REFERENCES users(id);
ALTER TABLE oauth_tokens ADD COLUMN user_id UUID REFERENCES users(id);
ALTER TABLE raw_events ADD COLUMN user_id UUID REFERENCES users(id);
ALTER TABLE workouts ADD COLUMN user_id UUID REFERENCES users(id);
ALTER TABLE activities ADD COLUMN user_id UUID REFERENCES users(id);
ALTER TABLE nutrition_daily ADD COLUMN user_id UUID REFERENCES users(id);
ALTER TABLE accounts ADD COLUMN user_id UUID REFERENCES users(id);
ALTER TABLE transactions ADD COLUMN user_id UUID REFERENCES users(id);
ALTER TABLE biometrics ADD COLUMN user_id UUID REFERENCES users(id);
ALTER TABLE sleep_sessions ADD COLUMN user_id UUID REFERENCES users(id);
ALTER TABLE journal_entries ADD COLUMN user_id UUID REFERENCES users(id);
ALTER TABLE zenmoney_tags ADD COLUMN user_id UUID REFERENCES users(id);

-- Step 2: Backfill with first user
DO $$
DECLARE first_user UUID;
BEGIN
    SELECT id INTO first_user FROM users ORDER BY created_at LIMIT 1;
    IF first_user IS NOT NULL THEN
        UPDATE sync_state SET user_id = first_user WHERE user_id IS NULL;
        UPDATE oauth_tokens SET user_id = first_user WHERE user_id IS NULL;
        UPDATE raw_events SET user_id = first_user WHERE user_id IS NULL;
        UPDATE workouts SET user_id = first_user WHERE user_id IS NULL;
        UPDATE activities SET user_id = first_user WHERE user_id IS NULL;
        UPDATE nutrition_daily SET user_id = first_user WHERE user_id IS NULL;
        UPDATE accounts SET user_id = first_user WHERE user_id IS NULL;
        UPDATE transactions SET user_id = first_user WHERE user_id IS NULL;
        UPDATE biometrics SET user_id = first_user WHERE user_id IS NULL;
        UPDATE sleep_sessions SET user_id = first_user WHERE user_id IS NULL;
        UPDATE journal_entries SET user_id = first_user WHERE user_id IS NULL;
        UPDATE zenmoney_tags SET user_id = first_user WHERE user_id IS NULL;
    END IF;
END $$;

-- Step 3: Set NOT NULL
ALTER TABLE sync_state ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE oauth_tokens ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE raw_events ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE workouts ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE activities ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE nutrition_daily ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE accounts ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE transactions ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE biometrics ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE sleep_sessions ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE journal_entries ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE zenmoney_tags ALTER COLUMN user_id SET NOT NULL;

-- Step 4: Update primary keys and unique constraints
ALTER TABLE sync_state DROP CONSTRAINT sync_state_pkey;
ALTER TABLE sync_state ADD PRIMARY KEY (source, user_id);

ALTER TABLE oauth_tokens DROP CONSTRAINT oauth_tokens_pkey;
ALTER TABLE oauth_tokens ADD PRIMARY KEY (source, user_id);

ALTER TABLE workouts DROP CONSTRAINT workouts_external_id_key;
ALTER TABLE workouts ADD CONSTRAINT workouts_user_external_id_key UNIQUE (user_id, external_id);

ALTER TABLE activities DROP CONSTRAINT activities_external_id_key;
ALTER TABLE activities ADD CONSTRAINT activities_user_external_id_key UNIQUE (user_id, external_id);

ALTER TABLE nutrition_daily DROP CONSTRAINT nutrition_daily_date_key;
ALTER TABLE nutrition_daily ADD CONSTRAINT nutrition_daily_user_date_key UNIQUE (user_id, date);

ALTER TABLE accounts DROP CONSTRAINT accounts_external_id_key;
ALTER TABLE accounts ADD CONSTRAINT accounts_user_external_id_key UNIQUE (user_id, external_id);

ALTER TABLE transactions DROP CONSTRAINT transactions_external_id_key;
ALTER TABLE transactions ADD CONSTRAINT transactions_user_external_id_key UNIQUE (user_id, external_id);

DROP INDEX IF EXISTS idx_biometrics_unique;
CREATE UNIQUE INDEX idx_biometrics_unique ON biometrics(user_id, timestamp, source, metric_type);

ALTER TABLE sleep_sessions DROP CONSTRAINT sleep_sessions_source_date_key;
ALTER TABLE sleep_sessions ADD CONSTRAINT sleep_sessions_user_source_date_key UNIQUE (user_id, source, date);

ALTER TABLE journal_entries DROP CONSTRAINT journal_entries_source_external_id_key;
ALTER TABLE journal_entries ADD CONSTRAINT journal_entries_user_source_external_id_key UNIQUE (user_id, source, external_id);

-- Step 5: Add indexes
CREATE INDEX idx_activities_user_id ON activities(user_id);
CREATE INDEX idx_workouts_user_id ON workouts(user_id);
CREATE INDEX idx_transactions_user_id ON transactions(user_id);
CREATE INDEX idx_accounts_user_id ON accounts(user_id);
CREATE INDEX idx_nutrition_daily_user_id ON nutrition_daily(user_id);
CREATE INDEX idx_raw_events_user_id ON raw_events(user_id);
CREATE INDEX idx_zenmoney_tags_user_id ON zenmoney_tags(user_id);

-- Step 6: Recreate timeline view
DROP VIEW IF EXISTS timeline;
CREATE VIEW timeline AS
    SELECT 'workout'::text AS event_type, id, user_id, started_at AS occurred_at, title AS summary FROM workouts
    UNION ALL
    SELECT 'activity', id, user_id, started_at, name FROM activities
    UNION ALL
    SELECT 'journal', id, user_id, date::timestamptz, title FROM journal_entries WHERE date IS NOT NULL
    UNION ALL
    SELECT 'transaction', id, user_id, occurred_at, COALESCE(category, 'uncategorized') || ': ' || amount::text FROM transactions
    ORDER BY occurred_at DESC;
