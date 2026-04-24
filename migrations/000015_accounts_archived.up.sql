ALTER TABLE accounts
    ADD COLUMN archived BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX idx_accounts_user_archived
    ON accounts(user_id, archived);
