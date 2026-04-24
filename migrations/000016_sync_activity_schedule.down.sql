DROP INDEX IF EXISTS idx_sync_state_source_enabled;
DROP INDEX IF EXISTS idx_users_last_active_at;

ALTER TABLE sync_state
DROP COLUMN IF EXISTS last_failed_at;

ALTER TABLE sync_state
DROP COLUMN IF EXISTS consecutive_failures;

ALTER TABLE users
DROP COLUMN IF EXISTS last_active_at;
