CREATE TABLE
  IF NOT EXISTS sirkel_engine.task_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES sirkel_engine.tasks (id) ON DELETE CASCADE,
    task_item_id UUID REFERENCES sirkel_engine.task_items (id) ON DELETE CASCADE,
    field TEXT NOT NULL CHECK (
      field IN (
        'name',
        'description',
        'state',
        'responsible_user_id',
        'assigned_user_id'
      )
    ),
    old_value TEXT,
    new_value TEXT,
    changed_by UUID REFERENCES auth.users (id) ON DELETE SET NULL,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT now ()
  );

CREATE INDEX IF NOT EXISTS task_events_task_id_changed_at_idx ON sirkel_engine.task_events (task_id, changed_at);

CREATE INDEX IF NOT EXISTS task_events_field_changed_at_idx ON sirkel_engine.task_events (field, changed_at);
