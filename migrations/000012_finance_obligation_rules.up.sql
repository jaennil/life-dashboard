CREATE TABLE finance_obligation_rules (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    match_key VARCHAR(255) NOT NULL,
    match_label VARCHAR(255) NOT NULL,
    action VARCHAR(20) NOT NULL CHECK (action IN ('ignore', 'force')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, match_key)
);

CREATE INDEX idx_finance_obligation_rules_user_id
    ON finance_obligation_rules(user_id);
