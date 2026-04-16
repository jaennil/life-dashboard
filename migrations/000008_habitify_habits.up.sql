CREATE TABLE habits (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    source VARCHAR(20) NOT NULL DEFAULT 'habitify',
    external_id VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    area_external_id VARCHAR(255),
    area_name VARCHAR(255),
    archived BOOLEAN NOT NULL DEFAULT FALSE,
    recurrence VARCHAR(50),
    log_method VARCHAR(50),
    time_of_day TEXT[] DEFAULT '{}',
    remind_at TEXT[] DEFAULT '{}',
    goal JSONB,
    goal_history_items JSONB,
    raw_payload JSONB,
    source_created_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, source, external_id)
);

CREATE INDEX idx_habits_user_source ON habits(user_id, source);
CREATE INDEX idx_habits_user_archived ON habits(user_id, archived);

CREATE TABLE habit_daily_statuses (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    habit_id UUID NOT NULL REFERENCES habits(id) ON DELETE CASCADE,
    target_date DATE NOT NULL,
    status VARCHAR(50) NOT NULL,
    current_value NUMERIC(10,2),
    target_value NUMERIC(10,2),
    unit_type VARCHAR(50),
    periodicity VARCHAR(50),
    raw_payload JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(habit_id, target_date)
);

CREATE INDEX idx_habit_daily_statuses_habit_id ON habit_daily_statuses(habit_id);
CREATE INDEX idx_habit_daily_statuses_target_date ON habit_daily_statuses(target_date);
