package sirkel_repository

import (
	"context"
	"errors"
	"fmt"

	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GetTaskItemsParams struct {
	TaskID *string
	Offset *int
	Limit  *int
}

type CountTaskItemsParams struct {
	TaskID *string
}

type TaskItemsRepository interface {
	SaveTaskItem(taskItem *sirkel_domain.TaskItem) error
	GetTaskItems(params *GetTaskItemsParams) ([]*sirkel_domain.TaskItem, error)
	CountTaskItems(params *CountTaskItemsParams) (int, error)
	DeleteTaskItem(taskItemID *string) error
	GetTaskItemByID(taskItemID *string) (*sirkel_domain.TaskItem, error)
}

type TaskItemsRepositoryHandler struct {
	ctx  context.Context
	pool *pgxpool.Pool
	user *sirkel_domain.User
}

func NewTaskItemsRepositoryHandler(ctx context.Context, pool *pgxpool.Pool, user *sirkel_domain.User) *TaskItemsRepositoryHandler {
	return &TaskItemsRepositoryHandler{
		ctx:  ctx,
		pool: pool,
		user: user,
	}
}

func (h *TaskItemsRepositoryHandler) SaveTaskItem(taskItem *sirkel_domain.TaskItem) error {
	if taskItem.AssignedUserID != nil {
		if err := userBelongsToTaskOrganization(h.ctx, h.pool, *taskItem.AssignedUserID, taskItem.TaskID); err != nil {
			return err
		}
	}

	return h.pool.QueryRow(h.ctx, `
		INSERT INTO sirkel_engine.task_items AS ti (id, task_id, name, description, state, assigned_user_id, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name,
			description = EXCLUDED.description,
			state = EXCLUDED.state,
			assigned_user_id = EXCLUDED.assigned_user_id,
			updated_by = COALESCE(EXCLUDED.updated_by, ti.updated_by),
			updated_at = now()
		RETURNING created_by, updated_by, created_at, updated_at
	`, taskItem.ID, taskItem.TaskID, taskItem.Name, taskItem.Description, taskItem.State, taskItem.AssignedUserID, actorID(h.user),
	).Scan(&taskItem.CreatedBy, &taskItem.UpdatedBy, &taskItem.CreatedAt, &taskItem.UpdatedAt)
}

func (h *TaskItemsRepositoryHandler) GetTaskItems(params *GetTaskItemsParams) ([]*sirkel_domain.TaskItem, error) {
	query := `
		SELECT id, task_id, name, description, state, assigned_user_id, created_by, updated_by, created_at, updated_at
		FROM sirkel_engine.task_items
		WHERE 1 = 1
	`
	var args []any

	if params != nil && params.TaskID != nil {
		args = append(args, *params.TaskID)
		query += fmt.Sprintf(" AND task_id = $%d", len(args))
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

	var taskItems []*sirkel_domain.TaskItem
	for rows.Next() {
		taskItem := &sirkel_domain.TaskItem{}
		if err := rows.Scan(
			&taskItem.ID,
			&taskItem.TaskID,
			&taskItem.Name,
			&taskItem.Description,
			&taskItem.State,
			&taskItem.AssignedUserID,
			&taskItem.CreatedBy,
			&taskItem.UpdatedBy,
			&taskItem.CreatedAt,
			&taskItem.UpdatedAt,
		); err != nil {
			return nil, err
		}
		taskItems = append(taskItems, taskItem)
	}

	return taskItems, rows.Err()
}

func (h *TaskItemsRepositoryHandler) CountTaskItems(params *CountTaskItemsParams) (int, error) {
	query := `SELECT count(*) FROM sirkel_engine.task_items WHERE 1 = 1`
	var args []any

	if params != nil && params.TaskID != nil {
		args = append(args, *params.TaskID)
		query += fmt.Sprintf(" AND task_id = $%d", len(args))
	}

	var count int
	err := h.pool.QueryRow(h.ctx, query, args...).Scan(&count)
	return count, err
}

func (h *TaskItemsRepositoryHandler) DeleteTaskItem(taskItemID *string) error {
	_, err := h.pool.Exec(h.ctx, `DELETE FROM sirkel_engine.task_items WHERE id = $1`, *taskItemID)
	return err
}

func (h *TaskItemsRepositoryHandler) GetTaskItemByID(taskItemID *string) (*sirkel_domain.TaskItem, error) {
	taskItem := &sirkel_domain.TaskItem{}
	err := h.pool.QueryRow(h.ctx, `
		SELECT id, task_id, name, description, state, assigned_user_id, created_by, updated_by, created_at, updated_at
		FROM sirkel_engine.task_items
		WHERE id = $1
	`, *taskItemID).Scan(
		&taskItem.ID,
		&taskItem.TaskID,
		&taskItem.Name,
		&taskItem.Description,
		&taskItem.State,
		&taskItem.AssignedUserID,
		&taskItem.CreatedBy,
		&taskItem.UpdatedBy,
		&taskItem.CreatedAt,
		&taskItem.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return taskItem, nil
}
