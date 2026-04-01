-- Add missing fields to activities
ALTER TABLE activities ADD COLUMN IF NOT EXISTS sport_type VARCHAR(50);
ALTER TABLE activities ADD COLUMN IF NOT EXISTS elapsed_time INTEGER;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS average_speed NUMERIC(8,3);
ALTER TABLE activities ADD COLUMN IF NOT EXISTS max_speed NUMERIC(8,3);
ALTER TABLE activities ADD COLUMN IF NOT EXISTS average_temp NUMERIC(5,1);
ALTER TABLE activities ADD COLUMN IF NOT EXISTS weighted_average_watts INTEGER;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS max_watts INTEGER;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS kilojoules NUMERIC(8,1);
ALTER TABLE activities ADD COLUMN IF NOT EXISTS elev_high NUMERIC(8,2);
ALTER TABLE activities ADD COLUMN IF NOT EXISTS elev_low NUMERIC(8,2);
ALTER TABLE activities ADD COLUMN IF NOT EXISTS suffer_score INTEGER;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS pr_count INTEGER;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS workout_type INTEGER;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS device_name VARCHAR(255);
ALTER TABLE activities ADD COLUMN IF NOT EXISTS gear_name VARCHAR(255);
ALTER TABLE activities ADD COLUMN IF NOT EXISTS start_lat NUMERIC(10,6);
ALTER TABLE activities ADD COLUMN IF NOT EXISTS start_lng NUMERIC(10,6);
ALTER TABLE activities ADD COLUMN IF NOT EXISTS map_summary_polyline TEXT;

-- Activity splits (per-km breakdown)
CREATE TABLE IF NOT EXISTS activity_splits (
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
CREATE INDEX IF NOT EXISTS idx_activity_splits_activity_id ON activity_splits(activity_id);

-- Activity laps
CREATE TABLE IF NOT EXISTS activity_laps (
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
CREATE INDEX IF NOT EXISTS idx_activity_laps_activity_id ON activity_laps(activity_id);
