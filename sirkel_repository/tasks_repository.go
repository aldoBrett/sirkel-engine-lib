package sirkel_repository

import (
	"context"
	"errors"
	"fmt"
	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GetTasksParams filters and pages the tasks, which come back in their manual order.
// Page with Offset, or with After for infinite scroll: After is the Cursor of the last task already loaded, and
// stays correct when tasks are created or moved between requests. Fewer tasks than Limit means the last page.
// Offset and After cannot be used together.
type GetTasksParams struct {
	GoalID *string
	State  *sirkel_domain.TaskState
	After  *sirkel_domain.Cursor
	Offset *int
	Limit  *int
}

type CountTasksParams struct {
	GoalID *string
	State  *sirkel_domain.TaskState
}

// MoveTaskParams places a task in its goal's order. AfterID is the task it should sit right below and BeforeID the
// one it should sit right above. With only one of them, the task goes right next to it. With none, the position stays,
// so State alone moves a task to another kanban column, or into an empty one.
type MoveTaskParams struct {
	TaskID   string
	AfterID  *string
	BeforeID *string
	State    *sirkel_domain.TaskState
}

type TasksRepository interface {
	// SaveTask creates the task at the top of its goal's order, or updates it in place. It never changes the position.
	SaveTask(task *sirkel_domain.Task) error
	// MoveTask repositions a task, and optionally changes its state in the same transaction. Only a state change is
	// recorded in the task history.
	MoveTask(params *MoveTaskParams) (*sirkel_domain.Task, error)
	GetTasks(params *GetTasksParams) ([]*sirkel_domain.Task, error)
	GetTasksForIndex(params *GetTasksParams) ([]*sirkel_domain.TaskForIndex, error)
	CountTasks(params *CountTasksParams) (int, error)
	DeleteTask(taskID *string) error
	GetTaskByID(taskID *string) (*sirkel_domain.Task, error)
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
	if task.ResponsibleUserID != nil {
		if err := userBelongsToGoalOrganization(h.ctx, h.pool, *task.ResponsibleUserID, task.GoalID); err != nil {
			return err
		}
	}

	tx, err := h.pool.Begin(h.ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(h.ctx)

	previous, err := lockTask(h.ctx, tx, task.ID)
	if err != nil {
		return err
	}

	// A new task goes to the top of its goal. An existing one keeps its position.
	var sortKey string
	if previous != nil {
		sortKey = previous.SortKey
	} else {
		if err := lockSortScope(h.ctx, tx, task.GoalID); err != nil {
			return err
		}
		if sortKey, err = taskSortScope.topKey(h.ctx, tx, task.GoalID); err != nil {
			return err
		}
	}

	err = tx.QueryRow(h.ctx, `
		INSERT INTO sirkel_engine.tasks AS t (id, goal_id, name, description, state, sort_key, responsible_user_id, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name,
			description = EXCLUDED.description,
			state = EXCLUDED.state,
			responsible_user_id = EXCLUDED.responsible_user_id,
			updated_by = COALESCE(EXCLUDED.updated_by, t.updated_by),
			updated_at = now()
		RETURNING sort_key, created_by, updated_by, created_at, updated_at
	`, task.ID, task.GoalID, task.Name, task.Description, task.State, sortKey, task.ResponsibleUserID, actorID(h.user),
	).Scan(&task.SortKey, &task.CreatedBy, &task.UpdatedBy, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return err
	}

	if err := insertTaskEvents(h.ctx, tx, task.ID, nil, actorID(h.user), taskChanges(previous, task)); err != nil {
		return err
	}

	return tx.Commit(h.ctx)
}

// lockTask returns the stored task locked for the rest of the transaction, or nil if it doesn't exist yet.
func lockTask(ctx context.Context, tx pgx.Tx, taskID string) (*sirkel_domain.Task, error) {
	previous := &sirkel_domain.Task{}
	err := tx.QueryRow(ctx, `
		SELECT goal_id, name, description, state, sort_key, responsible_user_id
		FROM sirkel_engine.tasks
		WHERE id = $1
		FOR UPDATE
	`, taskID).Scan(&previous.GoalID, &previous.Name, &previous.Description, &previous.State, &previous.SortKey, &previous.ResponsibleUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return previous, nil
}

func (h *TasksRepositoryHandler) MoveTask(params *MoveTaskParams) (*sirkel_domain.Task, error) {
	if params.AfterID == nil && params.BeforeID == nil && params.State == nil {
		return nil, ErrMoveTargetRequired
	}

	tx, err := h.pool.Begin(h.ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(h.ctx)

	// A task never changes goal, so it is safe to read it before taking the locks.
	var goalID string
	err = tx.QueryRow(h.ctx, `SELECT goal_id FROM sirkel_engine.tasks WHERE id = $1`, params.TaskID).Scan(&goalID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if err := lockSortScope(h.ctx, tx, goalID); err != nil {
		return nil, err
	}
	previous, err := lockTask(h.ctx, tx, params.TaskID)
	if err != nil {
		return nil, err
	}
	if previous == nil {
		return nil, ErrNotFound
	}

	sortKey := previous.SortKey
	if params.AfterID != nil || params.BeforeID != nil {
		if sortKey, err = taskSortScope.positionKey(h.ctx, tx, goalID, params.TaskID, params.AfterID, params.BeforeID); err != nil {
			return nil, err
		}
	}

	state := previous.State
	if params.State != nil {
		state = *params.State
	}
	stateChanged := state != previous.State

	task, err := scanTask(tx.QueryRow(h.ctx, `
		UPDATE sirkel_engine.tasks
		SET sort_key = $2,
			state = $3,
			updated_by = CASE WHEN $4 THEN COALESCE($5, updated_by) ELSE updated_by END,
			updated_at = CASE WHEN $4 THEN now() ELSE updated_at END
		WHERE id = $1
		RETURNING `+taskColumns, params.TaskID, sortKey, state, stateChanged, actorID(h.user)))
	if err != nil {
		return nil, err
	}

	if stateChanged {
		changes := changedFields(fieldChange{sirkel_domain.TaskEventFieldState, strPtr(string(previous.State)), strPtr(string(state))})
		if err := insertTaskEvents(h.ctx, tx, task.ID, nil, actorID(h.user), changes); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(h.ctx); err != nil {
		return nil, err
	}

	return task, nil
}

const taskColumns = `id, goal_id, name, description, state, sort_key, responsible_user_id, created_by, updated_by, created_at, updated_at`

// scanTask reads a row selected with taskColumns.
func scanTask(row pgx.Row) (*sirkel_domain.Task, error) {
	task := &sirkel_domain.Task{}
	err := row.Scan(
		&task.ID,
		&task.GoalID,
		&task.Name,
		&task.Description,
		&task.State,
		&task.SortKey,
		&task.ResponsibleUserID,
		&task.CreatedBy,
		&task.UpdatedBy,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return task, nil
}

// taskChanges lists the tracked fields that differ between the stored task (nil on insert) and the one being saved.
// An insert records the initial state, and the responsible user when there is one.
func taskChanges(previous, task *sirkel_domain.Task) []fieldChange {
	if previous == nil {
		return changedFields(
			fieldChange{sirkel_domain.TaskEventFieldState, nil, strPtr(string(task.State))},
			fieldChange{sirkel_domain.TaskEventFieldResponsibleUserID, nil, task.ResponsibleUserID},
		)
	}

	return changedFields(
		fieldChange{sirkel_domain.TaskEventFieldName, strPtr(previous.Name), strPtr(task.Name)},
		fieldChange{sirkel_domain.TaskEventFieldDescription, previous.Description, task.Description},
		fieldChange{sirkel_domain.TaskEventFieldState, strPtr(string(previous.State)), strPtr(string(task.State))},
		fieldChange{sirkel_domain.TaskEventFieldResponsibleUserID, previous.ResponsibleUserID, task.ResponsibleUserID},
	)
}

func (h *TasksRepositoryHandler) GetTasks(params *GetTasksParams) ([]*sirkel_domain.Task, error) {
	if params != nil && params.Offset != nil && params.After != nil {
		return nil, ErrConflictingPagination
	}

	query := `
		SELECT ` + taskColumns + `
		FROM sirkel_engine.tasks
		WHERE 1 = 1
	`
	var args []any

	if params != nil && params.GoalID != nil {
		args = append(args, *params.GoalID)
		query += fmt.Sprintf(" AND goal_id = $%d", len(args))
	}

	if params != nil && params.State != nil {
		args = append(args, *params.State)
		query += fmt.Sprintf(" AND state = $%d", len(args))
	}

	if params != nil && params.After != nil {
		args = append(args, params.After.SortKey, params.After.ID)
		query += fmt.Sprintf(" AND (sort_key, id) > ($%d, $%d)", len(args)-1, len(args))
	}

	query += " ORDER BY sort_key, id"

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
		task, err := scanTask(rows)
		if err != nil {
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
		countsByTask[task.ID] = make(map[sirkel_domain.TaskItemState]int, len(sirkel_domain.TaskItemStates))
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
		stateCounts := make([]sirkel_domain.TaskItemStateCount, len(sirkel_domain.TaskItemStates))
		for j, state := range sirkel_domain.TaskItemStates {
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

	if params != nil && params.State != nil {
		args = append(args, *params.State)
		query += fmt.Sprintf(" AND state = $%d", len(args))
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
	task, err := scanTask(h.pool.QueryRow(h.ctx, `
		SELECT `+taskColumns+`
		FROM sirkel_engine.tasks
		WHERE id = $1
	`, *taskID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return task, nil
}
