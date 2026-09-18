CREATE TABLE
  IF NOT EXISTS sirkel_engine.task_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    task_id UUID REFERENCES sirkel_engine.tasks (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    state TEXT NOT NULL DEFAULT 'pending' CHECK (
      state IN ('pending', 'in-progress', 'done', 'cancelled')
    ),
    created_at TIMESTAMPTZ DEFAULT now (),
    updated_at TIMESTAMPTZ DEFAULT now ()
  )