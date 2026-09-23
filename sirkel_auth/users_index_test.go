package sirkel_auth

import (
	"testing"

	"sirkel-engine-lib/sirkel_errors"
)

func TestSirkelAuthHandler_UsersIndex_Basic(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	insertTestUser(t, pool, organizationID, "a@example.com", "hash", "user")
	insertTestUser(t, pool, organizationID, "b@example.com", "hash", "user")

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	resp, err := h.UsersIndex(UsersIndexParams{})
	if err != nil {
		t.Fatalf("UsersIndex() error = %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("expected total 2, got %d", resp.Total)
	}
	if len(resp.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(resp.Users))
	}
}

func TestSirkelAuthHandler_UsersIndex_FiltersByOrganization(t *testing.T) {
	pool := testPool(t)
	orgA := insertTestOrganization(t, pool, "Acme")
	orgB := insertTestOrganization(t, pool, "Beta")
	insertTestUser(t, pool, orgA, "a1@example.com", "hash", "user")
	insertTestUser(t, pool, orgA, "a2@example.com", "hash", "user")
	insertTestUser(t, pool, orgB, "b1@example.com", "hash", "user")

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	resp, err := h.UsersIndex(UsersIndexParams{OrganizationID: &orgA})
	if err != nil {
		t.Fatalf("UsersIndex() error = %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("expected total 2, got %d", resp.Total)
	}
	for _, u := range resp.Users {
		if u.OrganizationID != orgA {
			t.Fatalf("expected user to belong to org %q, got %q", orgA, u.OrganizationID)
		}
	}
}

func TestSirkelAuthHandler_UsersIndex_TextSearch(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	insertTestUser(t, pool, organizationID, "findme@example.com", "hash", "user")
	insertTestUser(t, pool, organizationID, "other@example.com", "hash", "user")

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	term := "findme"
	resp, err := h.UsersIndex(UsersIndexParams{TextSearch: &term})
	if err != nil {
		t.Fatalf("UsersIndex() error = %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("expected total 1, got %d", resp.Total)
	}
	if len(resp.Users) != 1 || resp.Users[0].Email != "findme@example.com" {
		t.Fatalf("expected only findme@example.com, got %+v", resp.Users)
	}
}

func TestSirkelAuthHandler_UsersIndex_Pagination(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	for i := 0; i < 3; i++ {
		insertTestUser(t, pool, organizationID, testUUID(t)+"@example.com", "hash", "user")
	}

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	limit := 2
	offset := 1
	resp, err := h.UsersIndex(UsersIndexParams{Limit: &limit, Offset: &offset})
	if err != nil {
		t.Fatalf("UsersIndex() error = %v", err)
	}
	if resp.Total != 3 {
		t.Fatalf("expected total 3, got %d", resp.Total)
	}
	if len(resp.Users) != 2 {
		t.Fatalf("expected 2 users with limit/offset, got %d", len(resp.Users))
	}
}

func TestSirkelAuthHandler_UsersIndex_InvalidLimit(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})

	limit := 0
	_, err := h.UsersIndex(UsersIndexParams{Limit: &limit})
	assertErrorCode(t, err, sirkel_errors.CodeInvalidLimit)
}

func TestSirkelAuthHandler_UsersIndex_InvalidOffset(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})

	offset := -1
	_, err := h.UsersIndex(UsersIndexParams{Offset: &offset})
	assertErrorCode(t, err, sirkel_errors.CodeInvalidOffset)
}
