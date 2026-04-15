ALTER TABLE workouts
ADD COLUMN routine_external_id VARCHAR(255);

UPDATE workouts
SET routine_external_id = NULLIF(raw_payload->>'routine_id', '')
WHERE source = 'hevy'
  AND raw_payload IS NOT NULL
  AND raw_payload ? 'routine_id';

CREATE INDEX idx_workouts_user_routine_external_id
ON workouts(user_id, routine_external_id)
WHERE routine_external_id IS NOT NULL;

CREATE TABLE workout_routines (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    source VARCHAR(20) NOT NULL DEFAULT 'hevy',
    external_id VARCHAR(255) NOT NULL,
    title VARCHAR(255) NOT NULL,
    folder_id BIGINT,
    raw_payload JSONB,
    source_created_at TIMESTAMPTZ,
    source_updated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, external_id)
);

CREATE INDEX idx_workout_routines_user_id ON workout_routines(user_id);

CREATE TABLE routine_exercises (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    routine_id UUID NOT NULL REFERENCES workout_routines(id) ON DELETE CASCADE,
    exercise_index INTEGER NOT NULL,
    exercise_name VARCHAR(255) NOT NULL,
    notes TEXT,
    template_id VARCHAR(255),
    superset_id VARCHAR(255),
    rest_seconds INTEGER
);

CREATE INDEX idx_routine_exercises_routine_id ON routine_exercises(routine_id);

CREATE TABLE routine_sets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    routine_exercise_id UUID NOT NULL REFERENCES routine_exercises(id) ON DELETE CASCADE,
    set_index INTEGER NOT NULL,
    set_type VARCHAR(20) DEFAULT 'normal',
    weight_kg NUMERIC(6,2),
    reps INTEGER,
    distance_meters NUMERIC(10,2),
    duration_seconds INTEGER,
    custom_metric JSONB
);

CREATE INDEX idx_routine_sets_exercise_id ON routine_sets(routine_exercise_id);
