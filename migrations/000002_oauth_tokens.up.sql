CREATE TABLE oauth_tokens (
    source       VARCHAR(50) PRIMARY KEY,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    athlete_id   BIGINT,
    updated_at   TIMESTAMPTZ DEFAULT NOW()
);
