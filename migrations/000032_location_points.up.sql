-- Raw location fixes as they arrive from the phone. One row per reported point:
-- visits are derived from these later, and keeping the fixes means a better
-- visit detector can be run over the same history without asking for it again.
CREATE TABLE location_points (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source        VARCHAR(30) NOT NULL DEFAULT 'overland',
    recorded_at   TIMESTAMPTZ NOT NULL,
    latitude      DOUBLE PRECISION NOT NULL,
    longitude     DOUBLE PRECISION NOT NULL,
    accuracy_m    DOUBLE PRECISION,
    altitude_m    DOUBLE PRECISION,
    speed_ms      DOUBLE PRECISION,
    course_deg    DOUBLE PRECISION,
    -- iOS reports several motion kinds at once: ["driving","stationary"].
    motion        TEXT[] NOT NULL DEFAULT '{}',
    battery_level DOUBLE PRECISION,
    battery_state VARCHAR(20),
    wifi          VARCHAR(255),
    device_id     VARCHAR(100),
    -- The properties object as sent, so a field we do not model yet is not lost.
    raw           JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- The phone resends a batch until it is acknowledged, so the same fix can
    -- arrive several times.
    UNIQUE (user_id, source, recorded_at)
);

CREATE INDEX idx_location_points_user_time ON location_points (user_id, recorded_at DESC);
