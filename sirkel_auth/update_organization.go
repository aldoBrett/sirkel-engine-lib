package sirkel_auth

import (
	"context"
	"sirkel-engine-lib/sirkel_errors"
)

type UpdateOrganizationParams struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (h *SirkelAuthHandler) UpdateOrganization(params UpdateOrganizationParams) error {
	if params.ID == "" {
		return sirkel_errors.New(sirkel_errors.CodeOrganizationIDRequired, "id is required")
	}

	query := `UPDATE sirkel_engine.organizations SET name = COALESCE($2, name), description = COALESCE($3, description), updated_at = NOW() WHERE id = $1`
	result, err := h.Pool.Exec(context.Background(), query, params.ID, params.Name, params.Description)
	if err != nil {
		return sirkel_errors.Wrap(sirkel_errors.CodeOrganizationUpdateFailed, "failed to update organization", err)
	}
	if result.RowsAffected() == 0 {
		return sirkel_errors.New(sirkel_errors.CodeOrganizationNotFound, "organization not found")
	}

	return nil
}
