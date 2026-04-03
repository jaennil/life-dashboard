CREATE TABLE IF NOT EXISTS calendar_events (
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
