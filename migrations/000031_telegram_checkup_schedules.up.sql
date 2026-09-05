-- Telegram delivery for checkups, plus the schedules that trigger them.
--
-- The bot is server-wide: a chat is bound to an account by sending the bot a
-- one-time code, so the account never has to hold a bot token of its own.
CREATE TABLE telegram_accounts (
    user_id   uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    chat_id   bigint NOT NULL,
    username  varchar(255),
    linked_at timestamptz NOT NULL DEFAULT NOW(),
    UNIQUE (chat_id)
);

-- Codes are short-lived on purpose: anyone who sees one could bind their own
-- chat to this account and start receiving its reports.
CREATE TABLE telegram_link_codes (
    code       varchar(32) PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    expires_at timestamptz NOT NULL,
    used_at    timestamptz
);

CREATE INDEX idx_telegram_link_codes_user ON telegram_link_codes (user_id, created_at DESC);

-- getUpdates is a cursor: without a persisted offset a restart would replay
-- every message Telegram still holds, up to a day of them.
CREATE TABLE telegram_poll_state (
    singleton      boolean PRIMARY KEY DEFAULT TRUE,
    last_update_id bigint NOT NULL DEFAULT 0,
    updated_at     timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT telegram_poll_state_singleton_check CHECK (singleton)
);

INSERT INTO telegram_poll_state (singleton, last_update_id) VALUES (TRUE, 0);

CREATE TABLE checkup_schedules (
    id           uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- One of the checkup periods the report itself understands.
    period       varchar(20) NOT NULL,
    enabled      boolean NOT NULL DEFAULT FALSE,
    hour         smallint NOT NULL DEFAULT 21,
    minute       smallint NOT NULL DEFAULT 0,
    -- Weekly runs on this weekday (0 = Sunday, matching Go's time.Weekday);
    -- monthly runs on this day of month. Both are null for the daily schedule.
    weekday      smallint,
    day_of_month smallint,
    last_run_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT NOW(),
    updated_at   timestamptz NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, period),
    CONSTRAINT checkup_schedules_period_check CHECK (period IN ('today', 'week', 'month')),
    CONSTRAINT checkup_schedules_hour_check CHECK (hour BETWEEN 0 AND 23),
    CONSTRAINT checkup_schedules_minute_check CHECK (minute BETWEEN 0 AND 59),
    CONSTRAINT checkup_schedules_weekday_check CHECK (weekday IS NULL OR weekday BETWEEN 0 AND 6),
    -- Capped at 28 so every month actually has the day.
    CONSTRAINT checkup_schedules_day_check CHECK (day_of_month IS NULL OR day_of_month BETWEEN 1 AND 28)
);

CREATE INDEX idx_checkup_schedules_enabled ON checkup_schedules (enabled, period);
