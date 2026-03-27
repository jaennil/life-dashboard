ALTER TABLE sync_state ADD COLUMN IF NOT EXISTS enabled boolean NOT NULL DEFAULT true;

INSERT INTO sync_state (source, last_synced_at, updated_at, enabled)
  VALUES ('hevy', NULL, NOW(), true)
  ON CONFLICT (source) DO NOTHING;
