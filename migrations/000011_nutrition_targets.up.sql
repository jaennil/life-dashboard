CREATE TABLE nutrition_targets (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source VARCHAR(20) NOT NULL DEFAULT 'fatsecret',
    current_weight_kg NUMERIC(6,2),
    current_weight_date DATE,
    current_weight_comment TEXT,
    target_weight_kg NUMERIC(6,2),
    height_cm NUMERIC(6,2),
    target_calories NUMERIC(8,2),
    target_protein_g NUMERIC(6,2),
    target_carbs_g NUMERIC(6,2),
    target_fat_g NUMERIC(6,2),
    weight_measure VARCHAR(10),
    height_measure VARCHAR(10),
    raw_payload JSONB,
    synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, source)
);

CREATE INDEX idx_nutrition_targets_source ON nutrition_targets(source);
