DROP INDEX IF EXISTS idx_accounts_user_archived;

ALTER TABLE accounts
    DROP COLUMN IF EXISTS archived;
