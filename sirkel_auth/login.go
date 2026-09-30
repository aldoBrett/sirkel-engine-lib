package sirkel_auth

import (
	"context"
	"sirkel-engine-lib/sirkel_errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type LoginParams struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
	Role  string `json:"role"`
	Email string `json:"email"`
	// OrganizationName is the name of the user's current organization, empty
	// when they have none (e.g. a super_admin) or it has no name.
	OrganizationName string `json:"organization_name"`
}

func (h *SirkelAuthHandler) Login(params LoginParams) (LoginResponse, error) {
	// 2. Traer el usuario de la DB
	var userID uuid.UUID
	var passwordHash string
	var role string
	var emailVerifiedAt *time.Time
	var organizationName string

	query := `
		SELECT u.id, u.password_hash, u.role, u.email_verified_at, u.organization_id, COALESCE(o.name, '')
		FROM sirkel_engine.users u
		LEFT JOIN sirkel_engine.organizations o ON o.id = u.organization_id
		WHERE u.email = $1
	`

	var organizationID uuid.UUID
	err := h.Pool.QueryRow(context.Background(), query, params.Email).Scan(&userID, &passwordHash, &role, &emailVerifiedAt, &organizationID, &organizationName)
	if err != nil {
		// Usamos un mensaje genérico para no revelar si el email existe o no
		return LoginResponse{}, sirkel_errors.Wrap(sirkel_errors.CodeInvalidCredentials, "credenciales inválidas", err)
	}

	// TODO: enable checking is email is verified
	// if emailVerifiedAt == nil {
	// 	return c.Status(400).JSON(fiber.Map{"error": "Usuario no verificado"})
	// }

	// Compare password
	err = bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(params.Password))
	if err != nil {
		return LoginResponse{}, sirkel_errors.Wrap(sirkel_errors.CodeInvalidCredentials, "credenciales inválidas", err)
	}

	t, err := issueToken(userID, role, organizationID)
	if err != nil {
		return LoginResponse{}, err
	}

	return LoginResponse{
		Token:            t,
		Role:             role,
		Email:            params.Email,
		OrganizationName: organizationName,
	}, nil
}
