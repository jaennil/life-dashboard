DROP INDEX IF EXISTS idx_nutrition_items_food_id;
ALTER TABLE nutrition_items DROP COLUMN IF EXISTS number_of_units;
ALTER TABLE nutrition_items DROP COLUMN IF EXISTS serving_id;
ALTER TABLE nutrition_items DROP COLUMN IF EXISTS food_id;
