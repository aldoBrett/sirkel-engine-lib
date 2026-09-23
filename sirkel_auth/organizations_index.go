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
	defaultOrganizationsIndexLimit = 20
	maxOrganizationsIndexLimit     = 100
)

type OrganizationsIndexParams struct {
	// TextSearch is an optional search string that lets us search the name
	// and description fields.
	TextSearch *string `json:"text_search,omitempty"`
	Offset     *int    `json:"offset,omitempty"`
	Limit      *int    `json:"limit,omitempty"`
}

type OrganizationsIndexResponse struct {
	Organizations []sirkel_domain.Organization `json:"organizations"`
	Total         int                          `json:"total"`
	Offset        int                          `json:"offset"`
	Limit         int                          `json:"limit"`
}

func (h *SirkelAuthHandler) OrganizationsIndex(params OrganizationsIndexParams) (*OrganizationsIndexResponse, error) {
	limit := defaultOrganizationsIndexLimit
	if params.Limit != nil {
		limit = *params.Limit
	}
	if limit < 1 {
		return nil, sirkel_errors.New(sirkel_errors.CodeInvalidLimit, "limit must be at least 1")
	}
	if limit > maxOrganizationsIndexLimit {
		limit = maxOrganizationsIndexLimit
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
	if params.TextSearch != nil {
		if term := strings.TrimSpace(*params.TextSearch); term != "" {
			args = append(args, "%"+term+"%")
			idx := strconv.Itoa(len(args))
			conditions = append(conditions, "(name ILIKE $"+idx+" OR description ILIKE $"+idx+")")
		}
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	response := &OrganizationsIndexResponse{
		Organizations: []sirkel_domain.Organization{},
		Offset:        offset,
		Limit:         limit,
	}

	countQuery := `SELECT COUNT(*) FROM sirkel_engine.organizations` + where
	if err := h.Pool.QueryRow(ctx, countQuery, args...).Scan(&response.Total); err != nil {
		return nil, sirkel_errors.Wrap(sirkel_errors.CodeOrganizationsIndexFailed, "failed to count organizations", err)
	}

	// name and description are nullable columns, but tickets_domain.Organization
	// holds plain strings, so coalesce both.
	listArgs := append(append([]any{}, args...), limit, offset)
	listQuery := fmt.Sprintf(
		`SELECT id, COALESCE(name, ''), COALESCE(description, '') FROM sirkel_engine.organizations%s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, len(listArgs)-1, len(listArgs),
	)

	rows, err := h.Pool.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, sirkel_errors.Wrap(sirkel_errors.CodeOrganizationsIndexFailed, "failed to list organizations", err)
	}
	defer rows.Close()

	for rows.Next() {
		var org sirkel_domain.Organization
		if err := rows.Scan(&org.ID, &org.Name, &org.Description); err != nil {
			return nil, sirkel_errors.Wrap(sirkel_errors.CodeOrganizationsIndexFailed, "failed to read organization row", err)
		}
		response.Organizations = append(response.Organizations, org)
	}
	if err := rows.Err(); err != nil {
		return nil, sirkel_errors.Wrap(sirkel_errors.CodeOrganizationsIndexFailed, "failed to list organizations", err)
	}

	return response, nil
}
