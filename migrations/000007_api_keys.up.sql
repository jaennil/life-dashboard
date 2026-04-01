CREATE TABLE api_keys (
    user_id UUID PRIMARY KEY REFERENCES users(id),
    key VARCHAR(128) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_api_keys_key ON api_keys(key);
