CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS vector;

-- Users
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(50) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    totp_secret VARCHAR(255),
    totp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Sync state
CREATE TABLE sync_state (
    source VARCHAR(50) NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id),
    last_synced_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (source, user_id)
);

-- OAuth tokens
CREATE TABLE oauth_tokens (
    source VARCHAR(50) NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id),
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    athlete_id BIGINT,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (source, user_id)
);

-- API keys for webhooks
CREATE TABLE api_keys (
    user_id UUID PRIMARY KEY REFERENCES users(id),
    key VARCHAR(128) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_api_keys_key ON api_keys(key);

-- Raw events
CREATE TABLE raw_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    source VARCHAR(50) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    external_id VARCHAR(255),
    payload JSONB NOT NULL,
    ingested_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_raw_events_source ON raw_events(source);
CREATE INDEX idx_raw_events_user_id ON raw_events(user_id);

-- Workouts
CREATE TABLE workouts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    source VARCHAR(20) NOT NULL DEFAULT 'hevy',
    external_id VARCHAR(255) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    title VARCHAR(255),
    notes TEXT,
    raw_payload JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, external_id)
);
CREATE INDEX idx_workouts_started_at ON workouts(started_at);
CREATE INDEX idx_workouts_user_id ON workouts(user_id);

-- Workout sets
CREATE TABLE workout_sets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    workout_id UUID REFERENCES workouts(id) ON DELETE CASCADE,
    exercise_name VARCHAR(255) NOT NULL,
    exercise_category VARCHAR(100),
    set_index INTEGER NOT NULL,
    set_type VARCHAR(20) DEFAULT 'normal',
    weight_kg NUMERIC(6,2),
    reps INTEGER,
    duration_seconds INTEGER,
    rpe NUMERIC(3,1)
);
CREATE INDEX idx_workout_sets_workout_id ON workout_sets(workout_id);

-- Activities
CREATE TABLE activities (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    source VARCHAR(20) NOT NULL DEFAULT 'strava',
    external_id VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    sport_type VARCHAR(50),
    started_at TIMESTAMPTZ NOT NULL,
    duration_seconds INTEGER,
    elapsed_time INTEGER,
    distance_meters NUMERIC(10,2),
    elevation_gain_meters NUMERIC(8,2),
    avg_heart_rate INTEGER,
    max_heart_rate INTEGER,
    avg_power_watts INTEGER,
    avg_cadence NUMERIC(5,1),
    calories INTEGER,
    average_speed NUMERIC(8,3),
    max_speed NUMERIC(8,3),
    average_temp NUMERIC(5,1),
    weighted_average_watts INTEGER,
    max_watts INTEGER,
    kilojoules NUMERIC(8,1),
    elev_high NUMERIC(8,2),
    elev_low NUMERIC(8,2),
    suffer_score INTEGER,
    pr_count INTEGER,
    workout_type INTEGER,
    device_name VARCHAR(255),
    gear_name VARCHAR(255),
    start_lat NUMERIC(10,6),
    start_lng NUMERIC(10,6),
    map_summary_polyline TEXT,
    name VARCHAR(255),
    description TEXT,
    raw_payload JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, external_id)
);
CREATE INDEX idx_activities_started_at ON activities(started_at);
CREATE INDEX idx_activities_type ON activities(type);
CREATE INDEX idx_activities_user_id ON activities(user_id);

-- Activity splits
CREATE TABLE activity_splits (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    activity_id UUID REFERENCES activities(id) ON DELETE CASCADE,
    split INTEGER NOT NULL,
    distance NUMERIC(10,2),
    elapsed_time INTEGER,
    moving_time INTEGER,
    elevation_difference NUMERIC(6,1),
    average_speed NUMERIC(8,3),
    pace_zone INTEGER
);
CREATE INDEX idx_activity_splits_activity_id ON activity_splits(activity_id);

-- Activity laps
CREATE TABLE activity_laps (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    activity_id UUID REFERENCES activities(id) ON DELETE CASCADE,
    lap_index INTEGER NOT NULL,
    name VARCHAR(255),
    distance NUMERIC(10,2),
    elapsed_time INTEGER,
    moving_time INTEGER,
    start_date TIMESTAMPTZ,
    total_elevation_gain NUMERIC(8,2),
    average_speed NUMERIC(8,3),
    max_speed NUMERIC(8,3),
    average_cadence NUMERIC(5,1),
    average_watts NUMERIC(8,1)
);
CREATE INDEX idx_activity_laps_activity_id ON activity_laps(activity_id);

