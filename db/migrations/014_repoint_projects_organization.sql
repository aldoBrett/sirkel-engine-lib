-- sirkel_engine.projects.organization_id was created against auth.organizations (004_create_projects.sql), but
-- sirkel_auth now manages organizations in sirkel_engine.organizations (011_create_sirkel_organizations.sql), a
-- separate table with its own ids. Null out any references that don't exist there, then repoint the foreign key.
-- This also fixes userBelongsToTaskOrganization/userBelongsToGoalOrganization, which compare a
-- sirkel_engine.users.organization_id against sirkel_engine.projects.organization_id and need both to be ids of
-- the same sirkel_engine.organizations table.
UPDATE sirkel_engine.projects
SET organization_id = NULL
WHERE organization_id IS NOT NULL
  AND organization_id NOT IN (SELECT id FROM sirkel_engine.organizations);

ALTER TABLE sirkel_engine.projects
DROP CONSTRAINT IF EXISTS projects_organization_id_fkey,
ADD CONSTRAINT projects_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES sirkel_engine.organizations (id) ON DELETE CASCADE;
