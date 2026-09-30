package sirkel_auth

import (
	"context"

	"sirkel-engine-lib/sirkel_domain"
	"sirkel-engine-lib/sirkel_errors"

	"github.com/jackc/pgx/v5"
)

// requireSuperAdmin gates every membership-mutating call in this file: only a
// super_admin may add, remove, or move a user's organization membership.
func (h *SirkelAuthHandler) requireSuperAdmin() error {
	if h.User == nil || h.User.Role != sirkel_domain.RoleSuperAdmin {
		return sirkel_errors.New(sirkel_errors.CodeForbidden, "only a super admin can manage organization memberships")
	}
	return nil
}

type AddMembershipParams struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	Role           string `json:"role"`
}

// AddMembership gives a user access to an additional organization, without
// changing which organization is currently theirs (see MoveUserOrganization
// for that). Calling it again for the same user/organization pair updates
// the role on that membership.
func (h *SirkelAuthHandler) AddMembership(params AddMembershipParams) error {
	if err := h.requireSuperAdmin(); err != nil {
		return err
	}
	if params.UserID == "" {
		return sirkel_errors.New(sirkel_errors.CodeUserIDRequired, "user id is required")
	}
	if params.OrganizationID == "" {
		return sirkel_errors.New(sirkel_errors.CodeOrganizationIDRequired, "organization id is required")
	}
	if params.Role == "" {
		return sirkel_errors.New(sirkel_errors.CodeRoleRequired, "role is required")
	}

	query := `
		INSERT INTO sirkel_engine.user_organizations (user_id, organization_id, role, is_current)
		VALUES ($1, $2, $3, false)
		ON CONFLICT (user_id, organization_id)
		DO UPDATE SET role = EXCLUDED.role, updated_at = NOW()
	`
	if _, err := h.Pool.Exec(context.Background(), query, params.UserID, params.OrganizationID, params.Role); err != nil {
		return sirkel_errors.Wrap(sirkel_errors.CodeMembershipAddFailed, "failed to add membership", err)
	}

	return nil
}

type RemoveMembershipParams struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
}

// RemoveMembership revokes a user's access to an organization. It refuses to
// remove the user's current organization — move them somewhere else first
// with MoveUserOrganization, so they are never left without one.
func (h *SirkelAuthHandler) RemoveMembership(params RemoveMembershipParams) error {
	if err := h.requireSuperAdmin(); err != nil {
		return err
	}
	if params.UserID == "" {
		return sirkel_errors.New(sirkel_errors.CodeUserIDRequired, "user id is required")
	}
	if params.OrganizationID == "" {
		return sirkel_errors.New(sirkel_errors.CodeOrganizationIDRequired, "organization id is required")
	}

	ctx := context.Background()

	var isCurrent bool
	err := h.Pool.QueryRow(ctx, `
		SELECT is_current FROM sirkel_engine.user_organizations WHERE user_id = $1 AND organization_id = $2
	`, params.UserID, params.OrganizationID).Scan(&isCurrent)
	if err != nil {
		if err == pgx.ErrNoRows {
			return sirkel_errors.New(sirkel_errors.CodeMembershipNotFound, "membership not found")
		}
		return sirkel_errors.Wrap(sirkel_errors.CodeMembershipRemoveFailed, "failed to remove membership", err)
	}
	if isCurrent {
		return sirkel_errors.New(sirkel_errors.CodeCannotRemoveCurrentOrgMembership, "move the user to a different organization before removing this one")
	}

	result, err := h.Pool.Exec(ctx, `
		DELETE FROM sirkel_engine.user_organizations WHERE user_id = $1 AND organization_id = $2
	`, params.UserID, params.OrganizationID)
	if err != nil {
		return sirkel_errors.Wrap(sirkel_errors.CodeMembershipRemoveFailed, "failed to remove membership", err)
	}
	if result.RowsAffected() == 0 {
		return sirkel_errors.New(sirkel_errors.CodeMembershipNotFound, "membership not found")
	}

	return nil
}

type MoveUserOrganizationParams struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	// Role is the user's role in OrganizationID. Required when the user has
	// no existing membership there; if they already belong to it, an empty
	// Role keeps their existing role in that organization.
	Role string `json:"role,omitempty"`
}