-- Nutrition daily
CREATE TABLE nutrition_daily (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    date DATE NOT NULL,
    calories_total NUMERIC(8,2),
    protein_g NUMERIC(6,2),
    carbs_g NUMERIC(6,2),
    fat_g NUMERIC(6,2),
    fiber_g NUMERIC(6,2),
    water_ml NUMERIC(8,2),
    source VARCHAR(20) DEFAULT 'fatsecret',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, date)
);
CREATE INDEX idx_nutrition_daily_user_id ON nutrition_daily(user_id);

-- Nutrition items
CREATE TABLE nutrition_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    daily_id UUID REFERENCES nutrition_daily(id) ON DELETE CASCADE,
    meal_type VARCHAR(20),
    food_name VARCHAR(255),
    serving_description VARCHAR(255),
    calories NUMERIC(7,2),
    macros JSONB
);

-- Accounts
CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    external_id VARCHAR(255) NOT NULL,
    title VARCHAR(255) NOT NULL,
    type VARCHAR(50),
    currency VARCHAR(3) NOT NULL,
    balance NUMERIC(15,2),
    last_updated TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, external_id)
);
CREATE INDEX idx_accounts_user_id ON accounts(user_id);

-- Transactions
CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    external_id VARCHAR(255) NOT NULL,
    account_id UUID REFERENCES accounts(id),
    occurred_at TIMESTAMPTZ NOT NULL,
    amount NUMERIC(15,2) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    category VARCHAR(100),
    subcategory VARCHAR(100),
    payee VARCHAR(255),
    comment TEXT,
    tags TEXT[],
    is_transfer BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, external_id)
);
CREATE INDEX idx_transactions_occurred_at ON transactions(occurred_at);
CREATE INDEX idx_transactions_category ON transactions(category);
CREATE INDEX idx_transactions_user_id ON transactions(user_id);

-- ZenMoney tags
CREATE TABLE zenmoney_tags (
    id VARCHAR(36) PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    title VARCHAR(255) NOT NULL,
    parent_id VARCHAR(36),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_zenmoney_tags_user_id ON zenmoney_tags(user_id);

-- Biometrics
CREATE TABLE biometrics (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    timestamp TIMESTAMPTZ NOT NULL,
    source VARCHAR(30) NOT NULL,
    metric_type VARCHAR(50) NOT NULL,
    value NUMERIC NOT NULL,
    unit VARCHAR(20),
    metadata JSONB
);
CREATE INDEX idx_biometrics_timestamp ON biometrics USING BRIN(timestamp);
CREATE INDEX idx_biometrics_metric_type ON biometrics(metric_type);
CREATE UNIQUE INDEX idx_biometrics_unique ON biometrics(user_id, timestamp, source, metric_type);

-- Sleep sessions
CREATE TABLE sleep_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    source VARCHAR(30) NOT NULL,
    date DATE NOT NULL,
    sleep_start TIMESTAMPTZ,
    sleep_end TIMESTAMPTZ,
    total_sleep_minutes INTEGER,
    deep_sleep_minutes INTEGER,
    light_sleep_minutes INTEGER,
    rem_sleep_minutes INTEGER,
    awake_minutes INTEGER,
    sleep_score INTEGER,
    avg_hrv NUMERIC(6,2),
    avg_resting_hr INTEGER,
    raw_payload JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, source, date)
);
CREATE INDEX idx_sleep_sessions_date ON sleep_sessions(date);

-- Sleep stages
CREATE TABLE sleep_stages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID REFERENCES sleep_sessions(id) ON DELETE CASCADE,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ NOT NULL,
    stage VARCHAR(10) NOT NULL
);

-- Journal entries
CREATE TABLE journal_entries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    source VARCHAR(20) NOT NULL,
    external_id VARCHAR(255),
    date DATE,
    title VARCHAR(500),
    content TEXT,
    content_embedding vector(768),
    tags TEXT[],
    mood INTEGER,
    metadata JSONB,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    ingested_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, source, external_id)
);
CREATE INDEX idx_journal_entries_date ON journal_entries(date);

-- Calendar events
CREATE TABLE calendar_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    external_id VARCHAR(255) NOT NULL,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ,
    all_day BOOLEAN DEFAULT FALSE,
    location VARCHAR(500),
    source VARCHAR(50) DEFAULT 'google',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, external_id)
);
CREATE INDEX idx_calendar_events_user_start ON calendar_events(user_id, start_time);

-- Timeline view
CREATE VIEW timeline AS
    SELECT 'workout'::text AS event_type, id, user_id, started_at AS occurred_at, title AS summary FROM workouts
    UNION ALL
    SELECT 'activity', id, user_id, started_at, name FROM activities
    UNION ALL
    SELECT 'journal', id, user_id, date::timestamptz, title FROM journal_entries WHERE date IS NOT NULL
    UNION ALL
    SELECT 'transaction', id, user_id, occurred_at, COALESCE(category, 'uncategorized') || ': ' || amount::text FROM transactions
    ORDER BY occurred_at DESC;
