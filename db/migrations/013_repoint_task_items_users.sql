-- task_items.assigned_user_id/created_by/updated_by were originally tied to auth.users (008_add_user_tracking.sql),
-- but sirkel_auth now manages users in sirkel_engine.users (012_create_sirkel_users.sql), a separate table with its
-- own ids. Null out any references that don't exist there, then repoint the foreign keys.
UPDATE sirkel_engine.task_items
SET assigned_user_id = NULL
WHERE assigned_user_id IS NOT NULL
  AND assigned_user_id NOT IN (SELECT id FROM sirkel_engine.users);

UPDATE sirkel_engine.task_items
SET created_by = NULL
WHERE created_by IS NOT NULL
  AND created_by NOT IN (SELECT id FROM sirkel_engine.users);

UPDATE sirkel_engine.task_items
SET updated_by = NULL
WHERE updated_by IS NOT NULL
  AND updated_by NOT IN (SELECT id FROM sirkel_engine.users);

ALTER TABLE sirkel_engine.task_items
DROP CONSTRAINT IF EXISTS task_items_assigned_user_id_fkey,
DROP CONSTRAINT IF EXISTS task_items_created_by_fkey,
DROP CONSTRAINT IF EXISTS task_items_updated_by_fkey,
ADD CONSTRAINT task_items_assigned_user_id_fkey FOREIGN KEY (assigned_user_id) REFERENCES sirkel_engine.users (id) ON DELETE SET NULL,
ADD CONSTRAINT task_items_created_by_fkey FOREIGN KEY (created_by) REFERENCES sirkel_engine.users (id) ON DELETE SET NULL,
ADD CONSTRAINT task_items_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES sirkel_engine.users (id) ON DELETE SET NULL;
