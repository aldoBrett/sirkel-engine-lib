package sirkel_auth

import (
	"context"
	"testing"

	"sirkel-engine-lib/sirkel_domain"
	"sirkel-engine-lib/sirkel_errors"
)

func newSuperAdminActor(userID string) *sirkel_domain.User {
	return &sirkel_domain.User{ID: userID, Role: sirkel_domain.RoleSuperAdmin}
}

func TestSirkelAuthHandler_AddMembership_RequiresSuperAdmin(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	userID := insertTestUser(t, pool, organizationID, "user@example.com", "hash", "user")
	otherOrgID := insertTestOrganization(t, pool, "Globex")

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool, User: &sirkel_domain.User{ID: userID, Role: "user"}})
	err := h.AddMembership(AddMembershipParams{UserID: userID, OrganizationID: otherOrgID, Role: "member"})
	assertErrorCode(t, err, sirkel_errors.CodeForbidden)
}

func TestSirkelAuthHandler_AddMembership_CreatesNonCurrentMembership(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	userID := insertTestUser(t, pool, organizationID, "user@example.com", "hash", "user")
	otherOrgID := insertTestOrganization(t, pool, "Globex")
	adminID := insertTestUser(t, pool, organizationID, "admin@example.com", "hash", sirkel_domain.RoleSuperAdmin)

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool, User: newSuperAdminActor(adminID)})
	if err := h.AddMembership(AddMembershipParams{UserID: userID, OrganizationID: otherOrgID, Role: "member"}); err != nil {
		t.Fatalf("AddMembership() error = %v", err)
	}

	var role string
	var isCurrent bool
	row := pool.QueryRow(context.Background(), `SELECT role, is_current FROM sirkel_engine.user_organizations WHERE user_id = $1 AND organization_id = $2`, userID, otherOrgID)
	if err := row.Scan(&role, &isCurrent); err != nil {
		t.Fatalf("unable to read membership: %v", err)
	}
	if role != "member" {
		t.Fatalf("expected role %q, got %q", "member", role)
	}
	if isCurrent {
		t.Fatalf("expected new membership to not be current")
	}

	// The user's legacy organization_id/role snapshot should be untouched.
	var organizationIDOnUser string
	row = pool.QueryRow(context.Background(), `SELECT organization_id FROM sirkel_engine.users WHERE id = $1`, userID)
	if err := row.Scan(&organizationIDOnUser); err != nil {
		t.Fatalf("unable to read user: %v", err)
	}
	if organizationIDOnUser != organizationID {
		t.Fatalf("expected user's current organization to remain %q, got %q", organizationID, organizationIDOnUser)
	}
}

func TestSirkelAuthHandler_MoveUserOrganization_SwitchesCurrentOrganization(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	otherOrgID := insertTestOrganization(t, pool, "Globex")
	userID := insertTestUser(t, pool, organizationID, "user@example.com", "hash", "user")
	adminID := insertTestUser(t, pool, organizationID, "admin@example.com", "hash", sirkel_domain.RoleSuperAdmin)

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool, User: newSuperAdminActor(adminID)})
	if err := h.MoveUserOrganization(MoveUserOrganizationParams{UserID: userID, OrganizationID: otherOrgID, Role: "member"}); err != nil {
		t.Fatalf("MoveUserOrganization() error = %v", err)
	}

	// The legacy snapshot on users should follow the move.
	var organizationIDOnUser, roleOnUser string
	row := pool.QueryRow(context.Background(), `SELECT organization_id, role FROM sirkel_engine.users WHERE id = $1`, userID)
	if err := row.Scan(&organizationIDOnUser, &roleOnUser); err != nil {
		t.Fatalf("unable to read user: %v", err)
	}
	if organizationIDOnUser != otherOrgID {
		t.Fatalf("expected user's organization_id to become %q, got %q", otherOrgID, organizationIDOnUser)
	}
	if roleOnUser != "member" {
		t.Fatalf("expected user's role to become %q, got %q", "member", roleOnUser)
	}

	// The old membership should still exist, just no longer current.
	var oldIsCurrent bool
	row = pool.QueryRow(context.Background(), `SELECT is_current FROM sirkel_engine.user_organizations WHERE user_id = $1 AND organization_id = $2`, userID, organizationID)
	if err := row.Scan(&oldIsCurrent); err != nil {
		t.Fatalf("unable to read old membership: %v", err)
	}
	if oldIsCurrent {
		t.Fatalf("expected old membership to no longer be current")
	}

	var newIsCurrent bool
	row = pool.QueryRow(context.Background(), `SELECT is_current FROM sirkel_engine.user_organizations WHERE user_id = $1 AND organization_id = $2`, userID, otherOrgID)
	if err := row.Scan(&newIsCurrent); err != nil {
		t.Fatalf("unable to read new membership: %v", err)
	}
	if !newIsCurrent {
		t.Fatalf("expected new membership to be current")
	}
}

