package sirkel_repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://admin:secretpassword@localhost:5432/sirkel_test_db"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skipping: unable to create pool: %v", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		t.Skipf("skipping: test database not reachable: %v", err)
	}

	applyMigrations(t, pool)
	t.Cleanup(func() {
		truncateAll(t, pool)
		pool.Close()
	})

	return pool
}

func applyMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	files, err := filepath.Glob("../db/migrations/*.sql")
	if err != nil {
		t.Fatalf("unable to glob migrations: %v", err)
	}
	sort.Strings(files)

	for _, file := range files {
		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("unable to read migration %s: %v", file, err)
		}
		if _, err := pool.Exec(context.Background(), string(sqlBytes)); err != nil {
			t.Fatalf("unable to apply migration %s: %v", file, err)
		}
	}
}

func truncateAll(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	_, err := pool.Exec(context.Background(), `
		TRUNCATE TABLE
			sirkel_engine.task_items,
			sirkel_engine.tasks,
			sirkel_engine.goals,
			sirkel_engine.projects,
			auth.users,
			auth.organizations
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("unable to truncate tables: %v", err)
	}
}

func createTestOrganization(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	var organizationID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO auth.organizations (name, description)
		VALUES ($1, $2)
		RETURNING id
	`, "Test Org", "org for tests").Scan(&organizationID)
	if err != nil {
		t.Fatalf("unable to create test organization: %v", err)
	}

	return organizationID
}

func mustSaveProject(t *testing.T, handler *ProjectsRepositoryHandler, organizationID, name string, state sirkel_domain.ProjectState) *sirkel_domain.Project {
	t.Helper()

	project := &sirkel_domain.Project{
		OrganizationID: organizationID,
		Name:           name,
		Description:    "test description",
		State:          state,
	}
	if err := handler.SaveProject(project); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}

	return project
}

func TestProjectsRepositoryHandler_SaveProject_Insert(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	handler := NewProjectsRepositoryHandler(context.Background(), pool, nil)

	project := &sirkel_domain.Project{
		OrganizationID: organizationID,
		Name:           "Launch",
		Description:    "Ship it",
		State:          sirkel_domain.ProjectStatePlanning,
	}

	if err := handler.SaveProject(project); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}

	if project.ID == "" {
		t.Fatal("expected project ID to be set after insert")
	}
	if project.CreatedAt.IsZero() {
		t.Fatal("expected created_at to be set after insert")
	}
	if project.UpdatedAt.IsZero() {
		t.Fatal("expected updated_at to be set after insert")
	}
}

func TestProjectsRepositoryHandler_SaveProject_Update(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	handler := NewProjectsRepositoryHandler(context.Background(), pool, nil)

	project := mustSaveProject(t, handler, organizationID, "Launch", sirkel_domain.ProjectStatePlanning)

	project.Name = "Launch v2"
	project.State = sirkel_domain.ProjectStateActive

	if err := handler.SaveProject(project); err != nil {
		t.Fatalf("SaveProject() update error = %v", err)
	}

	fetched, err := handler.GetProjectByID(&project.ID)
	if err != nil {
		t.Fatalf("GetProjectByID() error = %v", err)
	}
	if fetched == nil {
		t.Fatal("expected project to exist after update")
	}
	if fetched.Name != "Launch v2" {
		t.Fatalf("expected name %q, got %q", "Launch v2", fetched.Name)
	}
	if fetched.State != sirkel_domain.ProjectStateActive {
		t.Fatalf("expected state %q, got %q", sirkel_domain.ProjectStateActive, fetched.State)
	}
}

func TestProjectsRepositoryHandler_GetProjectByID_NotFound(t *testing.T) {
	pool := testPool(t)
	handler := NewProjectsRepositoryHandler(context.Background(), pool, nil)

	missingID := "00000000-0000-0000-0000-000000000000"
	project, err := handler.GetProjectByID(&missingID)
	if err != nil {
		t.Fatalf("GetProjectByID() error = %v", err)
	}
	if project != nil {
		t.Fatalf("expected nil project, got %+v", project)
	}
}

func TestProjectsRepositoryHandler_GetProjects_FiltersByOrganization(t *testing.T) {
	pool := testPool(t)
	handler := NewProjectsRepositoryHandler(context.Background(), pool, nil)

	orgA := createTestOrganization(t, pool)
	orgB := createTestOrganization(t, pool)

	mustSaveProject(t, handler, orgA, "A1", sirkel_domain.ProjectStatePlanning)
	mustSaveProject(t, handler, orgA, "A2", sirkel_domain.ProjectStatePlanning)
	mustSaveProject(t, handler, orgB, "B1", sirkel_domain.ProjectStatePlanning)

	projects, err := handler.GetProjects(&GetProjectsParams{OrganizationID: &orgA})
	if err != nil {
		t.Fatalf("GetProjects() error = %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects for org A, got %d", len(projects))
	}
	for _, p := range projects {
		if p.OrganizationID != orgA {
			t.Fatalf("expected project to belong to org %q, got %q", orgA, p.OrganizationID)
		}
	}
}

func TestProjectsRepositoryHandler_GetProjects_Pagination(t *testing.T) {
	pool := testPool(t)
	handler := NewProjectsRepositoryHandler(context.Background(), pool, nil)

	organizationID := createTestOrganization(t, pool)
	for i := range 3 {
		mustSaveProject(t, handler, organizationID, fmt.Sprintf("Project %d", i), sirkel_domain.ProjectStatePlanning)
	}

	limit := 2
	offset := 1
	projects, err := handler.GetProjects(&GetProjectsParams{
		OrganizationID: &organizationID,
		Limit:          &limit,
		Offset:         &offset,
	})
	if err != nil {
		t.Fatalf("GetProjects() error = %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects with limit/offset, got %d", len(projects))
	}
}

func TestProjectsRepositoryHandler_CountProjects(t *testing.T) {
	pool := testPool(t)
	handler := NewProjectsRepositoryHandler(context.Background(), pool, nil)

	orgA := createTestOrganization(t, pool)
	orgB := createTestOrganization(t, pool)

	mustSaveProject(t, handler, orgA, "A1", sirkel_domain.ProjectStatePlanning)
	mustSaveProject(t, handler, orgA, "A2", sirkel_domain.ProjectStatePlanning)
	mustSaveProject(t, handler, orgB, "B1", sirkel_domain.ProjectStatePlanning)

	count, err := handler.CountProjects(&CountProjectParams{OrganizationID: &orgA})
	if err != nil {
		t.Fatalf("CountProjects() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}

	total, err := handler.CountProjects(&CountProjectParams{})
	if err != nil {
		t.Fatalf("CountProjects() error = %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total count 3, got %d", total)
	}
}

func TestProjectsRepositoryHandler_DeleteProject(t *testing.T) {
	pool := testPool(t)
	handler := NewProjectsRepositoryHandler(context.Background(), pool, nil)

	organizationID := createTestOrganization(t, pool)
	project := mustSaveProject(t, handler, organizationID, "To delete", sirkel_domain.ProjectStatePlanning)

	if err := handler.DeleteProject(&project.ID); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}

	fetched, err := handler.GetProjectByID(&project.ID)
	if err != nil {
		t.Fatalf("GetProjectByID() error = %v", err)
	}
	if fetched != nil {
		t.Fatalf("expected project to be deleted, got %+v", fetched)
	}
}
