-- Naming a place is a guess with a confidence, not a fact. The candidates are
-- kept so the guess can be corrected by hand instead of re-queried, and
-- named_at marks a place as already looked at even when nothing was found.
ALTER TABLE location_places ADD COLUMN candidates JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE location_places ADD COLUMN named_at TIMESTAMPTZ;

CREATE INDEX idx_location_places_unnamed ON location_places (user_id) WHERE named_at IS NULL;
