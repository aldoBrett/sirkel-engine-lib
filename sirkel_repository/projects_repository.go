package sirkel_repository

import (
	"context"
	"errors"
	"fmt"
	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GetProjectsParams struct {
	OrganizationID *string
	Offset         *int
	Limit          *int
}

type CountProjectParams struct {
	OrganizationID *string
}

type ProjectsRepository interface {
	SaveProject(project *sirkel_domain.Project) error
	GetProjects(params *GetProjectsParams) ([]*sirkel_domain.Project, error)
	CountProjects(params *CountProjectParams) (int, error)
	DeleteProject(projectID *string) error
	GetProjectByID(projectID *string) (*sirkel_domain.Project, error)
}

type ProjectsRepositoryHandler struct {
	ctx  context.Context
	pool *pgxpool.Pool
	user *sirkel_domain.User
}

func NewProjectsRepositoryHandler(ctx context.Context, pool *pgxpool.Pool, user *sirkel_domain.User) *ProjectsRepositoryHandler {
	return &ProjectsRepositoryHandler{
		ctx:  ctx,
		pool: pool,
		user: user,
	}
}

func (h *ProjectsRepositoryHandler) SaveProject(project *sirkel_domain.Project) error {
	if project.ID == "" {
		return h.pool.QueryRow(h.ctx, `
			INSERT INTO sirkel_engine.projects (organization_id, name, description, state)
			VALUES ($1, $2, $3, $4)
			RETURNING id, created_at, updated_at
		`, project.OrganizationID, project.Name, project.Description, project.State,
		).Scan(&project.ID, &project.CreatedAt, &project.UpdatedAt)
	}

	return h.pool.QueryRow(h.ctx, `
		UPDATE sirkel_engine.projects
		SET name = $1, description = $2, state = $3, updated_at = now()
		WHERE id = $4
		RETURNING updated_at
	`, project.Name, project.Description, project.State, project.ID,
	).Scan(&project.UpdatedAt)
}

func (h *ProjectsRepositoryHandler) GetProjects(params *GetProjectsParams) ([]*sirkel_domain.Project, error) {
	query := `
		SELECT id, organization_id, name, description, state, created_at, updated_at
		FROM sirkel_engine.projects
		WHERE 1 = 1
	`
	var args []any

	if params != nil && params.OrganizationID != nil {
		args = append(args, *params.OrganizationID)
		query += fmt.Sprintf(" AND organization_id = $%d", len(args))
	}

	query += " ORDER BY created_at DESC"

	if params != nil && params.Limit != nil {
		args = append(args, *params.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	if params != nil && params.Offset != nil {
		args = append(args, *params.Offset)
		query += fmt.Sprintf(" OFFSET $%d", len(args))
	}

	rows, err := h.pool.Query(h.ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []*sirkel_domain.Project
	for rows.Next() {
		project := &sirkel_domain.Project{}
		if err := rows.Scan(
			&project.ID,
			&project.OrganizationID,
			&project.Name,
			&project.Description,
			&project.State,
			&project.CreatedAt,
			&project.UpdatedAt,
		); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}

	return projects, rows.Err()
}

func (h *ProjectsRepositoryHandler) CountProjects(params *CountProjectParams) (int, error) {
	query := `SELECT count(*) FROM sirkel_engine.projects WHERE 1 = 1`
	var args []any

	if params != nil && params.OrganizationID != nil {
		args = append(args, *params.OrganizationID)
		query += fmt.Sprintf(" AND organization_id = $%d", len(args))
	}

	var count int
	err := h.pool.QueryRow(h.ctx, query, args...).Scan(&count)
	return count, err
}

func (h *ProjectsRepositoryHandler) DeleteProject(projectID *string) error {
	_, err := h.pool.Exec(h.ctx, `DELETE FROM sirkel_engine.projects WHERE id = $1`, *projectID)
	return err
}

func (h *ProjectsRepositoryHandler) GetProjectByID(projectID *string) (*sirkel_domain.Project, error) {
	project := &sirkel_domain.Project{}
	err := h.pool.QueryRow(h.ctx, `
		SELECT id, organization_id, name, description, state, created_at, updated_at
		FROM sirkel_engine.projects
		WHERE id = $1
	`, *projectID).Scan(
		&project.ID,
		&project.OrganizationID,
		&project.Name,
		&project.Description,
		&project.State,
		&project.CreatedAt,
		&project.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return project, nil
}
