ALTER TABLE location_visits DROP COLUMN IF EXISTS place_id;
ALTER TABLE location_visits ADD COLUMN place_name TEXT;
ALTER TABLE location_visits ADD COLUMN place_kind VARCHAR(50);
ALTER TABLE location_visits ADD COLUMN place_source VARCHAR(30);
DROP TABLE IF EXISTS location_places;
