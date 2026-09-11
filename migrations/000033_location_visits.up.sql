-- Visits as iOS itself reports them: the phone decides it has settled in one
-- place and later that it has left. Detecting that on our side from sparse
-- background fixes would be guesswork; CoreLocation already knows.
CREATE TABLE location_visits (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source       VARCHAR(30) NOT NULL DEFAULT 'overland',
    arrived_at   TIMESTAMPTZ NOT NULL,
    -- NULL while the visit is still open: the arrival is reported first and the
    -- departure lands in a later batch.
    departed_at  TIMESTAMPTZ,
    latitude     DOUBLE PRECISION NOT NULL,
    longitude    DOUBLE PRECISION NOT NULL,
    accuracy_m   DOUBLE PRECISION,
    wifi         VARCHAR(255),
    device_id    VARCHAR(100),
    -- Naming a place is a separate step with its own failure modes, so a visit
    -- is stored whether or not anything could name it.
    place_name   TEXT,
    place_kind   VARCHAR(50),
    place_source VARCHAR(30),
    raw          JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, source, arrived_at)
);

CREATE INDEX idx_location_visits_user_time ON location_visits (user_id, arrived_at DESC);
CREATE INDEX idx_location_visits_unnamed ON location_visits (user_id, arrived_at DESC) WHERE place_name IS NULL;
