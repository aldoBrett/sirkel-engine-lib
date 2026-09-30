package sirkel_auth

import (
	"testing"

	"sirkel-engine-lib/sirkel_domain"
	"sirkel-engine-lib/sirkel_errors"

	"github.com/golang-jwt/jwt"
)

func TestSirkelAuthHandler_RefreshToken_ReflectsCurrentRoleAndOrganization(t *testing.T) {
	pool := testPool(t)
	t.Setenv("JWT_SECRET", "test-secret")

	organizationID := insertTestOrganization(t, pool, "Acme")
	userID := insertTestUser(t, pool, organizationID, "user@example.com", "hash", "user")

	// A stale token, as if issued before the user's role/organization changed.
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool, User: &sirkel_domain.User{ID: userID}})

	otherOrgID := insertTestOrganization(t, pool, "Globex")
	adminID := insertTestUser(t, pool, organizationID, "admin@example.com", "hash", sirkel_domain.RoleSuperAdmin)
	admin := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool, User: newSuperAdminActor(adminID)})
	if err := admin.MoveUserOrganization(MoveUserOrganizationParams{UserID: userID, OrganizationID: otherOrgID, Role: "manager"}); err != nil {
		t.Fatalf("MoveUserOrganization() error = %v", err)
	}

	resp, err := h.RefreshToken()
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if resp.Role != "manager" {
		t.Fatalf("expected refreshed role %q, got %q", "manager", resp.Role)
	}
	if resp.Email != "user@example.com" {
		t.Fatalf("expected email %q, got %q", "user@example.com", resp.Email)
	}

	token, err := jwt.Parse(resp.Token, func(*jwt.Token) (interface{}, error) {
		return []byte("test-secret"), nil
	})
	if err != nil {
		t.Fatalf("unable to parse token: %v", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		t.Fatalf("expected valid claims, got %+v", token.Claims)
	}
	if claims["organization_id"] != otherOrgID {
		t.Fatalf("expected organization_id claim %q, got %v", otherOrgID, claims["organization_id"])
	}
	if claims["role"] != "manager" {
		t.Fatalf("expected role claim %q, got %v", "manager", claims["role"])
	}
}

func TestSirkelAuthHandler_RefreshToken_RequiresAuthenticatedUser(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})

	_, err := h.RefreshToken()
	assertErrorCode(t, err, sirkel_errors.CodeUserIDRequired)
}
