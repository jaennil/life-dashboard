-- Keep the provider identifiers on a diary item.
--
-- food_entries.get returns food_id, serving_id and number_of_units, and the
-- connector was discarding all three - so the account's own food history existed
-- locally as names only, and a dictated meal could not be resolved to anything
-- writable. The history endpoints cover only the top twenty foods per meal, which
-- is roughly a third of what has actually been logged; these columns are how the
-- rest becomes reachable.
--
-- number_of_units is kept because it is the quantity the entry was logged with,
-- which is the natural default when the same food is dictated again.
ALTER TABLE nutrition_items ADD COLUMN IF NOT EXISTS food_id         varchar(64);
ALTER TABLE nutrition_items ADD COLUMN IF NOT EXISTS serving_id      varchar(64);
ALTER TABLE nutrition_items ADD COLUMN IF NOT EXISTS number_of_units numeric(10,3);

CREATE INDEX IF NOT EXISTS idx_nutrition_items_food_id ON nutrition_items (food_id)
    WHERE food_id IS NOT NULL;
