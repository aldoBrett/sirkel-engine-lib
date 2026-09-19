package sirkel_repository

import (
	"context"
	"errors"
	"fmt"
	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GetTasksParams struct {
	GoalID *string
	Offset *int
	Limit  *int
}

type CountTasksParams struct {
	GoalID *string
}

type TasksRepository interface {
	SaveTask(task *sirkel_domain.Task) error
	GetTasks(params *GetTasksParams) ([]*sirkel_domain.Task, error)
	GetTasksForIndex(params *GetTasksParams) ([]*sirkel_domain.TaskForIndex, error)
	CountTasks(params *CountTasksParams) (int, error)
	DeleteTask(taskID *string) error
	GetTaskByID(taskID *string) (*sirkel_domain.Task, error)
}

var taskItemStatesInOrder = []sirkel_domain.TaskItemState{
	sirkel_domain.TaskItemStatePending,
	sirkel_domain.TaskItemStateInProgress,
	sirkel_domain.TaskItemStateDone,
	sirkel_domain.TaskItemStateCancelled,
}

type TasksRepositoryHandler struct {
	ctx  context.Context
	pool *pgxpool.Pool
	user *sirkel_domain.User
}

func NewTasksRepositoryHandler(ctx context.Context, pool *pgxpool.Pool, user *sirkel_domain.User) *TasksRepositoryHandler {
	return &TasksRepositoryHandler{
		ctx:  ctx,
		pool: pool,
		user: user,
	}
}

func (h *TasksRepositoryHandler) SaveTask(task *sirkel_domain.Task) error {
	return h.pool.QueryRow(h.ctx, `
		INSERT INTO sirkel_engine.tasks (id, goal_id, name, description, state)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name,
			description = EXCLUDED.description,
			state = EXCLUDED.state,
			updated_at = now()
		RETURNING created_at, updated_at
	`, task.ID, task.GoalID, task.Name, task.Description, task.State,
	).Scan(&task.CreatedAt, &task.UpdatedAt)
}

func (h *TasksRepositoryHandler) GetTasks(params *GetTasksParams) ([]*sirkel_domain.Task, error) {
	query := `
		SELECT id, goal_id, name, description, state, created_at, updated_at
		FROM sirkel_engine.tasks
		WHERE 1 = 1
	`
	var args []any

	if params != nil && params.GoalID != nil {
		args = append(args, *params.GoalID)
		query += fmt.Sprintf(" AND goal_id = $%d", len(args))
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

	var tasks []*sirkel_domain.Task
	for rows.Next() {
		task := &sirkel_domain.Task{}
		if err := rows.Scan(
			&task.ID,
			&task.GoalID,
			&task.Name,
			&task.Description,
			&task.State,
			&task.CreatedAt,
			&task.UpdatedAt,
		); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, rows.Err()
}

func (h *TasksRepositoryHandler) GetTasksForIndex(params *GetTasksParams) ([]*sirkel_domain.TaskForIndex, error) {
	tasks, err := h.GetTasks(params)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, nil
	}

	taskIDs := make([]string, len(tasks))
	countsByTask := make(map[string]map[sirkel_domain.TaskItemState]int, len(tasks))
	for i, task := range tasks {
		taskIDs[i] = task.ID
		countsByTask[task.ID] = make(map[sirkel_domain.TaskItemState]int, len(taskItemStatesInOrder))
	}

	rows, err := h.pool.Query(h.ctx, `
		SELECT task_id, state, count(*)
		FROM sirkel_engine.task_items
		WHERE task_id = ANY($1)
		GROUP BY task_id, state
	`, taskIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var taskID string
		var state sirkel_domain.TaskItemState
		var count int
		if err := rows.Scan(&taskID, &state, &count); err != nil {
			return nil, err
		}
		countsByTask[taskID][state] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	tasksForIndex := make([]*sirkel_domain.TaskForIndex, len(tasks))
	for i, task := range tasks {
		stateCounts := make([]sirkel_domain.TaskItemStateCount, len(taskItemStatesInOrder))
		for j, state := range taskItemStatesInOrder {
			stateCounts[j] = sirkel_domain.TaskItemStateCount{State: state, Count: countsByTask[task.ID][state]}
		}
		tasksForIndex[i] = &sirkel_domain.TaskForIndex{Task: *task, TaskItemStateCounts: stateCounts}
	}

	return tasksForIndex, nil
}

func (h *TasksRepositoryHandler) CountTasks(params *CountTasksParams) (int, error) {
	query := `SELECT count(*) FROM sirkel_engine.tasks WHERE 1 = 1`
	var args []any

	if params != nil && params.GoalID != nil {
		args = append(args, *params.GoalID)
		query += fmt.Sprintf(" AND goal_id = $%d", len(args))
	}

	var count int
	err := h.pool.QueryRow(h.ctx, query, args...).Scan(&count)
	return count, err
}

func (h *TasksRepositoryHandler) DeleteTask(taskID *string) error {
	_, err := h.pool.Exec(h.ctx, `DELETE FROM sirkel_engine.tasks WHERE id = $1`, *taskID)
	return err
}

func (h *TasksRepositoryHandler) GetTaskByID(taskID *string) (*sirkel_domain.Task, error) {
	task := &sirkel_domain.Task{}
	err := h.pool.QueryRow(h.ctx, `
		SELECT id, goal_id, name, description, state, created_at, updated_at
		FROM sirkel_engine.tasks
		WHERE id = $1
	`, *taskID).Scan(
		&task.ID,
		&task.GoalID,
		&task.Name,
		&task.Description,
		&task.State,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return task, nil
}