// MoveUserOrganization makes OrganizationID the user's current organization.
// It does not delete their membership in whatever organization was current
// before the move — that membership just stops being current, and can be
// removed separately with RemoveMembership if the user is leaving it for
// good. This also keeps users.organization_id/role (the legacy columns
// several queries still join on) in sync with the new current membership.
//
// It does not touch the moved user's existing token: JWTs are self-contained,
// so there is nothing here to push an update into. That token keeps carrying
// the old organization/role until it expires or the user calls RefreshToken.
func (h *SirkelAuthHandler) MoveUserOrganization(params MoveUserOrganizationParams) error {
	if err := h.requireSuperAdmin(); err != nil {
		return err
	}
	if params.UserID == "" {
		return sirkel_errors.New(sirkel_errors.CodeUserIDRequired, "user id is required")
	}
	if params.OrganizationID == "" {
		return sirkel_errors.New(sirkel_errors.CodeOrganizationIDRequired, "organization id is required")
	}

	ctx := context.Background()
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return sirkel_errors.Wrap(sirkel_errors.CodeMoveUserOrganizationFailed, "failed to move user", err)
	}
	defer tx.Rollback(ctx)

	if params.Role == "" {
		err := tx.QueryRow(ctx, `
			SELECT role FROM sirkel_engine.user_organizations WHERE user_id = $1 AND organization_id = $2
		`, params.UserID, params.OrganizationID).Scan(&params.Role)
		if err != nil {
			if err == pgx.ErrNoRows {
				return sirkel_errors.New(sirkel_errors.CodeRoleRequired, "role is required when the user has no existing membership in this organization")
			}
			return sirkel_errors.Wrap(sirkel_errors.CodeMoveUserOrganizationFailed, "failed to move user", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE sirkel_engine.user_organizations SET is_current = false, updated_at = NOW()
		WHERE user_id = $1 AND is_current
	`, params.UserID); err != nil {
		return sirkel_errors.Wrap(sirkel_errors.CodeMoveUserOrganizationFailed, "failed to move user", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO sirkel_engine.user_organizations (user_id, organization_id, role, is_current)
		VALUES ($1, $2, $3, true)
		ON CONFLICT (user_id, organization_id)
		DO UPDATE SET is_current = true, role = EXCLUDED.role, updated_at = NOW()
	`, params.UserID, params.OrganizationID, params.Role); err != nil {
		return sirkel_errors.Wrap(sirkel_errors.CodeMoveUserOrganizationFailed, "failed to move user", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE sirkel_engine.users SET organization_id = $2, role = $3, updated_at = NOW() WHERE id = $1
	`, params.UserID, params.OrganizationID, params.Role); err != nil {
		return sirkel_errors.Wrap(sirkel_errors.CodeMoveUserOrganizationFailed, "failed to move user", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return sirkel_errors.Wrap(sirkel_errors.CodeMoveUserOrganizationFailed, "failed to move user", err)
	}

	return nil
}

type ListUserOrganizationsParams struct {
	UserID string `json:"user_id"`
}

// ListUserOrganizations returns every organization a user belongs to, most
// recently joined first.
func (h *SirkelAuthHandler) ListUserOrganizations(params ListUserOrganizationsParams) ([]sirkel_domain.UserOrganization, error) {
	if params.UserID == "" {
		return nil, sirkel_errors.New(sirkel_errors.CodeUserIDRequired, "user id is required")
	}

	rows, err := h.Pool.Query(context.Background(), `
		SELECT user_id, organization_id, role, is_current
		FROM sirkel_engine.user_organizations
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, params.UserID)
	if err != nil {
		return nil, sirkel_errors.Wrap(sirkel_errors.CodeUserOrganizationsIndexFailed, "failed to list user organizations", err)
	}
	defer rows.Close()

	memberships := []sirkel_domain.UserOrganization{}
	for rows.Next() {
		var m sirkel_domain.UserOrganization
		if err := rows.Scan(&m.UserID, &m.OrganizationID, &m.Role, &m.IsCurrent); err != nil {
			return nil, sirkel_errors.Wrap(sirkel_errors.CodeUserOrganizationsIndexFailed, "failed to read user organization row", err)
		}
		memberships = append(memberships, m)
	}
	if err := rows.Err(); err != nil {
		return nil, sirkel_errors.Wrap(sirkel_errors.CodeUserOrganizationsIndexFailed, "failed to list user organizations", err)
	}

	return memberships, nil
}
