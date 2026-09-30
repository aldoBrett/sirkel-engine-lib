-- A user can belong to more than one organization, with a per-organization role. This introduces the
-- association table and backfills it from the existing users.organization_id/role.
--
-- users.organization_id and users.role are NOT removed yet: they become a denormalized snapshot of the
-- user's *current* membership (the one with is_current = true below), kept in sync by the code that
-- writes to user_organizations. This lets every existing query that joins on users.organization_id /
-- users.role (login, users_index, userBelongsToTaskOrganization, userBelongsToGoalOrganization, the
-- creator/assignee lookups in task_items) keep working unchanged. A later migration can drop them once
-- those call sites are moved over to join through user_organizations directly.
CREATE TABLE
  IF NOT EXISTS sirkel_engine.user_organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    user_id UUID NOT NULL REFERENCES sirkel_engine.users (id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES sirkel_engine.organizations (id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    is_current BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT now (),
    updated_at TIMESTAMPTZ DEFAULT now (),
    UNIQUE (user_id, organization_id)
  );

CREATE INDEX IF NOT EXISTS user_organizations_organization_id_idx ON sirkel_engine.user_organizations (organization_id);

-- At most one current organization per user.
CREATE UNIQUE INDEX IF NOT EXISTS user_organizations_one_current_idx ON sirkel_engine.user_organizations (user_id)
WHERE
  is_current;

INSERT INTO
  sirkel_engine.user_organizations (user_id, organization_id, role, is_current)
SELECT
  id,
  organization_id,
  role,
  true
FROM
  sirkel_engine.users
WHERE
  organization_id IS NOT NULL ON CONFLICT (user_id, organization_id)
DO NOTHING;
