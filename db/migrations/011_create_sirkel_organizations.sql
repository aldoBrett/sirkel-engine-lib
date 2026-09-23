CREATE TABLE
  IF NOT EXISTS sirkel_engine.organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    name TEXT,
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT now (),
    updated_at TIMESTAMPTZ DEFAULT now ()
  )