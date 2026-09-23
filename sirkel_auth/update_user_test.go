package sirkel_auth

import (
	"context"
	"testing"

	"sirkel-engine-lib/sirkel_errors"

	"golang.org/x/crypto/bcrypt"
)

func TestSirkelAuthHandler_UpdateUser_UpdatesFields(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	userID := insertTestUser(t, pool, organizationID, "user@example.com", "hash", "user")

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	newEmail := "updated@example.com"
	newName := "Updated Name"
	newRole := "admin"
	err := h.UpdateUser(UpdateUserParams{
		UserID: userID,
		Email:  &newEmail,
		Name:   &newName,
		Role:   &newRole,
	})
	if err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}

	var email, name, role, firstSurname string
	row := pool.QueryRow(context.Background(), `SELECT email, name, role, first_surname FROM sirkel_engine.users WHERE id = $1`, userID)
	if err := row.Scan(&email, &name, &role, &firstSurname); err != nil {
		t.Fatalf("unable to read user: %v", err)
	}
	if email != newEmail {
		t.Fatalf("expected email %q, got %q", newEmail, email)
	}
	if name != newName {
		t.Fatalf("expected name %q, got %q", newName, name)
	}
	if role != newRole {
		t.Fatalf("expected role %q, got %q", newRole, role)
	}
	if firstSurname != "Surname" {
		t.Fatalf("expected first_surname to remain unchanged, got %q", firstSurname)
	}
}

func TestSirkelAuthHandler_UpdateUser_RehashesPassword(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	userID := insertTestUser(t, pool, organizationID, "user@example.com", "hash", "user")

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	newPassword := "n3wpassword!"
	if err := h.UpdateUser(UpdateUserParams{UserID: userID, Password: &newPassword}); err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}

	var passwordHash string
	row := pool.QueryRow(context.Background(), `SELECT password_hash FROM sirkel_engine.users WHERE id = $1`, userID)
	if err := row.Scan(&passwordHash); err != nil {
		t.Fatalf("unable to read user: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(newPassword)); err != nil {
		t.Fatalf("expected password hash to match, got error: %v", err)
	}
}

func TestSirkelAuthHandler_UpdateUser_RequiresUserID(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})

	err := h.UpdateUser(UpdateUserParams{})
	assertErrorCode(t, err, sirkel_errors.CodeUserIDRequired)
}

func TestSirkelAuthHandler_UpdateUser_NoFieldsToUpdate(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	userID := insertTestUser(t, pool, organizationID, "user@example.com", "hash", "user")

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	err := h.UpdateUser(UpdateUserParams{UserID: userID})
	assertErrorCode(t, err, sirkel_errors.CodeNoFieldsToUpdate)
}

func TestSirkelAuthHandler_UpdateUser_NotFound(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})

	missingID := "00000000-0000-0000-0000-000000000000"
	newName := "Nobody"
	err := h.UpdateUser(UpdateUserParams{UserID: missingID, Name: &newName})
	assertErrorCode(t, err, sirkel_errors.CodeUserNotFound)
}
