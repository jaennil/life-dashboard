ALTER TABLE users
ADD COLUMN IF NOT EXISTS last_active_at TIMESTAMPTZ;

UPDATE users u
SET last_active_at = GREATEST(
    u.created_at,
    COALESCE((SELECT MAX(created_at) FROM ai_chat_messages WHERE user_id = u.id), u.created_at),
    COALESCE((SELECT MAX(created_at) FROM ai_checkup_reports WHERE user_id = u.id), u.created_at)
)
WHERE last_active_at IS NULL;

ALTER TABLE sync_state
ADD COLUMN IF NOT EXISTS consecutive_failures INTEGER NOT NULL DEFAULT 0;

ALTER TABLE sync_state
ADD COLUMN IF NOT EXISTS last_failed_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_users_last_active_at
ON users(last_active_at);

CREATE INDEX IF NOT EXISTS idx_sync_state_source_enabled
ON sync_state(source, enabled);
