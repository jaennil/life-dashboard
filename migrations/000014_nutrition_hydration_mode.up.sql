ALTER TABLE nutrition_targets
    ADD COLUMN hydration_mode VARCHAR(20);

CREATE TABLE nutrition_hydration_entries (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date DATE NOT NULL,
    beverage_type VARCHAR(20) NOT NULL,
    amount_ml NUMERIC(8,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, date, beverage_type),
    CHECK (beverage_type IN ('tea', 'coffee', 'energy', 'milkshake', 'other')),
    CHECK (amount_ml >= 0)
);

CREATE INDEX idx_nutrition_hydration_entries_user_date
    ON nutrition_hydration_entries(user_id, date DESC);
