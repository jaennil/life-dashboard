-- A place is where several visits landed. iOS reports each visit as bare
-- coordinates that drift by tens of metres between stays, so the same kitchen
-- table arrives as a dozen slightly different points; grouping them is what
-- turns "55.7558, 37.6176" into somewhere with a name and a history.
CREATE TABLE location_places (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Running centroid of the visits that belong here.
    latitude    DOUBLE PRECISION NOT NULL,
    longitude   DOUBLE PRECISION NOT NULL,
    -- What this place is: home, work or other. Derived from when the visits
    -- happen, so it needs no map service and no configuration.
    kind        VARCHAR(20) NOT NULL DEFAULT 'other',
    -- name is whatever we could learn: a geocoded address or POI. custom_name
    -- is the user's own word for it and always wins.
    name        TEXT,
    custom_name TEXT,
    name_source VARCHAR(30),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_location_places_user ON location_places (user_id);

-- The speculative naming columns on visits are replaced by the place a visit
-- belongs to.
ALTER TABLE location_visits DROP COLUMN place_name;
ALTER TABLE location_visits DROP COLUMN place_kind;
ALTER TABLE location_visits DROP COLUMN place_source;
ALTER TABLE location_visits ADD COLUMN place_id UUID REFERENCES location_places(id) ON DELETE SET NULL;

CREATE INDEX idx_location_visits_place ON location_visits (place_id, arrived_at DESC);
CREATE INDEX idx_location_visits_unplaced ON location_visits (user_id, arrived_at) WHERE place_id IS NULL;
