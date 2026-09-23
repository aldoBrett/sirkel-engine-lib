package sirkel_auth

import (
	"context"
	"fmt"
	"sirkel-engine-lib/sirkel_domain"
	"sirkel-engine-lib/sirkel_errors"
	"strconv"
	"strings"
)

const (
	defaultUsersIndexLimit = 20
	maxUsersIndexLimit     = 100
)

type UsersIndexParams struct {
	OrganizationID *string `json:"organization_id,omitempty"`
	// TextSearch is an optional search string that lets us search email,
	// name, firstSurname, and lastSurname fields.
	TextSearch *string `json:"text_search,omitempty"`
	Offset     *int    `json:"offset,omitempty"`
	Limit      *int    `json:"limit,omitempty"`
}
type UsersIndexResponse struct {
	Users  []sirkel_domain.UserComplete `json:"users"`
	Total  int                          `json:"total"`
	Offset int                          `json:"offset"`
	Limit  int                          `json:"limit"`
}

func (h *SirkelAuthHandler) UsersIndex(params UsersIndexParams) (*UsersIndexResponse, error) {
	limit := defaultUsersIndexLimit
	if params.Limit != nil {
		limit = *params.Limit
	}
	if limit < 1 {
		return nil, sirkel_errors.New(sirkel_errors.CodeInvalidLimit, "limit must be at least 1")
	}
	if limit > maxUsersIndexLimit {
		limit = maxUsersIndexLimit
	}

	offset := 0
	if params.Offset != nil {
		offset = *params.Offset
	}
	if offset < 0 {
		return nil, sirkel_errors.New(sirkel_errors.CodeInvalidOffset, "offset must not be negative")
	}

	ctx := context.Background()

	conditions := []string{}
	args := []any{}
	if params.OrganizationID != nil {
		args = append(args, *params.OrganizationID)
		conditions = append(conditions, "organization_id = $"+strconv.Itoa(len(args)))
	}
	if params.TextSearch != nil {
		if term := strings.TrimSpace(*params.TextSearch); term != "" {
			args = append(args, "%"+term+"%")
			idx := strconv.Itoa(len(args))
			conditions = append(conditions, "(email ILIKE $"+idx+" OR name ILIKE $"+idx+" OR first_surname ILIKE $"+idx+" OR second_surname ILIKE $"+idx+")")
		}
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	response := &UsersIndexResponse{
		Users:  []sirkel_domain.UserComplete{},
		Offset: offset,
		Limit:  limit,
	}

	countQuery := `SELECT COUNT(*) FROM sirkel_engine.users` + where
	if err := h.Pool.QueryRow(ctx, countQuery, args...).Scan(&response.Total); err != nil {
		return nil, sirkel_errors.Wrap(sirkel_errors.CodeUsersIndexFailed, "failed to count users", err)
	}

	// organization_id is nullable (a user may not belong to an organization yet),
	// but tickets_domain.User.OrganizationID is a plain string, so coalesce it.
	listArgs := append(append([]any{}, args...), limit, offset)
	listQuery := fmt.Sprintf(
		`SELECT id, COALESCE(organization_id::text, ''), email, role, name, first_surname, second_surname, phone FROM sirkel_engine.users%s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, len(listArgs)-1, len(listArgs),
	)

	rows, err := h.Pool.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, sirkel_errors.Wrap(sirkel_errors.CodeUsersIndexFailed, "failed to list users", err)
	}
	defer rows.Close()

	for rows.Next() {
		var user sirkel_domain.UserComplete
		if err := rows.Scan(&user.ID, &user.OrganizationID, &user.Email, &user.Role, &user.Name, &user.FirstSurname, &user.SecondSurname, &user.Phone); err != nil {
			return nil, sirkel_errors.Wrap(sirkel_errors.CodeUsersIndexFailed, "failed to read user row", err)
		}
		response.Users = append(response.Users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, sirkel_errors.Wrap(sirkel_errors.CodeUsersIndexFailed, "failed to list users", err)
	}

	return response, nil
}
