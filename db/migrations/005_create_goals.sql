CREATE TABLE
  IF NOT EXISTS sirkel_engine.goals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    project_id UUID REFERENCES sirkel_engine.projects (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    state TEXT NOT NULL DEFAULT 'active' CHECK (
      state IN ('active', 'achieved', 'on-hold', 'abandoned')
    ),
    created_at TIMESTAMPTZ DEFAULT now (),
    updated_at TIMESTAMPTZ DEFAULT now ()
  )