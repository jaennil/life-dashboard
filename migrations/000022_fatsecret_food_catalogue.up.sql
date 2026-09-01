-- The account's own food catalogue, mirrored from FatSecret.
--
-- Dictated food has to be resolved to a food_id before it can be written to the
-- diary, and searching for it is not an option: the developer key is on the US
-- dataset, where Russian queries return nothing and the localization feature is
-- silently ignored. The account's own eating history solves it instead - it
-- carries real food_ids under the Russian names the app shows, brands included.
--
-- Scoped per user because it is personal history, not a shared catalogue.
CREATE TABLE IF NOT EXISTS fatsecret_foods (
    user_id      uuid NOT NULL REFERENCES users(id),
    food_id      varchar(64) NOT NULL,
    food_name    varchar(255) NOT NULL,
    brand_name   varchar(255),
    food_type    varchar(50),
    food_url     text,
    -- Which history endpoint surfaced it. most_eaten ranks higher than
    -- recently_eaten when the same phrase could mean either of two products.
    source       varchar(20) NOT NULL,
    -- Meals it has been eaten in, which is a usable prior for guessing the meal
    -- when the phrase does not name one.
    meals        text[],
    last_seen_at timestamptz DEFAULT NOW(),
    raw_payload  jsonb,
    created_at   timestamptz DEFAULT NOW(),
    updated_at   timestamptz DEFAULT NOW(),
    PRIMARY KEY (user_id, food_id)
);

CREATE INDEX IF NOT EXISTS idx_fatsecret_foods_name ON fatsecret_foods (user_id, lower(food_name));
