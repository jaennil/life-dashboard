-- Daily rollup of iOS Screen Time, one row per (user, source, day).
CREATE TABLE screen_time_daily (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    source VARCHAR(30) NOT NULL DEFAULT 'ios_screentime',
    day DATE NOT NULL,
    -- Sum of kind='app' rows only. Website time is already counted inside the
    -- browser app's own total (Safari 1h13m includes example.com 47m), so the
    -- two columns must never be added together.
    app_seconds INTEGER NOT NULL DEFAULT 0 CHECK (app_seconds >= 0),
    website_seconds INTEGER NOT NULL DEFAULT 0 CHECK (website_seconds >= 0),
    app_count INTEGER NOT NULL DEFAULT 0,
    website_count INTEGER NOT NULL DEFAULT 0,
    -- Lines the parser could not read, so a silently degraded payload is visible.
    unparsed_lines INTEGER NOT NULL DEFAULT 0,
    -- iOS 26 occasionally reports absurd durations; per-item values above 24h
    -- are clamped and the day is flagged rather than dropped.
    clamped BOOLEAN NOT NULL DEFAULT FALSE,
    -- TRUE while the day is still in progress (payload pulled with during=today).
    is_partial BOOLEAN NOT NULL DEFAULT FALSE,
    raw_payload JSONB,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, source, day)
);

CREATE INDEX idx_screen_time_daily_user_day ON screen_time_daily(user_id, day DESC);

-- Per-app and per-website daily totals.
CREATE TABLE screen_time_app_usage (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    source VARCHAR(30) NOT NULL DEFAULT 'ios_screentime',
    day DATE NOT NULL,
    kind VARCHAR(10) NOT NULL DEFAULT 'app' CHECK (kind IN ('app', 'website')),
    -- Normalized lowercase name or hostname. Screen Time gives display names,
    -- not bundle ids, so this is the most stable key available.
    item_key VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    seconds INTEGER NOT NULL CHECK (seconds >= 0 AND seconds <= 86400),
    -- TRUE when app/website was guessed from the combined list rather than
    -- taken from a dedicated apps-only or websites-only payload field.
    kind_inferred BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, source, day, kind, item_key)
);

CREATE INDEX idx_screen_time_app_usage_user_day ON screen_time_app_usage(user_id, day DESC);
CREATE INDEX idx_screen_time_app_usage_user_item ON screen_time_app_usage(user_id, item_key, day DESC);
CREATE INDEX idx_screen_time_app_usage_user_top ON screen_time_app_usage(user_id, day DESC, seconds DESC);
