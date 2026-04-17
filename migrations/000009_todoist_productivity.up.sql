CREATE TABLE todoist_tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    external_id VARCHAR(255) NOT NULL,
    parent_external_id VARCHAR(255),
    project_external_id VARCHAR(255),
    project_name VARCHAR(255),
    section_external_id VARCHAR(255),
    section_name VARCHAR(255),
    content TEXT NOT NULL,
    description TEXT,
    labels TEXT[] NOT NULL DEFAULT '{}',
    priority INTEGER,
    is_recurring BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    added_at TIMESTAMPTZ,
    due_at TIMESTAMPTZ,
    due_date DATE,
    due_string VARCHAR(255),
    due_timezone VARCHAR(100),
    last_completed_at TIMESTAMPTZ,
    raw_payload JSONB,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, external_id)
);

CREATE INDEX idx_todoist_tasks_user_active ON todoist_tasks(user_id, is_active);
CREATE INDEX idx_todoist_tasks_user_due_date ON todoist_tasks(user_id, due_date);
CREATE INDEX idx_todoist_tasks_user_due_at ON todoist_tasks(user_id, due_at);
CREATE INDEX idx_todoist_tasks_user_added_at ON todoist_tasks(user_id, added_at);

CREATE TABLE todoist_task_completions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    task_external_id VARCHAR(255) NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL,
    content TEXT,
    project_external_id VARCHAR(255),
    project_name VARCHAR(255),
    section_external_id VARCHAR(255),
    section_name VARCHAR(255),
    is_recurring BOOLEAN NOT NULL DEFAULT FALSE,
    raw_payload JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, task_external_id, completed_at)
);

CREATE INDEX idx_todoist_task_completions_user_completed_at ON todoist_task_completions(user_id, completed_at DESC);
CREATE INDEX idx_todoist_task_completions_user_task ON todoist_task_completions(user_id, task_external_id);
