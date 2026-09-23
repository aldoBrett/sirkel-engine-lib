package sirkel_auth

import (
	"context"
	"sirkel-engine-lib/sirkel_errors"

	"github.com/google/uuid"
)

type CreateOrganizationParams struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

func (h *SirkelAuthHandler) CreateOrganization(params CreateOrganizationParams) error {
	if params.Name == "" {
		return sirkel_errors.New(sirkel_errors.CodeOrganizationNameRequired, "organization name is required")
	}

	query := `INSERT INTO sirkel_engine.organizations (id, name, description, created_at, updated_at) VALUES ($1, $2, $3, NOW(), NOW())`
	_, err := h.Pool.Exec(context.Background(), query, uuid.New(), params.Name, params.Description)
	return err
}
