package sirkel_analytics

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"sirkel-engine-lib/sirkel_domain"
	"sirkel-engine-lib/sirkel_repository"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testDatabaseLockID is the advisory lock that serializes tests across packages.
const testDatabaseLockID = 7264120

func testUUID(t *testing.T) string {
	t.Helper()

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("unable to generate uuid: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

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

	// Other packages share this database, so tests take turns using it.
	t.Cleanup(pool.Close)
	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("unable to acquire connection: %v", err)
	}
	t.Cleanup(lockConn.Release)
	if _, err := lockConn.Exec(ctx, `SELECT pg_advisory_lock($1)`, testDatabaseLockID); err != nil {
		t.Fatalf("unable to lock test database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := lockConn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, testDatabaseLockID); err != nil {
			t.Errorf("unable to unlock test database: %v", err)
		}
	})

	applyMigrations(t, pool)
	t.Cleanup(func() {
		truncateAll(t, pool)
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
			sirkel_engine.task_events,
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
		VALUES ('Test Org', 'org for tests')
		RETURNING id
	`).Scan(&organizationID)
	if err != nil {
		t.Fatalf("unable to create test organization: %v", err)
	}

	return organizationID
}

func createTestUser(t *testing.T, pool *pgxpool.Pool, organizationID, email string) *sirkel_domain.User {
	t.Helper()

	user := &sirkel_domain.User{
		ID:             testUUID(t),
		OrganizationID: organizationID,
		Email:          email,
		Role:           "user",
	}
	_, err := pool.Exec(context.Background(), `
		INSERT INTO auth.users (id, organization_id, email, password_hash, role)
		VALUES ($1, $2, $3, 'test-hash', $4)
	`, user.ID, user.OrganizationID, user.Email, user.Role)
	if err != nil {
		t.Fatalf("unable to create test user: %v", err)
	}

	return user
}

// tenant is an organization with a user acting in it, and the repositories used to fill it with data.
type tenant struct {
	t              *testing.T
	pool           *pgxpool.Pool
	organizationID string
	user           *sirkel_domain.User
	repositories   *sirkel_repository.Repositories
	analytics      *Analytics
}

func newTenant(t *testing.T, pool *pgxpool.Pool, email string) *tenant {
	t.Helper()

	organizationID := createTestOrganization(t, pool)
	user := createTestUser(t, pool, organizationID, email)

	return &tenant{
		t:              t,
		pool:           pool,
		organizationID: organizationID,
		user:           user,
		repositories:   sirkel_repository.NewRepositories(context.Background(), pool, user),
		analytics:      NewAnalytics(context.Background(), pool, user),
	}
}

func (tn *tenant) project(name string) *sirkel_domain.Project {
	tn.t.Helper()

	project := &sirkel_domain.Project{
		ID:          testUUID(tn.t),
		Name:        name,
		Description: "test description",
		State:       sirkel_domain.ProjectStateActive,
	}
	if err := tn.repositories.Projects.SaveProject(project); err != nil {
		tn.t.Fatalf("SaveProject() error = %v", err)
	}

	return project
}

func (tn *tenant) goal(projectID, name string) *sirkel_domain.Goal {
	tn.t.Helper()

	goal := &sirkel_domain.Goal{
		ID:        testUUID(tn.t),
		ProjectID: projectID,
		Name:      name,
		State:     sirkel_domain.GoalStateActive,
	}
	if err := tn.repositories.Goals.SaveGoal(goal); err != nil {
		tn.t.Fatalf("SaveGoal() error = %v", err)
	}

	return goal
}

func (tn *tenant) task(goalID, name string, state sirkel_domain.TaskState) *sirkel_domain.Task {
	tn.t.Helper()

	task := &sirkel_domain.Task{
		ID:     testUUID(tn.t),
		GoalID: goalID,
		Name:   name,
		State:  state,
	}
	if err := tn.repositories.Tasks.SaveTask(task); err != nil {
		tn.t.Fatalf("SaveTask() error = %v", err)
	}

	return task
}

func (tn *tenant) setTaskState(task *sirkel_domain.Task, state sirkel_domain.TaskState) {
	tn.t.Helper()

	task.State = state
	if err := tn.repositories.Tasks.SaveTask(task); err != nil {
		tn.t.Fatalf("SaveTask() error = %v", err)
	}
}

func (tn *tenant) taskItem(taskID, name string, state sirkel_domain.TaskItemState, assignedUserID *string) *sirkel_domain.TaskItem {
	tn.t.Helper()

	taskItem := &sirkel_domain.TaskItem{
		ID:             testUUID(tn.t),
		TaskID:         taskID,
		Name:           name,
		State:          state,
		AssignedUserID: assignedUserID,
	}
	if err := tn.repositories.TaskItems.SaveTaskItem(taskItem); err != nil {
		tn.t.Fatalf("SaveTaskItem() error = %v", err)
	}

	return taskItem
}

func (tn *tenant) exec(query string, args ...any) {
	tn.t.Helper()

	if _, err := tn.pool.Exec(context.Background(), query, args...); err != nil {
		tn.t.Fatalf("unable to run %q: %v", query, err)
	}
}

// backdateTask moves a task's own timestamp, and those of its items, to the past.
func (tn *tenant) backdateTask(taskID string, age time.Duration) {
	tn.t.Helper()

	at := time.Now().Add(-age)
	tn.exec(`UPDATE sirkel_engine.tasks SET updated_at = $2 WHERE id = $1`, taskID, at)
	tn.exec(`UPDATE sirkel_engine.task_items SET updated_at = $2 WHERE task_id = $1`, taskID, at)
}

// setTaskHistory rewrites when a task was created and when it moved to each state.
func (tn *tenant) setTaskHistory(taskID string, createdAt time.Time, stateChanges map[sirkel_domain.TaskState]time.Time) {
	tn.t.Helper()

	tn.exec(`UPDATE sirkel_engine.tasks SET created_at = $2 WHERE id = $1`, taskID, createdAt)
	for state, at := range stateChanges {
		tn.exec(`
			UPDATE sirkel_engine.task_events SET changed_at = $3
			WHERE task_id = $1 AND task_item_id IS NULL AND field = 'state' AND new_value = $2
		`, taskID, string(state), at)
	}
}

func requireError(t *testing.T, err, want error) {
	t.Helper()

	if !errors.Is(err, want) {
		t.Fatalf("expected error %v, got %v", want, err)
	}
}

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 12, 0, 0, 0, time.UTC)
}
