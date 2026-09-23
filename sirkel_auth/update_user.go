package sirkel_auth

import (
	"context"
	"fmt"
	"sirkel-engine-lib/sirkel_errors"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type UpdateUserParams struct {
	UserID         string  `json:"user_id"`
	Email          *string `json:"email,omitempty"`
	Password       *string `json:"password,omitempty"`
	Name           *string `json:"name,omitempty"`
	FirstSurname   *string `json:"first_surname,omitempty"`
	SecondSurname  *string `json:"second_surname,omitempty"`
	Phone          *string `json:"phone,omitempty"`
	Role           *string `json:"role,omitempty"`
	OrganizationID *string `json:"organization_id,omitempty"`
}

func (h *SirkelAuthHandler) UpdateUser(params UpdateUserParams) error {
	if params.UserID == "" {
		return sirkel_errors.New(sirkel_errors.CodeUserIDRequired, "user id is required")
	}

	setClauses := []string{}
	args := []any{}

	addSet := func(column string, value any) {
		args = append(args, value)
		setClauses = append(setClauses, column+" = $"+strconv.Itoa(len(args)))
	}

	if params.Email != nil {
		addSet("email", *params.Email)
	}
	if params.Name != nil {
		addSet("name", *params.Name)
	}
	if params.FirstSurname != nil {
		addSet("first_surname", *params.FirstSurname)
	}
	if params.SecondSurname != nil {
		addSet("second_surname", *params.SecondSurname)
	}
	if params.Phone != nil {
		addSet("phone", *params.Phone)
	}
	if params.Role != nil {
		addSet("role", *params.Role)
	}
	if params.OrganizationID != nil {
		addSet("organization_id", *params.OrganizationID)
	}
	if params.Password != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(*params.Password), 10)
		if err != nil {
			return sirkel_errors.Wrap(sirkel_errors.CodePasswordHashFailed, "failed to hash password", err)
		}
		addSet("password_hash", string(hash))
	}

	if len(setClauses) == 0 {
		return sirkel_errors.New(sirkel_errors.CodeNoFieldsToUpdate, "no fields to update")
	}
	setClauses = append(setClauses, "updated_at = NOW()")

	args = append(args, params.UserID)
	query := fmt.Sprintf(
		`UPDATE sirkel_engine.users SET %s WHERE id = $%d`,
		strings.Join(setClauses, ", "), len(args),
	)

	result, err := h.Pool.Exec(context.Background(), query, args...)
	if err != nil {
		return sirkel_errors.Wrap(sirkel_errors.CodeUserUpdateFailed, "failed to update user", err)
	}
	if result.RowsAffected() == 0 {
		return sirkel_errors.New(sirkel_errors.CodeUserNotFound, "user not found")
	}

	return nil
}
