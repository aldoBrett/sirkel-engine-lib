package sirkel_auth

import (
	"context"
	"testing"

	"sirkel-engine-lib/sirkel_domain"
	"sirkel-engine-lib/sirkel_errors"
)

// usersIndexHandler returns a handler acting as a user with the given role
// whose current organization is organizationID.
func usersIndexHandler(t *testing.T, h *SirkelAuthHandler, organizationID, role string) *SirkelAuthHandler {
	t.Helper()

	h.User = &sirkel_domain.User{ID: testUUID(t), OrganizationID: organizationID, Role: role}
	return h
}

func TestSirkelAuthHandler_UsersIndex_Basic(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	insertTestUser(t, pool, organizationID, "a@example.com", "hash", "user")
	insertTestUser(t, pool, organizationID, "b@example.com", "hash", "user")

	h := usersIndexHandler(t, NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool}), "", sirkel_domain.RoleSuperAdmin)
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

func TestSirkelAuthHandler_UsersIndex_SuperAdminFiltersByOrganization(t *testing.T) {
	pool := testPool(t)
	orgA := insertTestOrganization(t, pool, "Acme")
	orgB := insertTestOrganization(t, pool, "Beta")
	insertTestUser(t, pool, orgA, "a1@example.com", "hash", "user")
	insertTestUser(t, pool, orgA, "a2@example.com", "hash", "user")
	insertTestUser(t, pool, orgB, "b1@example.com", "hash", "user")

	h := usersIndexHandler(t, NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool}), orgB, sirkel_domain.RoleSuperAdmin)
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

func TestSirkelAuthHandler_UsersIndex_ScopesToCurrentOrganization(t *testing.T) {
	pool := testPool(t)
	orgA := insertTestOrganization(t, pool, "Acme")
	orgB := insertTestOrganization(t, pool, "Beta")
	insertTestUser(t, pool, orgA, "a1@example.com", "hash", "user")
	insertTestUser(t, pool, orgB, "b1@example.com", "hash", "user")

	// A non super_admin asking for another organization still gets their own.
	h := usersIndexHandler(t, NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool}), orgA, "admin")
	resp, err := h.UsersIndex(UsersIndexParams{OrganizationID: &orgB})
	if err != nil {
		t.Fatalf("UsersIndex() error = %v", err)
	}
	if resp.Total != 1 || len(resp.Users) != 1 || resp.Users[0].Email != "a1@example.com" {
		t.Fatalf("expected only a1@example.com, got total %d: %+v", resp.Total, resp.Users)
	}
}

func TestSirkelAuthHandler_UsersIndex_IncludesMembersCurrentlyInAnotherOrganization(t *testing.T) {
	pool := testPool(t)
	orgA := insertTestOrganization(t, pool, "Acme")
	orgB := insertTestOrganization(t, pool, "Beta")
	insertTestUser(t, pool, orgA, "a1@example.com", "hash", "user")
	// Current in orgB, but also a member of orgA with a different role there.
	multiID := insertTestUser(t, pool, orgB, "multi@example.com", "hash", "user")
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO sirkel_engine.user_organizations (user_id, organization_id, role, is_current)
		VALUES ($1, $2, 'admin', false)
	`, multiID, orgA); err != nil {
		t.Fatalf("unable to add membership: %v", err)
	}

	h := usersIndexHandler(t, NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool}), orgA, "user")
	resp, err := h.UsersIndex(UsersIndexParams{})
	if err != nil {
		t.Fatalf("UsersIndex() error = %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("expected total 2, got %d: %+v", resp.Total, resp.Users)
	}
	var multi *sirkel_domain.UserComplete
	for i := range resp.Users {
		if resp.Users[i].ID == multiID {
			multi = &resp.Users[i]
		}
	}
	if multi == nil {
		t.Fatalf("expected multi@example.com to be listed, got %+v", resp.Users)
	}
	if multi.OrganizationID != orgA || multi.Role != "admin" {
		t.Fatalf("expected orgA membership with role admin, got org %q role %q", multi.OrganizationID, multi.Role)
	}
}

func TestSirkelAuthHandler_UsersIndex_RequiresCurrentOrganization(t *testing.T) {
	pool := testPool(t)

	_, err := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool}).UsersIndex(UsersIndexParams{})
	assertErrorCode(t, err, sirkel_errors.CodeOrganizationIDRequired)

	h := usersIndexHandler(t, NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool}), "", "user")
	_, err = h.UsersIndex(UsersIndexParams{})
	assertErrorCode(t, err, sirkel_errors.CodeOrganizationIDRequired)
}

func TestSirkelAuthHandler_UsersIndex_TextSearch(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	insertTestUser(t, pool, organizationID, "findme@example.com", "hash", "user")
	insertTestUser(t, pool, organizationID, "other@example.com", "hash", "user")

	h := usersIndexHandler(t, NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool}), organizationID, "user")
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

	h := usersIndexHandler(t, NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool}), organizationID, "user")
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
