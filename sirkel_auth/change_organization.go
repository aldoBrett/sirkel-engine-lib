package sirkel_auth

import (
	"context"
	"errors"

	"sirkel-engine-lib/sirkel_errors"

	"github.com/jackc/pgx/v5"
)

type ChangeOrganizationParams struct {
	OrganizationID string `json:"organization_id"`
}

// ChangeOrganization is the self-service move: a user switches which of
// their own organizations is current — e.g. the UI shows the list from
// ListUserOrganizations, the user picks one, this makes it current, and the
// fresh token in the response replaces whatever the UI has stored, all in
// one round trip. It only ever acts on h.User (never a user id from the
// caller) and only ever switches to an organization the user already
// belongs to; granting access to a new one is MoveUserOrganization's job,
// restricted to a super_admin.
func (h *SirkelAuthHandler) ChangeOrganization(params ChangeOrganizationParams) (RefreshTokenResponse, error) {
	if h.User == nil || h.User.ID == "" {
		return RefreshTokenResponse{}, sirkel_errors.New(sirkel_errors.CodeUserIDRequired, "an authenticated user is required")
	}
	if params.OrganizationID == "" {
		return RefreshTokenResponse{}, sirkel_errors.New(sirkel_errors.CodeOrganizationIDRequired, "organization id is required")
	}

	ctx := context.Background()
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return RefreshTokenResponse{}, sirkel_errors.Wrap(sirkel_errors.CodeChangeOrganizationFailed, "failed to change organization", err)
	}
	defer tx.Rollback(ctx)

	var role string
	err = tx.QueryRow(ctx, `
		SELECT role FROM sirkel_engine.user_organizations WHERE user_id = $1 AND organization_id = $2
	`, h.User.ID, params.OrganizationID).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RefreshTokenResponse{}, sirkel_errors.New(sirkel_errors.CodeMembershipNotFound, "you do not belong to this organization")
		}
		return RefreshTokenResponse{}, sirkel_errors.Wrap(sirkel_errors.CodeChangeOrganizationFailed, "failed to change organization", err)
	}

	if err := setCurrentOrganization(ctx, tx, h.User.ID, params.OrganizationID, role); err != nil {
		return RefreshTokenResponse{}, sirkel_errors.Wrap(sirkel_errors.CodeChangeOrganizationFailed, "failed to change organization", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return RefreshTokenResponse{}, sirkel_errors.Wrap(sirkel_errors.CodeChangeOrganizationFailed, "failed to change organization", err)
	}

	return h.tokenForCurrentState(ctx, h.User.ID)
}
