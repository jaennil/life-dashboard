-- Fix the name index: it could not match a Russian food name.
--
-- The database runs with LC_COLLATE = C, where lower() folds ASCII only, so
-- lower('Банан') is 'Банан' and the index built on lower(food_name) never
-- matched a lowercase query. Every food in this catalogue except one is named in
-- Russian, so the index was useless for the whole table.
--
-- An explicit ICU collation folds Cyrillic correctly, and queries have to use the
-- same expression to hit the index.
DROP INDEX IF EXISTS idx_fatsecret_foods_name;
CREATE INDEX IF NOT EXISTS idx_fatsecret_foods_name
    ON fatsecret_foods (user_id, lower(food_name COLLATE "ru-RU-x-icu"));
