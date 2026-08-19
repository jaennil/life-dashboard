-- Marks accounts whose syncs must never be deprioritized.
--
-- The scheduler normally slows a user down as they stop opening the dashboard
-- (hot -> warm -> cold -> dormant, where dormant stops syncing altogether) and
-- backs off hard after failures. For the owner's own account that is the wrong
-- trade-off: the data should keep arriving whether or not they visited today.
ALTER TABLE users
    ADD COLUMN sync_priority BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN users.sync_priority IS
    'Treat this user as always active: shortest interval per source, no dormancy, minimal failure backoff.';

-- The owner's account. Scoped by username so re-running on a fresh database
-- reproduces the same intent rather than silently leaving it unset.
UPDATE users SET sync_priority = TRUE WHERE username = 'jaennil';
