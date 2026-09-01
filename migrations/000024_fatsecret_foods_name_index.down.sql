DROP INDEX IF EXISTS idx_fatsecret_foods_name;
CREATE INDEX IF NOT EXISTS idx_fatsecret_foods_name ON fatsecret_foods (user_id, lower(food_name));
