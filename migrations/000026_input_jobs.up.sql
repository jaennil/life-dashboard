CREATE TABLE input_jobs (
    id           uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    raw_event_id uuid REFERENCES raw_events(id) ON DELETE SET NULL,
    text         text NOT NULL,
    finish       boolean NOT NULL DEFAULT FALSE,
    duration_minutes integer NOT NULL DEFAULT 0,
    typed        boolean NOT NULL DEFAULT FALSE,
    status       varchar(20) NOT NULL DEFAULT 'queued',
    attempts     integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL DEFAULT NOW(),
    locked_until timestamptz,
    result       jsonb,
    last_error   text,
    created_at   timestamptz NOT NULL DEFAULT NOW(),
    started_at   timestamptz,
    completed_at timestamptz,
    updated_at   timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT input_jobs_status_check
        CHECK (status IN ('queued', 'processing', 'succeeded', 'failed'))
);

CREATE INDEX idx_input_jobs_claim
    ON input_jobs (status, available_at, created_at);
CREATE INDEX idx_input_jobs_user_created
    ON input_jobs (user_id, created_at DESC);

CREATE TABLE web_push_subscriptions (
    id         uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint   text NOT NULL,
    p256dh     text NOT NULL,
    auth       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, endpoint)
);

CREATE INDEX idx_web_push_subscriptions_user
    ON web_push_subscriptions (user_id);
