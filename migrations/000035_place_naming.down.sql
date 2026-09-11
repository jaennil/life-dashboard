DROP INDEX IF EXISTS idx_location_places_unnamed;
ALTER TABLE location_places DROP COLUMN IF EXISTS candidates;
ALTER TABLE location_places DROP COLUMN IF EXISTS named_at;
