package sirkel_repository

import (
	"context"
	"errors"
	"fmt"

	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GetTaskItemsParams filters and pages the task items, which come back in their manual order.
// Page with Offset, or with After for infinite scroll: After is the Cursor of the last task item already loaded, and
// stays correct when task items are created or moved between requests. Fewer task items than Limit means the last page.
// Offset and After cannot be used together.
type GetTaskItemsParams struct {
	TaskID *string
	State  *sirkel_domain.TaskItemState
	After  *sirkel_domain.Cursor
	Offset *int
	Limit  *int
}

type CountTaskItemsParams struct {
	TaskID *string
	State  *sirkel_domain.TaskItemState
}

// MoveTaskItemParams places a task item in its task's order. AfterID is the task item it should sit right below and
// BeforeID the one it should sit right above. With only one of them, the task item goes right next to it. With none,
// the position stays, so State alone moves a task item to another kanban column, or into an empty one.
type MoveTaskItemParams struct {
	TaskItemID string
	AfterID    *string
	BeforeID   *string
	State      *sirkel_domain.TaskItemState
}

type TaskItemsRepository interface {
	// SaveTaskItem creates the task item at the top of its task's order, or updates it in place. It never changes the position.
	SaveTaskItem(taskItem *sirkel_domain.TaskItem) error
	// MoveTaskItem repositions a task item, and optionally changes its state in the same transaction. Only a state
	// change is recorded in the task history.
	MoveTaskItem(params *MoveTaskItemParams) (*sirkel_domain.TaskItem, error)
	GetTaskItems(params *GetTaskItemsParams) ([]*sirkel_domain.TaskItem, error)
	// GetTaskItemsWithUsers is GetTaskItems with the creator and assignee resolved from sirkel_engine.users.
	GetTaskItemsWithUsers(params *GetTaskItemsParams) ([]*sirkel_domain.TaskItemWithUsers, error)
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

	tx, err := h.pool.Begin(h.ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(h.ctx)

	previous, err := lockTaskItem(h.ctx, tx, taskItem.ID)
	if err != nil {
		return err
	}

	// A new task item goes to the top of its task. An existing one keeps its position.
	var sortKey string
	if previous != nil {
		sortKey = previous.SortKey
	} else {
		if err := lockSortScope(h.ctx, tx, taskItem.TaskID); err != nil {
			return err
		}
		if sortKey, err = taskItemSortScope.topKey(h.ctx, tx, taskItem.TaskID); err != nil {
			return err
		}
	}

	err = tx.QueryRow(h.ctx, `
		INSERT INTO sirkel_engine.task_items AS ti (id, task_id, name, description, state, sort_key, assigned_user_id, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name,
			description = EXCLUDED.description,
			state = EXCLUDED.state,
			assigned_user_id = EXCLUDED.assigned_user_id,
			updated_by = COALESCE(EXCLUDED.updated_by, ti.updated_by),
			updated_at = now()
		RETURNING sort_key, created_by, updated_by, created_at, updated_at
	`, taskItem.ID, taskItem.TaskID, taskItem.Name, taskItem.Description, taskItem.State, sortKey, taskItem.AssignedUserID, actorID(h.user),
	).Scan(&taskItem.SortKey, &taskItem.CreatedBy, &taskItem.UpdatedBy, &taskItem.CreatedAt, &taskItem.UpdatedAt)
	if err != nil {
		return err
	}

	// An existing item stays with its stored task, whatever TaskID the caller sent.
	taskID := taskItem.TaskID
	if previous != nil {
		taskID = previous.TaskID
	}
	if err := insertTaskEvents(h.ctx, tx, taskID, &taskItem.ID, actorID(h.user), taskItemChanges(previous, taskItem)); err != nil {
		return err
	}

	return tx.Commit(h.ctx)
}

// lockTaskItem returns the stored task item locked for the rest of the transaction, or nil if it doesn't exist yet.
func lockTaskItem(ctx context.Context, tx pgx.Tx, taskItemID string) (*sirkel_domain.TaskItem, error) {
	previous := &sirkel_domain.TaskItem{}
	err := tx.QueryRow(ctx, `
		SELECT task_id, name, description, state, sort_key, assigned_user_id
		FROM sirkel_engine.task_items
		WHERE id = $1
		FOR UPDATE
	`, taskItemID).Scan(&previous.TaskID, &previous.Name, &previous.Description, &previous.State, &previous.SortKey, &previous.AssignedUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return previous, nil
}

func (h *TaskItemsRepositoryHandler) MoveTaskItem(params *MoveTaskItemParams) (*sirkel_domain.TaskItem, error) {
	if params.AfterID == nil && params.BeforeID == nil && params.State == nil {
		return nil, ErrMoveTargetRequired
	}

	tx, err := h.pool.Begin(h.ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(h.ctx)

	// A task item never changes task, so it is safe to read it before taking the locks.
	var taskID string
	err = tx.QueryRow(h.ctx, `SELECT task_id FROM sirkel_engine.task_items WHERE id = $1`, params.TaskItemID).Scan(&taskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if err := lockSortScope(h.ctx, tx, taskID); err != nil {
		return nil, err
	}
	previous, err := lockTaskItem(h.ctx, tx, params.TaskItemID)
	if err != nil {
		return nil, err
	}
	if previous == nil {
		return nil, ErrNotFound
	}

	sortKey := previous.SortKey
	if params.AfterID != nil || params.BeforeID != nil {
		if sortKey, err = taskItemSortScope.positionKey(h.ctx, tx, taskID, params.TaskItemID, params.AfterID, params.BeforeID); err != nil {
			return nil, err
		}
	}

	state := previous.State
	if params.State != nil {
		state = *params.State
	}
	stateChanged := state != previous.State

	taskItem, err := scanTaskItem(tx.QueryRow(h.ctx, `
		UPDATE sirkel_engine.task_items
		SET sort_key = $2,
			state = $3,
			updated_by = CASE WHEN $4 THEN COALESCE($5, updated_by) ELSE updated_by END,
			updated_at = CASE WHEN $4 THEN now() ELSE updated_at END
		WHERE id = $1
		RETURNING `+taskItemColumns, params.TaskItemID, sortKey, state, stateChanged, actorID(h.user)))
	if err != nil {
		return nil, err
	}

	if stateChanged {
		changes := changedFields(fieldChange{sirkel_domain.TaskEventFieldState, strPtr(string(previous.State)), strPtr(string(state))})
		if err := insertTaskEvents(h.ctx, tx, taskID, &taskItem.ID, actorID(h.user), changes); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(h.ctx); err != nil {
		return nil, err
	}

	return taskItem, nil
}

const taskItemColumns = `id, task_id, name, description, state, sort_key, assigned_user_id, created_by, updated_by, created_at, updated_at`

// scanTaskItem reads a row selected with taskItemColumns.
func scanTaskItem(row pgx.Row) (*sirkel_domain.TaskItem, error) {
	taskItem := &sirkel_domain.TaskItem{}
	err := row.Scan(
		&taskItem.ID,
		&taskItem.TaskID,
		&taskItem.Name,
		&taskItem.Description,
		&taskItem.State,
		&taskItem.SortKey,
		&taskItem.AssignedUserID,
		&taskItem.CreatedBy,
		&taskItem.UpdatedBy,
		&taskItem.CreatedAt,
		&taskItem.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return taskItem, nil
}

// taskItemChanges lists the tracked fields that differ between the stored task item (nil on insert) and the one being saved.
// An insert records the initial state, and the assigned user when there is one.
func taskItemChanges(previous, taskItem *sirkel_domain.TaskItem) []fieldChange {
	if previous == nil {
		return changedFields(
			fieldChange{sirkel_domain.TaskEventFieldState, nil, strPtr(string(taskItem.State))},
			fieldChange{sirkel_domain.TaskEventFieldAssignedUserID, nil, taskItem.AssignedUserID},
		)
	}

	return changedFields(
		fieldChange{sirkel_domain.TaskEventFieldName, strPtr(previous.Name), strPtr(taskItem.Name)},
		fieldChange{sirkel_domain.TaskEventFieldDescription, previous.Description, taskItem.Description},
		fieldChange{sirkel_domain.TaskEventFieldState, strPtr(string(previous.State)), strPtr(string(taskItem.State))},
		fieldChange{sirkel_domain.TaskEventFieldAssignedUserID, previous.AssignedUserID, taskItem.AssignedUserID},
	)
}

// taskItemFilters appends the WHERE/ORDER/LIMIT/OFFSET clauses shared by GetTaskItems and GetTaskItemsWithUsers.
// tableAlias prefixes the filtered/ordered columns (task_id, state, sort_key, id), and is empty when the query
// selects straight from an unaliased task_items.
func taskItemFilters(query, tableAlias string, params *GetTaskItemsParams) (string, []any, error) {
	if params != nil && params.Offset != nil && params.After != nil {
		return "", nil, ErrConflictingPagination
	}

	col := func(name string) string {
		if tableAlias == "" {
			return name
		}
		return tableAlias + "." + name
	}

	var args []any

	if params != nil && params.TaskID != nil {
		args = append(args, *params.TaskID)
		query += fmt.Sprintf(" AND %s = $%d", col("task_id"), len(args))
	}

	if params != nil && params.State != nil {
		args = append(args, *params.State)
		query += fmt.Sprintf(" AND %s = $%d", col("state"), len(args))
	}

	if params != nil && params.After != nil {
		args = append(args, params.After.SortKey, params.After.ID)
		query += fmt.Sprintf(" AND (%s, %s) > ($%d, $%d)", col("sort_key"), col("id"), len(args)-1, len(args))
	}

	query += fmt.Sprintf(" ORDER BY %s, %s", col("sort_key"), col("id"))

	if params != nil && params.Limit != nil {
		args = append(args, *params.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	if params != nil && params.Offset != nil {
		args = append(args, *params.Offset)
		query += fmt.Sprintf(" OFFSET $%d", len(args))
	}

	return query, args, nil
}

func (h *TaskItemsRepositoryHandler) GetTaskItems(params *GetTaskItemsParams) ([]*sirkel_domain.TaskItem, error) {
	query, args, err := taskItemFilters(`
		SELECT `+taskItemColumns+`
		FROM sirkel_engine.task_items
		WHERE 1 = 1
	`, "", params)
	if err != nil {
		return nil, err
	}

	rows, err := h.pool.Query(h.ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var taskItems []*sirkel_domain.TaskItem
	for rows.Next() {
		taskItem, err := scanTaskItem(rows)
		if err != nil {
			return nil, err
		}
		taskItems = append(taskItems, taskItem)
	}

	return taskItems, rows.Err()
}

// taskItemWithUsersSelect resolves each task item's creator and assignee from sirkel_engine.users. Either join
// misses when the task item has no such user recorded, or the recorded user no longer exists.
const taskItemWithUsersSelect = `
	SELECT
		ti.id, ti.task_id, ti.name, ti.description, ti.state, ti.sort_key, ti.assigned_user_id, ti.created_by, ti.updated_by, ti.created_at, ti.updated_at,
		creator.id, creator.organization_id, creator.email, creator.role, creator.name, creator.first_surname, creator.second_surname, creator.phone,
		assignee.id, assignee.organization_id, assignee.email, assignee.role, assignee.name, assignee.first_surname, assignee.second_surname, assignee.phone
	FROM sirkel_engine.task_items ti
	LEFT JOIN sirkel_engine.users creator ON creator.id = ti.created_by
	LEFT JOIN sirkel_engine.users assignee ON assignee.id = ti.assigned_user_id
	WHERE 1 = 1
`

// nullableUser scans a possibly-missing joined user row before it is converted to a *UserComplete.
type nullableUser struct {
	ID             *string
	OrganizationID *string
	Email          *string
	Role           *string
	Name           *string
	FirstSurname   *string
	SecondSurname  *string
	Phone          *string
}

func (u nullableUser) toDomain() *sirkel_domain.UserComplete {
	if u.ID == nil {
		return nil
	}

	user := &sirkel_domain.UserComplete{ID: *u.ID, Phone: u.Phone}
	if u.OrganizationID != nil {
		user.OrganizationID = *u.OrganizationID
	}
	if u.Email != nil {
		user.Email = *u.Email
	}
	if u.Role != nil {
		user.Role = *u.Role
	}
	if u.Name != nil {
		user.Name = *u.Name
	}
	if u.FirstSurname != nil {
		user.FirstSurname = *u.FirstSurname
	}
	if u.SecondSurname != nil {
		user.SecondSurname = *u.SecondSurname
	}

	return user
}

// scanTaskItemWithUsers reads a row selected with taskItemWithUsersSelect.
func scanTaskItemWithUsers(row pgx.Row) (*sirkel_domain.TaskItemWithUsers, error) {
	taskItem := &sirkel_domain.TaskItemWithUsers{}
	var creator, assignee nullableUser

	err := row.Scan(
		&taskItem.ID,
		&taskItem.TaskID,
		&taskItem.Name,
		&taskItem.Description,
		&taskItem.State,
		&taskItem.SortKey,
		&taskItem.AssignedUserID,
		&taskItem.CreatedBy,
		&taskItem.UpdatedBy,
		&taskItem.CreatedAt,
		&taskItem.UpdatedAt,
		&creator.ID, &creator.OrganizationID, &creator.Email, &creator.Role, &creator.Name, &creator.FirstSurname, &creator.SecondSurname, &creator.Phone,
		&assignee.ID, &assignee.OrganizationID, &assignee.Email, &assignee.Role, &assignee.Name, &assignee.FirstSurname, &assignee.SecondSurname, &assignee.Phone,
	)
	if err != nil {
		return nil, err
	}

	taskItem.CreatedByUser = creator.toDomain()
	taskItem.AssignedUser = assignee.toDomain()

	return taskItem, nil
}

func (h *TaskItemsRepositoryHandler) GetTaskItemsWithUsers(params *GetTaskItemsParams) ([]*sirkel_domain.TaskItemWithUsers, error) {
	query, args, err := taskItemFilters(taskItemWithUsersSelect, "ti", params)
	if err != nil {
		return nil, err
	}

	rows, err := h.pool.Query(h.ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var taskItems []*sirkel_domain.TaskItemWithUsers
	for rows.Next() {
		taskItem, err := scanTaskItemWithUsers(rows)
		if err != nil {
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

	if params != nil && params.State != nil {
		args = append(args, *params.State)
		query += fmt.Sprintf(" AND state = $%d", len(args))
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
	taskItem, err := scanTaskItem(h.pool.QueryRow(h.ctx, `
		SELECT `+taskItemColumns+`
		FROM sirkel_engine.task_items
		WHERE id = $1
	`, *taskItemID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return taskItem, nil
}
