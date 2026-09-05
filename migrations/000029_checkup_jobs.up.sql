-- Moves checkup generation off the request that asked for it.
--
-- The report was streamed straight into the HTTP response and only stored once
-- the model finished. A phone that locked its screen mid-generation therefore
-- lost the whole report: the request context was canceled, the upstream read
-- stopped, and nothing was written. The job survives the connection instead,
-- and the notification columns mirror input_jobs so a finished report can be
-- pushed to a closed app.
CREATE TABLE ai_checkup_jobs (
    id               uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    requested_period varchar(20) NOT NULL,
    status           varchar(20) NOT NULL DEFAULT 'queued',
    attempts         integer NOT NULL DEFAULT 0,
    available_at     timestamptz NOT NULL DEFAULT NOW(),
    locked_until     timestamptz,
    content          text,
    last_error       text,
    notification_status       varchar(20) NOT NULL DEFAULT 'pending',
    notification_attempts     integer NOT NULL DEFAULT 0,
    notification_available_at timestamptz NOT NULL DEFAULT NOW(),
    notification_sent_at      timestamptz,
    notification_error        text,
    created_at   timestamptz NOT NULL DEFAULT NOW(),
    started_at   timestamptz,
    completed_at timestamptz,
    updated_at   timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT ai_checkup_jobs_status_check
        CHECK (status IN ('queued', 'processing', 'succeeded', 'failed')),
    CONSTRAINT ai_checkup_jobs_notification_status_check
        CHECK (notification_status IN ('pending', 'sending', 'sent'))
);

CREATE INDEX idx_ai_checkup_jobs_claim
    ON ai_checkup_jobs (status, available_at, created_at);
CREATE INDEX idx_ai_checkup_jobs_user_created
    ON ai_checkup_jobs (user_id, created_at DESC);
CREATE INDEX idx_ai_checkup_jobs_notification_claim
    ON ai_checkup_jobs (notification_status, notification_available_at, completed_at)
    WHERE notification_status <> 'sent';
