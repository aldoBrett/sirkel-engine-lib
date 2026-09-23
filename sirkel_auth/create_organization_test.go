package sirkel_auth

import (
	"context"
	"testing"

	"sirkel-engine-lib/sirkel_errors"
)

func TestSirkelAuthHandler_CreateOrganization_Insert(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})

	description := "a fine org"
	err := h.CreateOrganization(CreateOrganizationParams{
		Name:        "Acme",
		Description: &description,
	})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	var count int
	var name, desc string
	row := pool.QueryRow(context.Background(), `SELECT COUNT(*), MAX(name), MAX(description) FROM sirkel_engine.organizations`)
	if err := row.Scan(&count, &name, &desc); err != nil {
		t.Fatalf("unable to read organization: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 organization, got %d", count)
	}
	if name != "Acme" {
		t.Fatalf("expected name %q, got %q", "Acme", name)
	}
	if desc != description {
		t.Fatalf("expected description %q, got %q", description, desc)
	}
}

func TestSirkelAuthHandler_CreateOrganization_NilDescription(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})

	err := h.CreateOrganization(CreateOrganizationParams{Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	var desc *string
	row := pool.QueryRow(context.Background(), `SELECT description FROM sirkel_engine.organizations LIMIT 1`)
	if err := row.Scan(&desc); err != nil {
		t.Fatalf("unable to read organization: %v", err)
	}
	if desc != nil {
		t.Fatalf("expected nil description, got %v", *desc)
	}
}

func TestSirkelAuthHandler_CreateOrganization_RequiresName(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})

	err := h.CreateOrganization(CreateOrganizationParams{Name: ""})
	assertErrorCode(t, err, sirkel_errors.CodeOrganizationNameRequired)
}
