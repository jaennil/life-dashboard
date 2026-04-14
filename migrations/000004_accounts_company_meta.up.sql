ALTER TABLE accounts
    ADD COLUMN company_id INTEGER,
    ADD COLUMN company_title TEXT;

CREATE INDEX idx_accounts_user_company_id
    ON accounts(user_id, company_id);
