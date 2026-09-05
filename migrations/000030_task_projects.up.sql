-- Mirrors the provider's project list locally.
--
-- The dictated-task parser has to name real projects in its prompt and resolve
-- what the model answers back to an id, and doing that over the provider API on
-- every phrase would put a network round trip in front of every dictated word.
-- The sync already reads the full list, so it keeps this table current.
CREATE TABLE task_projects (
    id                 uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id            uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source             varchar(50) NOT NULL,
    external_id        varchar(255) NOT NULL,
    name               varchar(255) NOT NULL,
    -- Full path including parent projects, the way the app shows it.
    path               varchar(1000) NOT NULL,
    parent_external_id varchar(255),
    archived           boolean NOT NULL DEFAULT FALSE,
    -- Where the provider puts a task created without naming a project.
    is_default         boolean NOT NULL DEFAULT FALSE,
    created_at         timestamptz NOT NULL DEFAULT NOW(),
    updated_at         timestamptz NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, source, external_id)
);

CREATE INDEX idx_task_projects_user_source ON task_projects (user_id, source, archived);
