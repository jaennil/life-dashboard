-- Hevy exercise templates, needed to write workouts back to Hevy.
--
-- Creating a workout requires an exercise_template_id, so dictated exercise
-- names have to be resolved against this catalogue. It is stored locally rather
-- than fetched per request because matching wants the whole list at once and the
-- catalogue changes only when a custom exercise is added.
--
-- Built-in templates are shared by every Hevy account, so they are keyed by the
-- template id alone and owner_user_id stays NULL. Custom templates are visible
-- only to the account that created them, so they record their owner and matching
-- filters on it - otherwise one user's custom exercises would leak into another
-- user's candidates.
CREATE TABLE IF NOT EXISTS hevy_exercise_templates (
    id                      varchar(64) PRIMARY KEY,
    owner_user_id           uuid REFERENCES users(id),
    title                   varchar(255) NOT NULL,
    -- weight_reps, reps_only, duration, distance_duration and so on. It decides
    -- which set fields are legal: sending weight_kg for a reps_only exercise
    -- puts nonsense in the training history.
    type                    varchar(50) NOT NULL,
    primary_muscle_group    varchar(100),
    secondary_muscle_groups text[],
    equipment               varchar(100),
    is_custom               boolean NOT NULL DEFAULT FALSE,
    raw_payload             jsonb,
    created_at              timestamptz DEFAULT NOW(),
    updated_at              timestamptz DEFAULT NOW()
);

-- Titles are English, so lower() folds them correctly even under LC_COLLATE = C,
-- which does not case-fold Cyrillic.
CREATE INDEX IF NOT EXISTS idx_hevy_templates_title ON hevy_exercise_templates (lower(title));
CREATE INDEX IF NOT EXISTS idx_hevy_templates_owner ON hevy_exercise_templates (owner_user_id)
    WHERE owner_user_id IS NOT NULL;
