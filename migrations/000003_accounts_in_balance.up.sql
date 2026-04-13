ALTER TABLE accounts
    ADD COLUMN in_balance BOOLEAN NOT NULL DEFAULT TRUE;

CREATE INDEX idx_accounts_user_in_balance
    ON accounts(user_id, in_balance);
