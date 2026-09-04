DROP INDEX IF EXISTS idx_input_jobs_notification_claim;
ALTER TABLE input_jobs
    DROP CONSTRAINT IF EXISTS input_jobs_notification_status_check,
    DROP COLUMN IF EXISTS notification_error,
    DROP COLUMN IF EXISTS notification_sent_at,
    DROP COLUMN IF EXISTS notification_available_at,
    DROP COLUMN IF EXISTS notification_attempts,
    DROP COLUMN IF EXISTS notification_status;
