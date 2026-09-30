package sirkel_auth

import (
	"context"
	"errors"
	"sirkel-engine-lib/sirkel_errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

// postgresUniqueViolation is the SQLSTATE code Postgres returns when a UNIQUE
// constraint (here, users.email) is violated.
const postgresUniqueViolation = "23505"

type CreateUserParams struct {
	OrganizationID string `json:"organization_id"`
	// Role is the user's role within OrganizationID (the membership this call
	// creates as their current one). Defaults to "user".
	Role          *string `json:"role,omitempty"`
	Email         string  `json:"email"`
	Password      string  `json:"password"`
	Name          string  `json:"name"`
	FirstSurname  string  `json:"first_surname"`
	SecondSurname string  `json:"second_surname"`
	Phone         *string `json:"phone,omitempty"`
}

func (h *SirkelAuthHandler) CreateUser(params CreateUserParams) error {
	if params.OrganizationID == "" {
		return sirkel_errors.New(sirkel_errors.CodeOrganizationIDRequired, "organization id is required")
	}
	if params.Email == "" {
		return sirkel_errors.New(sirkel_errors.CodeEmailRequired, "email is required")
	}
	if params.Password == "" {
		return sirkel_errors.New(sirkel_errors.CodePasswordRequired, "password is required")
	}
	if params.Name == "" {
		return sirkel_errors.New(sirkel_errors.CodeNameRequired, "name is required")
	}
	if params.FirstSurname == "" {
		return sirkel_errors.New(sirkel_errors.CodeFirstSurnameRequired, "first surname is required")
	}
	if params.SecondSurname == "" {
		return sirkel_errors.New(sirkel_errors.CodeSecondSurnameRequired, "second surname is required")
	}

	// Hash the password using the secret
	hash, err := bcrypt.GenerateFromPassword([]byte(params.Password), 10)
	if err != nil {
		return sirkel_errors.Wrap(sirkel_errors.CodePasswordHashFailed, "failed to hash password", err)
	}
	role := "user"
	if params.Role != nil && *params.Role != "" {
		role = *params.Role
	}
	userID := uuid.New()

	ctx := context.Background()
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return sirkel_errors.Wrap(sirkel_errors.CodeUserCreateFailed, "failed to create user", err)
	}
	defer tx.Rollback(ctx)

	// users.organization_id/role stay in sync as a snapshot of the current
	// membership below; see 016_create_user_organizations.sql.
	userQuery := `INSERT INTO sirkel_engine.users (id, email, password_hash, name, first_surname, second_surname, phone, role, organization_id, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())`
	if _, err = tx.Exec(ctx, userQuery, userID, params.Email, string(hash), params.Name, params.FirstSurname, params.SecondSurname, params.Phone, role, params.OrganizationID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			return sirkel_errors.Wrap(sirkel_errors.CodeEmailAlreadyExists, "a user with this email already exists", err)
		}
		return sirkel_errors.Wrap(sirkel_errors.CodeUserCreateFailed, "failed to create user", err)
	}

	membershipQuery := `INSERT INTO sirkel_engine.user_organizations (user_id, organization_id, role, is_current) VALUES ($1, $2, $3, true)`
	if _, err = tx.Exec(ctx, membershipQuery, userID, params.OrganizationID, role); err != nil {
		return sirkel_errors.Wrap(sirkel_errors.CodeUserCreateFailed, "failed to create user", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return sirkel_errors.Wrap(sirkel_errors.CodeUserCreateFailed, "failed to create user", err)
	}

	// TODO: enqueue confirmation email

	return nil
}
