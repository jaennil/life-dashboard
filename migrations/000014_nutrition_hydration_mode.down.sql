DROP INDEX IF EXISTS idx_nutrition_hydration_entries_user_date;
DROP TABLE IF EXISTS nutrition_hydration_entries;

ALTER TABLE nutrition_targets
    DROP COLUMN IF EXISTS hydration_mode;
