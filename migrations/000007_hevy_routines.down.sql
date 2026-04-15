DROP TABLE IF EXISTS routine_sets;
DROP TABLE IF EXISTS routine_exercises;
DROP TABLE IF EXISTS workout_routines;
DROP INDEX IF EXISTS idx_workouts_user_routine_external_id;

ALTER TABLE workouts
DROP COLUMN IF EXISTS routine_external_id;