func TestSirkelAuthHandler_MoveUserOrganization_RequiresRoleForNewMembership(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	otherOrgID := insertTestOrganization(t, pool, "Globex")
	userID := insertTestUser(t, pool, organizationID, "user@example.com", "hash", "user")
	adminID := insertTestUser(t, pool, organizationID, "admin@example.com", "hash", sirkel_domain.RoleSuperAdmin)

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool, User: newSuperAdminActor(adminID)})
	err := h.MoveUserOrganization(MoveUserOrganizationParams{UserID: userID, OrganizationID: otherOrgID})
	assertErrorCode(t, err, sirkel_errors.CodeRoleRequired)
}

func TestSirkelAuthHandler_RemoveMembership_RefusesCurrentOrganization(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	userID := insertTestUser(t, pool, organizationID, "user@example.com", "hash", "user")
	adminID := insertTestUser(t, pool, organizationID, "admin@example.com", "hash", sirkel_domain.RoleSuperAdmin)

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool, User: newSuperAdminActor(adminID)})
	err := h.RemoveMembership(RemoveMembershipParams{UserID: userID, OrganizationID: organizationID})
	assertErrorCode(t, err, sirkel_errors.CodeCannotRemoveCurrentOrgMembership)
}

func TestSirkelAuthHandler_RemoveMembership_RemovesNonCurrentMembership(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	otherOrgID := insertTestOrganization(t, pool, "Globex")
	userID := insertTestUser(t, pool, organizationID, "user@example.com", "hash", "user")
	adminID := insertTestUser(t, pool, organizationID, "admin@example.com", "hash", sirkel_domain.RoleSuperAdmin)

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool, User: newSuperAdminActor(adminID)})
	if err := h.AddMembership(AddMembershipParams{UserID: userID, OrganizationID: otherOrgID, Role: "member"}); err != nil {
		t.Fatalf("AddMembership() error = %v", err)
	}
	if err := h.RemoveMembership(RemoveMembershipParams{UserID: userID, OrganizationID: otherOrgID}); err != nil {
		t.Fatalf("RemoveMembership() error = %v", err)
	}

	var count int
	row := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM sirkel_engine.user_organizations WHERE user_id = $1 AND organization_id = $2`, userID, otherOrgID)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("unable to count memberships: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected membership to be removed, found %d rows", count)
	}
}

func TestSirkelAuthHandler_ListUserOrganizations(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	otherOrgID := insertTestOrganization(t, pool, "Globex")
	userID := insertTestUser(t, pool, organizationID, "user@example.com", "hash", "user")
	adminID := insertTestUser(t, pool, organizationID, "admin@example.com", "hash", sirkel_domain.RoleSuperAdmin)

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool, User: newSuperAdminActor(adminID)})
	if err := h.AddMembership(AddMembershipParams{UserID: userID, OrganizationID: otherOrgID, Role: "member"}); err != nil {
		t.Fatalf("AddMembership() error = %v", err)
	}

	memberships, err := h.ListUserOrganizations(ListUserOrganizationsParams{UserID: userID})
	if err != nil {
		t.Fatalf("ListUserOrganizations() error = %v", err)
	}
	if len(memberships) != 2 {
		t.Fatalf("expected 2 memberships, got %d", len(memberships))
	}
}
