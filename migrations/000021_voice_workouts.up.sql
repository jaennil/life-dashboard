-- Dictated workouts: a session accumulates spoken phrases until it is finished,
-- and only then is written to Hevy.
--
-- The phone deliberately tracks no session identifier. It just posts text, and
-- the open session for the user is the session - which is what the partial
-- unique index below enforces. That keeps the Shortcut to a single action with
-- no state to lose.
CREATE TABLE IF NOT EXISTS voice_workout_sessions (
    id                uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id           uuid NOT NULL REFERENCES users(id),
    -- open -> finished -> pushed, or failed if Hevy rejected the write.
    status            varchar(20) NOT NULL DEFAULT 'open',
    started_at        timestamptz NOT NULL DEFAULT NOW(),
    last_utterance_at timestamptz NOT NULL DEFAULT NOW(),
    finished_at       timestamptz,
    -- Title is generated from the exercises actually logged, at finish time.
    title             varchar(255),
    -- Spoken duration, used by the one-shot mode where the whole workout is
    -- dictated afterwards and start_time has to be derived from the end.
    duration_seconds  integer,
    -- Merged draft of the whole workout, rebuilt as phrases arrive.
    draft             jsonb,
    hevy_workout_id   varchar(64),
    push_error        text,
    created_at        timestamptz DEFAULT NOW(),
    updated_at        timestamptz DEFAULT NOW()
);

-- At most one open session per user. Two open sessions would silently split a
-- workout in half depending on which one a phrase landed in.
CREATE UNIQUE INDEX IF NOT EXISTS idx_voice_sessions_one_open
    ON voice_workout_sessions (user_id) WHERE status = 'open';
CREATE INDEX IF NOT EXISTS idx_voice_sessions_user_started
    ON voice_workout_sessions (user_id, started_at DESC);

-- Every phrase is kept verbatim alongside what was made of it. Speech
-- recognition and parsing both fail in ways that are only diagnosable from the
-- original wording, so the text is archived before anything interprets it.
CREATE TABLE IF NOT EXISTS voice_workout_utterances (
    id          uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id  uuid NOT NULL REFERENCES voice_workout_sessions(id) ON DELETE CASCADE,
    said_at     timestamptz NOT NULL DEFAULT NOW(),
    text        text NOT NULL,
    is_finish   boolean NOT NULL DEFAULT FALSE,
    parsed      jsonb,
    parse_error text,
    created_at  timestamptz DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_voice_utterances_session
    ON voice_workout_utterances (session_id, said_at);
