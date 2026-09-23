-- projects/goals/tasks/task_events track their actor (created_by, updated_by, responsible_user_id, changed_by)
-- against auth.users (008_add_user_tracking.sql, 009_create_task_events.sql), but sirkel_auth issues sessions
-- against sirkel_engine.users (012_create_sirkel_users.sql). This is the same drift already fixed for task_items
-- in 013_repoint_task_items_users.sql; repoint the rest the same way.
UPDATE sirkel_engine.projects SET created_by = NULL
WHERE created_by IS NOT NULL AND created_by NOT IN (SELECT id FROM sirkel_engine.users);
UPDATE sirkel_engine.projects SET updated_by = NULL
WHERE updated_by IS NOT NULL AND updated_by NOT IN (SELECT id FROM sirkel_engine.users);

UPDATE sirkel_engine.goals SET created_by = NULL
WHERE created_by IS NOT NULL AND created_by NOT IN (SELECT id FROM sirkel_engine.users);
UPDATE sirkel_engine.goals SET updated_by = NULL
WHERE updated_by IS NOT NULL AND updated_by NOT IN (SELECT id FROM sirkel_engine.users);

UPDATE sirkel_engine.tasks SET responsible_user_id = NULL
WHERE responsible_user_id IS NOT NULL AND responsible_user_id NOT IN (SELECT id FROM sirkel_engine.users);
UPDATE sirkel_engine.tasks SET created_by = NULL
WHERE created_by IS NOT NULL AND created_by NOT IN (SELECT id FROM sirkel_engine.users);
UPDATE sirkel_engine.tasks SET updated_by = NULL
WHERE updated_by IS NOT NULL AND updated_by NOT IN (SELECT id FROM sirkel_engine.users);

UPDATE sirkel_engine.task_events SET changed_by = NULL
WHERE changed_by IS NOT NULL AND changed_by NOT IN (SELECT id FROM sirkel_engine.users);

ALTER TABLE sirkel_engine.projects
DROP CONSTRAINT IF EXISTS projects_created_by_fkey,
DROP CONSTRAINT IF EXISTS projects_updated_by_fkey,
ADD CONSTRAINT projects_created_by_fkey FOREIGN KEY (created_by) REFERENCES sirkel_engine.users (id) ON DELETE SET NULL,
ADD CONSTRAINT projects_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES sirkel_engine.users (id) ON DELETE SET NULL;

ALTER TABLE sirkel_engine.goals
DROP CONSTRAINT IF EXISTS goals_created_by_fkey,
DROP CONSTRAINT IF EXISTS goals_updated_by_fkey,
ADD CONSTRAINT goals_created_by_fkey FOREIGN KEY (created_by) REFERENCES sirkel_engine.users (id) ON DELETE SET NULL,
ADD CONSTRAINT goals_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES sirkel_engine.users (id) ON DELETE SET NULL;

ALTER TABLE sirkel_engine.tasks
DROP CONSTRAINT IF EXISTS tasks_responsible_user_id_fkey,
DROP CONSTRAINT IF EXISTS tasks_created_by_fkey,
DROP CONSTRAINT IF EXISTS tasks_updated_by_fkey,
ADD CONSTRAINT tasks_responsible_user_id_fkey FOREIGN KEY (responsible_user_id) REFERENCES sirkel_engine.users (id) ON DELETE SET NULL,
ADD CONSTRAINT tasks_created_by_fkey FOREIGN KEY (created_by) REFERENCES sirkel_engine.users (id) ON DELETE SET NULL,
ADD CONSTRAINT tasks_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES sirkel_engine.users (id) ON DELETE SET NULL;

ALTER TABLE sirkel_engine.task_events
DROP CONSTRAINT IF EXISTS task_events_changed_by_fkey,
ADD CONSTRAINT task_events_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES sirkel_engine.users (id) ON DELETE SET NULL;
