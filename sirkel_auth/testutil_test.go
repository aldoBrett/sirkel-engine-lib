package sirkel_auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"sirkel-engine-lib/sirkel_errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testDatabaseLockID is the advisory lock that serializes tests across
// packages sharing the test database (see sirkel_repository, sirkel_analytics).
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
			sirkel_engine.users,
			sirkel_engine.organizations
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("unable to truncate tables: %v", err)
	}
}

// insertTestOrganization inserts an organization directly, bypassing
// CreateOrganization, and returns its id.
func insertTestOrganization(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()

	var organizationID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO sirkel_engine.organizations (id, name, description)
		VALUES ($1, $2, $3)
		RETURNING id
	`, testUUID(t), name, "org for tests").Scan(&organizationID)
	if err != nil {
		t.Fatalf("unable to create test organization: %v", err)
	}

	return organizationID
}

// insertTestUser inserts a user directly, bypassing CreateUser, and returns
// its id. passwordHash is stored as-is, so callers wanting to exercise Login
// should pass a real bcrypt hash.
func insertTestUser(t *testing.T, pool *pgxpool.Pool, organizationID, email, passwordHash, role string) string {
	t.Helper()

	var userID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO sirkel_engine.users (id, organization_id, role, name, first_surname, second_surname, email, password_hash)
		VALUES ($1, $2, $3, 'Test', 'Surname', 'SecondSurname', $4, $5)
		RETURNING id
	`, testUUID(t), organizationID, role, email, passwordHash).Scan(&userID)
	if err != nil {
		t.Fatalf("unable to create test user: %v", err)
	}

	return userID
}

// assertErrorCode fails the test unless err is a *sirkel_errors.Error with
// the given code.
func assertErrorCode(t *testing.T, err error, code sirkel_errors.Code) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error with code %q, got nil", code)
	}
	sirkelErr, ok := err.(*sirkel_errors.Error)
	if !ok {
		t.Fatalf("expected *sirkel_errors.Error with code %q, got %T: %v", code, err, err)
	}
	if sirkelErr.Code != code {
		t.Fatalf("expected error code %q, got %q", code, sirkelErr.Code)
	}
}
