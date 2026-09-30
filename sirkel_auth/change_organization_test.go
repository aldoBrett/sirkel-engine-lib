package sirkel_auth

import (
	"context"
	"testing"

	"sirkel-engine-lib/sirkel_domain"
	"sirkel-engine-lib/sirkel_errors"

	"github.com/golang-jwt/jwt"
)

func TestSirkelAuthHandler_ChangeOrganization_SwitchesToOwnedOrganization(t *testing.T) {
	pool := testPool(t)
	t.Setenv("JWT_SECRET", "test-secret")

	organizationID := insertTestOrganization(t, pool, "Acme")
	otherOrgID := insertTestOrganization(t, pool, "Globex")
	userID := insertTestUser(t, pool, organizationID, "user@example.com", "hash", "user")
	adminID := insertTestUser(t, pool, organizationID, "admin@example.com", "hash", sirkel_domain.RoleSuperAdmin)

	admin := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool, User: newSuperAdminActor(adminID)})
	if err := admin.AddMembership(AddMembershipParams{UserID: userID, OrganizationID: otherOrgID, Role: "manager"}); err != nil {
		t.Fatalf("AddMembership() error = %v", err)
	}

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool, User: &sirkel_domain.User{ID: userID}})
	resp, err := h.ChangeOrganization(ChangeOrganizationParams{OrganizationID: otherOrgID})
	if err != nil {
		t.Fatalf("ChangeOrganization() error = %v", err)
	}
	if resp.Role != "manager" {
		t.Fatalf("expected role %q, got %q", "manager", resp.Role)
	}
	if resp.Token == "" {
		t.Fatal("expected non-empty token")
	}

	token, err := jwt.Parse(resp.Token, func(*jwt.Token) (interface{}, error) {
		return []byte("test-secret"), nil
	})
	if err != nil {
		t.Fatalf("unable to parse token: %v", err)
	}
	claims := token.Claims.(jwt.MapClaims)
	if claims["organization_id"] != otherOrgID {
		t.Fatalf("expected organization_id claim %q, got %v", otherOrgID, claims["organization_id"])
	}

	// The old membership stays, just no longer current; the new one is.
	var oldCurrent, newCurrent bool
	pool.QueryRow(context.Background(), `SELECT is_current FROM sirkel_engine.user_organizations WHERE user_id = $1 AND organization_id = $2`, userID, organizationID).Scan(&oldCurrent)
	pool.QueryRow(context.Background(), `SELECT is_current FROM sirkel_engine.user_organizations WHERE user_id = $1 AND organization_id = $2`, userID, otherOrgID).Scan(&newCurrent)
	if oldCurrent {
		t.Fatalf("expected old membership to no longer be current")
	}
	if !newCurrent {
		t.Fatalf("expected new membership to be current")
	}

	var organizationIDOnUser, roleOnUser string
	pool.QueryRow(context.Background(), `SELECT organization_id, role FROM sirkel_engine.users WHERE id = $1`, userID).Scan(&organizationIDOnUser, &roleOnUser)
	if organizationIDOnUser != otherOrgID || roleOnUser != "manager" {
		t.Fatalf("expected user snapshot to follow the change, got organization_id=%q role=%q", organizationIDOnUser, roleOnUser)
	}
}

func TestSirkelAuthHandler_ChangeOrganization_RefusesOrganizationNotBelongedTo(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	otherOrgID := insertTestOrganization(t, pool, "Globex")
	userID := insertTestUser(t, pool, organizationID, "user@example.com", "hash", "user")

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool, User: &sirkel_domain.User{ID: userID}})
	_, err := h.ChangeOrganization(ChangeOrganizationParams{OrganizationID: otherOrgID})
	assertErrorCode(t, err, sirkel_errors.CodeMembershipNotFound)
}

func TestSirkelAuthHandler_ChangeOrganization_RequiresAuthenticatedUser(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	_, err := h.ChangeOrganization(ChangeOrganizationParams{OrganizationID: organizationID})
	assertErrorCode(t, err, sirkel_errors.CodeUserIDRequired)
}
