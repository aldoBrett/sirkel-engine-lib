CREATE TABLE
  IF NOT EXISTS sirkel_engine.users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    organization_id UUID REFERENCES sirkel_engine.organizations (id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    name TEXT NOT NULL,
    first_surname TEXT NOT NULL,
    second_surname TEXT NOT NULL,
    phone TEXT,
    email TEXT NOT NULL UNIQUE,
    email_verified_at TIMESTAMPTZ,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now (),
    updated_at TIMESTAMPTZ DEFAULT now ()
  )