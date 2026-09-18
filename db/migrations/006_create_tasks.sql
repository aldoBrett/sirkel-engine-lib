CREATE TABLE
  IF NOT EXISTS sirkel_engine.tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    goal_id UUID REFERENCES sirkel_engine.goals (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    state TEXT NOT NULL DEFAULT 'todo' CHECK (
      state IN ('todo', 'in-progress', 'blocked', 'done', 'cancelled', 'in-review')
    ),
    created_at TIMESTAMPTZ DEFAULT now (),
    updated_at TIMESTAMPTZ DEFAULT now ()
  )