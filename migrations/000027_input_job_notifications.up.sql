ALTER TABLE input_jobs
    ADD COLUMN notification_status varchar(20) NOT NULL DEFAULT 'pending',
    ADD COLUMN notification_attempts integer NOT NULL DEFAULT 0,
    ADD COLUMN notification_available_at timestamptz NOT NULL DEFAULT NOW(),
    ADD COLUMN notification_sent_at timestamptz,
    ADD COLUMN notification_error text,
    ADD CONSTRAINT input_jobs_notification_status_check
        CHECK (notification_status IN ('pending', 'sending', 'sent'));

-- Do not send historical successes when push is first deployed. Failed jobs are
-- deliberately left pending so the failure that exposed the broken VAPID token
-- is delivered after the authentication fix rolls out.
UPDATE input_jobs
SET notification_status = 'sent', notification_sent_at = NOW()
WHERE status <> 'failed';

CREATE INDEX idx_input_jobs_notification_claim
    ON input_jobs (notification_status, notification_available_at, completed_at)
    WHERE notification_status <> 'sent';
