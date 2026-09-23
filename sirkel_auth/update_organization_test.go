package sirkel_auth

import (
	"context"
	"testing"

	"sirkel-engine-lib/sirkel_errors"
)

func TestSirkelAuthHandler_UpdateOrganization_UpdatesFields(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	newName := "Acme Renamed"
	newDescription := "new description"
	err := h.UpdateOrganization(UpdateOrganizationParams{
		ID:          organizationID,
		Name:        &newName,
		Description: &newDescription,
	})
	if err != nil {
		t.Fatalf("UpdateOrganization() error = %v", err)
	}

	var name, description string
	row := pool.QueryRow(context.Background(), `SELECT name, description FROM sirkel_engine.organizations WHERE id = $1`, organizationID)
	if err := row.Scan(&name, &description); err != nil {
		t.Fatalf("unable to read organization: %v", err)
	}
	if name != newName {
		t.Fatalf("expected name %q, got %q", newName, name)
	}
	if description != newDescription {
		t.Fatalf("expected description %q, got %q", newDescription, description)
	}
}

func TestSirkelAuthHandler_UpdateOrganization_PartialUpdateKeepsOtherFields(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	newName := "Acme Renamed"
	if err := h.UpdateOrganization(UpdateOrganizationParams{ID: organizationID, Name: &newName}); err != nil {
		t.Fatalf("UpdateOrganization() error = %v", err)
	}

	var name, description string
	row := pool.QueryRow(context.Background(), `SELECT name, description FROM sirkel_engine.organizations WHERE id = $1`, organizationID)
	if err := row.Scan(&name, &description); err != nil {
		t.Fatalf("unable to read organization: %v", err)
	}
	if name != newName {
		t.Fatalf("expected name %q, got %q", newName, name)
	}
	if description != "org for tests" {
		t.Fatalf("expected description to remain unchanged, got %q", description)
	}
}

func TestSirkelAuthHandler_UpdateOrganization_RequiresID(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})

	err := h.UpdateOrganization(UpdateOrganizationParams{})
	assertErrorCode(t, err, sirkel_errors.CodeOrganizationIDRequired)
}

func TestSirkelAuthHandler_UpdateOrganization_NotFound(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})

	missingID := "00000000-0000-0000-0000-000000000000"
	err := h.UpdateOrganization(UpdateOrganizationParams{ID: missingID})
	assertErrorCode(t, err, sirkel_errors.CodeOrganizationNotFound)
}
