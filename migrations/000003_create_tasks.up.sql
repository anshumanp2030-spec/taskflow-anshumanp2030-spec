CREATE TYPE task_status   AS ENUM ('todo', 'in_progress', 'done');
CREATE TYPE task_priority AS ENUM ('low', 'medium', 'high');

CREATE TABLE IF NOT EXISTS tasks (
    id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    title       VARCHAR(255)  NOT NULL,
    description TEXT,
    status      task_status   NOT NULL DEFAULT 'todo',
    priority    task_priority NOT NULL DEFAULT 'medium',
    project_id  UUID          NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    assignee_id UUID          REFERENCES users(id) ON DELETE SET NULL,
    due_date    DATE,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tasks_project_id   ON tasks(project_id);
CREATE INDEX IF NOT EXISTS idx_tasks_assignee_id  ON tasks(assignee_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status       ON tasks(status);
