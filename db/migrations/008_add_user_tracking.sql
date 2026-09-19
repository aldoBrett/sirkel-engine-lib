ALTER TABLE sirkel_engine.projects
ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES auth.users (id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS updated_by UUID REFERENCES auth.users (id) ON DELETE SET NULL;

ALTER TABLE sirkel_engine.goals
ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES auth.users (id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS updated_by UUID REFERENCES auth.users (id) ON DELETE SET NULL;

ALTER TABLE sirkel_engine.tasks
ADD COLUMN IF NOT EXISTS responsible_user_id UUID REFERENCES auth.users (id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES auth.users (id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS updated_by UUID REFERENCES auth.users (id) ON DELETE SET NULL;

ALTER TABLE sirkel_engine.task_items
ADD COLUMN IF NOT EXISTS assigned_user_id UUID REFERENCES auth.users (id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES auth.users (id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS updated_by UUID REFERENCES auth.users (id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS tasks_responsible_user_id_idx ON sirkel_engine.tasks (responsible_user_id);

CREATE INDEX IF NOT EXISTS task_items_assigned_user_id_idx ON sirkel_engine.task_items (assigned_user_id);
