-- Only Todoist rows fit the single-source tables this restores.
DELETE FROM task_completions WHERE source <> 'todoist';
DELETE FROM tasks WHERE source <> 'todoist';

ALTER TABLE task_completions DROP CONSTRAINT task_completions_user_source_task_completed_key;
ALTER TABLE tasks DROP CONSTRAINT tasks_user_source_external_key;

ALTER TABLE task_completions DROP COLUMN source;
ALTER TABLE tasks DROP COLUMN source;

ALTER TABLE task_completions
    RENAME CONSTRAINT task_completions_user_id_fkey TO todoist_task_completions_user_id_fkey;
ALTER TABLE tasks RENAME CONSTRAINT tasks_user_id_fkey TO todoist_tasks_user_id_fkey;
ALTER INDEX task_completions_pkey RENAME TO todoist_task_completions_pkey;
ALTER INDEX tasks_pkey RENAME TO todoist_tasks_pkey;

ALTER INDEX idx_task_completions_user_task
    RENAME TO idx_todoist_task_completions_user_task;
ALTER INDEX idx_task_completions_user_completed_at
    RENAME TO idx_todoist_task_completions_user_completed_at;
ALTER INDEX idx_tasks_user_added_at RENAME TO idx_todoist_tasks_user_added_at;
ALTER INDEX idx_tasks_user_due_at RENAME TO idx_todoist_tasks_user_due_at;
ALTER INDEX idx_tasks_user_due_date RENAME TO idx_todoist_tasks_user_due_date;
ALTER INDEX idx_tasks_user_active RENAME TO idx_todoist_tasks_user_active;

ALTER TABLE tasks
    ADD CONSTRAINT todoist_tasks_user_id_external_id_key UNIQUE (user_id, external_id);
ALTER TABLE task_completions
    ADD CONSTRAINT todoist_task_completions_user_id_task_external_id_completed_at_key
    UNIQUE (user_id, task_external_id, completed_at);

ALTER TABLE task_completions RENAME TO todoist_task_completions;
ALTER TABLE tasks RENAME TO todoist_tasks;
