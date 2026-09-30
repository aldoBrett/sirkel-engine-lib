package sirkel_auth

import (
	"context"

	"sirkel-engine-lib/sirkel_errors"

	"github.com/google/uuid"
)

type RefreshTokenResponse struct {
	Token string `json:"token"`
	Role  string `json:"role"`
	Email string `json:"email"`
}

// RefreshToken re-signs a token for the caller (h.User, populated from their current, still-valid token) using
// their role and organization as they stand in the database right now, rather than whatever was true when their
// existing token was issued. There is no way to push an update into a token already in someone else's hands —
// JWTs are self-contained — so after MoveUserOrganization (or any role change) the moved user's existing token
// keeps its old claims until either it expires or they call this. It never takes a user id from the caller: it
// only ever refreshes the token of whoever is already authenticated, so it can't be used to mint a token for
// someone else.
func (h *SirkelAuthHandler) RefreshToken() (RefreshTokenResponse, error) {
	if h.User == nil || h.User.ID == "" {
		return RefreshTokenResponse{}, sirkel_errors.New(sirkel_errors.CodeUserIDRequired, "an authenticated user is required")
	}

	return h.tokenForCurrentState(context.Background(), h.User.ID)
}

// tokenForCurrentState reads a user's current role/organization/email from
// the database and issues a token for it. ChangeOrganization builds its
// response on this too, once it has switched the user's current
// organization.
func (h *SirkelAuthHandler) tokenForCurrentState(ctx context.Context, userID string) (RefreshTokenResponse, error) {
	var id, organizationID uuid.UUID
	var role, email string
	err := h.Pool.QueryRow(ctx, `
		SELECT id, role, email, organization_id FROM sirkel_engine.users WHERE id = $1
	`, userID).Scan(&id, &role, &email, &organizationID)
	if err != nil {
		return RefreshTokenResponse{}, sirkel_errors.Wrap(sirkel_errors.CodeUserNotFound, "user not found", err)
	}

	token, err := issueToken(id, role, organizationID)
	if err != nil {
		return RefreshTokenResponse{}, err
	}

	return RefreshTokenResponse{Token: token, Role: role, Email: email}, nil
}
