CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE sync_state (
    source VARCHAR(50) PRIMARY KEY,
    last_synced_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE raw_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source VARCHAR(50) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    external_id VARCHAR(255),
    payload JSONB NOT NULL,
    ingested_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_raw_events_source ON raw_events(source);
CREATE INDEX idx_raw_events_ingested_at ON raw_events(ingested_at);

CREATE TABLE workouts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source VARCHAR(20) NOT NULL DEFAULT 'hevy',
    external_id VARCHAR(255) UNIQUE NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    title VARCHAR(255),
    notes TEXT,
    raw_payload JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_workouts_started_at ON workouts(started_at);

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

CREATE TABLE activities (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source VARCHAR(20) NOT NULL DEFAULT 'strava',
    external_id VARCHAR(255) UNIQUE NOT NULL,
    type VARCHAR(50) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    duration_seconds INTEGER,
    distance_meters NUMERIC(10,2),
    elevation_gain_meters NUMERIC(8,2),
    avg_heart_rate INTEGER,
    max_heart_rate INTEGER,
    avg_power_watts INTEGER,
    avg_cadence NUMERIC(5,1),
    calories INTEGER,
    name VARCHAR(255),
    description TEXT,
    raw_payload JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_activities_started_at ON activities(started_at);
CREATE INDEX idx_activities_type ON activities(type);

CREATE TABLE nutrition_daily (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    date DATE UNIQUE NOT NULL,
    calories_total NUMERIC(8,2),
    protein_g NUMERIC(6,2),
    carbs_g NUMERIC(6,2),
    fat_g NUMERIC(6,2),
    fiber_g NUMERIC(6,2),
    water_ml NUMERIC(8,2),
    source VARCHAR(20) DEFAULT 'fatsecret',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE nutrition_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    daily_id UUID REFERENCES nutrition_daily(id) ON DELETE CASCADE,
    meal_type VARCHAR(20),
    food_name VARCHAR(255),
    serving_description VARCHAR(255),
    calories NUMERIC(7,2),
    macros JSONB
);

CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    external_id VARCHAR(255) UNIQUE NOT NULL,
    title VARCHAR(255) NOT NULL,
    type VARCHAR(50),
    currency VARCHAR(3) NOT NULL,
    balance NUMERIC(15,2),
    last_updated TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    external_id VARCHAR(255) UNIQUE NOT NULL,
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
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_transactions_occurred_at ON transactions(occurred_at);
CREATE INDEX idx_transactions_category ON transactions(category);

CREATE TABLE biometrics (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    timestamp TIMESTAMPTZ NOT NULL,
    source VARCHAR(30) NOT NULL,
    metric_type VARCHAR(50) NOT NULL,
    value NUMERIC NOT NULL,
    unit VARCHAR(20),
    metadata JSONB
);
CREATE INDEX idx_biometrics_timestamp ON biometrics USING BRIN(timestamp);
CREATE INDEX idx_biometrics_metric_type ON biometrics(metric_type);
CREATE UNIQUE INDEX idx_biometrics_unique ON biometrics(timestamp, source, metric_type);

CREATE TABLE sleep_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
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
    UNIQUE(source, date)
);
CREATE INDEX idx_sleep_sessions_date ON sleep_sessions(date);

CREATE TABLE sleep_stages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID REFERENCES sleep_sessions(id) ON DELETE CASCADE,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ NOT NULL,
    stage VARCHAR(10) NOT NULL
);

CREATE TABLE journal_entries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
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
    UNIQUE(source, external_id)
);
CREATE INDEX idx_journal_entries_date ON journal_entries(date);

CREATE VIEW timeline AS
    SELECT 'workout'::text AS event_type, id, started_at AS occurred_at, title AS summary FROM workouts
    UNION ALL
    SELECT 'activity', id, started_at, name FROM activities
    UNION ALL
    SELECT 'journal', id, date::timestamptz, title FROM journal_entries WHERE date IS NOT NULL
    UNION ALL
    SELECT 'transaction', id, occurred_at, COALESCE(category, 'uncategorized') || ': ' || amount::text FROM transactions
    ORDER BY occurred_at DESC;
