CREATE TABLE
  IF NOT EXISTS sirkel_engine.projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    organization_id UUID REFERENCES auth.organizations (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'planning' CHECK (
      state IN ('planning', 'active', 'on-hold', 'completed', 'archived')
    ),
    created_at TIMESTAMPTZ DEFAULT now (),
    updated_at TIMESTAMPTZ DEFAULT now ()
  )