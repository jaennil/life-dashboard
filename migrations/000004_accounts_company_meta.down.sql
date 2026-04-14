DROP INDEX IF EXISTS idx_accounts_user_company_id;

ALTER TABLE accounts
    DROP COLUMN IF EXISTS company_title,
    DROP COLUMN IF EXISTS company_id;
