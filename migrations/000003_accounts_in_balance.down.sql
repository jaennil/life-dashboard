DROP INDEX IF EXISTS idx_accounts_user_in_balance;

ALTER TABLE accounts
    DROP COLUMN IF EXISTS in_balance;
