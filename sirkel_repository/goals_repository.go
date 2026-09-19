package sirkel_repository

import (
	"context"
	"errors"
	"fmt"

	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GetGoalsParams struct {
	ProjectID *string
	Offset    *int
	Limit     *int
}

type CountGoalsParams struct {
	ProjectID *string
}

type GoalsRepository interface {
	SaveGoal(goal *sirkel_domain.Goal) error
	GetGoals(params *GetGoalsParams) ([]*sirkel_domain.Goal, error)
	CountGoals(params *CountGoalsParams) (int, error)
	DeleteGoal(goalID *string) error
	GetGoalByID(goalID *string) (*sirkel_domain.Goal, error)
}

type GoalsRepositoryHandler struct {
	ctx  context.Context
	pool *pgxpool.Pool
	user *sirkel_domain.User
}

func NewGoalsRepositoryHandler(ctx context.Context, pool *pgxpool.Pool, user *sirkel_domain.User) *GoalsRepositoryHandler {
	return &GoalsRepositoryHandler{
		ctx:  ctx,
		pool: pool,
		user: user,
	}
}

func (h *GoalsRepositoryHandler) SaveGoal(goal *sirkel_domain.Goal) error {
	return h.pool.QueryRow(h.ctx, `
		INSERT INTO sirkel_engine.goals AS g (id, project_id, name, description, state, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name,
			description = EXCLUDED.description,
			state = EXCLUDED.state,
			updated_by = COALESCE(EXCLUDED.updated_by, g.updated_by),
			updated_at = now()
		RETURNING created_by, updated_by, created_at, updated_at
	`, goal.ID, goal.ProjectID, goal.Name, goal.Description, goal.State, actorID(h.user),
	).Scan(&goal.CreatedBy, &goal.UpdatedBy, &goal.CreatedAt, &goal.UpdatedAt)
}

func (h *GoalsRepositoryHandler) GetGoals(params *GetGoalsParams) ([]*sirkel_domain.Goal, error) {
	query := `
		SELECT id, project_id, name, description, state, created_by, updated_by, created_at, updated_at
		FROM sirkel_engine.goals
		WHERE 1 = 1
	`
	var args []any

	if params != nil && params.ProjectID != nil {
		args = append(args, *params.ProjectID)
		query += fmt.Sprintf(" AND project_id = $%d", len(args))
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

	var goals []*sirkel_domain.Goal
	for rows.Next() {
		goal := &sirkel_domain.Goal{}
		if err := rows.Scan(
			&goal.ID,
			&goal.ProjectID,
			&goal.Name,
			&goal.Description,
			&goal.State,
			&goal.CreatedBy,
			&goal.UpdatedBy,
			&goal.CreatedAt,
			&goal.UpdatedAt,
		); err != nil {
			return nil, err
		}
		goals = append(goals, goal)
	}

	return goals, rows.Err()
}

func (h *GoalsRepositoryHandler) CountGoals(params *CountGoalsParams) (int, error) {
	query := `SELECT count(*) FROM sirkel_engine.goals WHERE 1 = 1`
	var args []any

	if params != nil && params.ProjectID != nil {
		args = append(args, *params.ProjectID)
		query += fmt.Sprintf(" AND project_id = $%d", len(args))
	}

	var count int
	err := h.pool.QueryRow(h.ctx, query, args...).Scan(&count)
	return count, err
}

func (h *GoalsRepositoryHandler) DeleteGoal(goalID *string) error {
	_, err := h.pool.Exec(h.ctx, `DELETE FROM sirkel_engine.goals WHERE id = $1`, *goalID)
	return err
}

func (h *GoalsRepositoryHandler) GetGoalByID(goalID *string) (*sirkel_domain.Goal, error) {
	goal := &sirkel_domain.Goal{}
	err := h.pool.QueryRow(h.ctx, `
		SELECT id, project_id, name, description, state, created_by, updated_by, created_at, updated_at
		FROM sirkel_engine.goals
		WHERE id = $1
	`, *goalID).Scan(
		&goal.ID,
		&goal.ProjectID,
		&goal.Name,
		&goal.Description,
		&goal.State,
		&goal.CreatedBy,
		&goal.UpdatedBy,
		&goal.CreatedAt,
		&goal.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return goal, nil
}
