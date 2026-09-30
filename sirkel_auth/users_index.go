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
	// OrganizationID is only honored for a super_admin, who may list any
	// organization's members, or every user when it is nil. Everyone else
	// always gets their own current organization's members.
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

	organizationID := params.OrganizationID
	if h.User == nil || h.User.Role != sirkel_domain.RoleSuperAdmin {
		if h.User == nil || h.User.OrganizationID == "" {
			return nil, sirkel_errors.New(sirkel_errors.CodeOrganizationIDRequired, "an authenticated user with a current organization is required")
		}
		organizationID = &h.User.OrganizationID
	}

	ctx := context.Background()

	// Scoped to an organization, users are listed through their membership in
	// it, so users whose current organization is another one are still
	// included, with their role in this organization. Unscoped (super_admin
	// only), each user is listed once, with their current organization/role.
	from := " FROM sirkel_engine.users u"
	orgColumns := "COALESCE(u.organization_id::text, ''), u.role"
	conditions := []string{}
	args := []any{}
	if organizationID != nil {
		args = append(args, *organizationID)
		from += " JOIN sirkel_engine.user_organizations uo ON uo.user_id = u.id AND uo.organization_id = $" + strconv.Itoa(len(args))
		orgColumns = "uo.organization_id::text, uo.role"
	}
	if params.TextSearch != nil {
		if term := strings.TrimSpace(*params.TextSearch); term != "" {
			args = append(args, "%"+term+"%")
			idx := strconv.Itoa(len(args))
			conditions = append(conditions, "(u.email ILIKE $"+idx+" OR u.name ILIKE $"+idx+" OR u.first_surname ILIKE $"+idx+" OR u.second_surname ILIKE $"+idx+")")
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

	countQuery := `SELECT COUNT(*)` + from + where
	if err := h.Pool.QueryRow(ctx, countQuery, args...).Scan(&response.Total); err != nil {
		return nil, sirkel_errors.Wrap(sirkel_errors.CodeUsersIndexFailed, "failed to count users", err)
	}

	// users.organization_id is nullable (a user may not belong to an organization yet),
	// but sirkel_domain.UserComplete.OrganizationID is a plain string, so it is coalesced.
	listArgs := append(append([]any{}, args...), limit, offset)
	listQuery := fmt.Sprintf(
		`SELECT u.id, %s, u.email, u.name, u.first_surname, u.second_surname, u.phone%s%s ORDER BY u.created_at DESC, u.id LIMIT $%d OFFSET $%d`,
		orgColumns, from, where, len(listArgs)-1, len(listArgs),
	)

	rows, err := h.Pool.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, sirkel_errors.Wrap(sirkel_errors.CodeUsersIndexFailed, "failed to list users", err)
	}
	defer rows.Close()

	for rows.Next() {
		var user sirkel_domain.UserComplete
		if err := rows.Scan(&user.ID, &user.OrganizationID, &user.Role, &user.Email, &user.Name, &user.FirstSurname, &user.SecondSurname, &user.Phone); err != nil {
			return nil, sirkel_errors.Wrap(sirkel_errors.CodeUsersIndexFailed, "failed to read user row", err)
		}
		response.Users = append(response.Users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, sirkel_errors.Wrap(sirkel_errors.CodeUsersIndexFailed, "failed to list users", err)
	}

	return response, nil
}
