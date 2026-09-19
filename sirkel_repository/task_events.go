package sirkel_repository

import (
	"context"
	"fmt"

	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fieldChange is a single tracked field that changed during a save.
type fieldChange struct {
	field    sirkel_domain.TaskEventField
	oldValue *string
	newValue *string
}

func strPtr(s string) *string {
	return &s
}

func equalStrings(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}

	return *a == *b
}

// changedFields returns a change for each pair whose values differ.
func changedFields(changes ...fieldChange) []fieldChange {
	var changed []fieldChange
	for _, change := range changes {
		if !equalStrings(change.oldValue, change.newValue) {
			changed = append(changed, change)
		}
	}

	return changed
}

// insertTaskEvents appends the changes to the task history. taskItemID is nil for task events.
func insertTaskEvents(ctx context.Context, tx pgx.Tx, taskID string, taskItemID *string, changedBy *string, changes []fieldChange) error {
	for _, change := range changes {
		_, err := tx.Exec(ctx, `
			INSERT INTO sirkel_engine.task_events (task_id, task_item_id, field, old_value, new_value, changed_by)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, taskID, taskItemID, change.field, change.oldValue, change.newValue, changedBy)
		if err != nil {
			return err
		}
	}

	return nil
}

type GetTaskEventsParams struct {
	TaskID     *string
	TaskItemID *string
	Field      *sirkel_domain.TaskEventField
	Offset     *int
	Limit      *int
}

type CountTaskEventsParams struct {
	TaskID     *string
	TaskItemID *string
	Field      *sirkel_domain.TaskEventField
}

type TaskEventsRepository interface {
	// GetTaskEvents returns the recorded changes, newest first. Filtering by task includes the events of its items.
	GetTaskEvents(params *GetTaskEventsParams) ([]*sirkel_domain.TaskEvent, error)
	CountTaskEvents(params *CountTaskEventsParams) (int, error)
}

type TaskEventsRepositoryHandler struct {
	ctx  context.Context
	pool *pgxpool.Pool
	user *sirkel_domain.User
}

func NewTaskEventsRepositoryHandler(ctx context.Context, pool *pgxpool.Pool, user *sirkel_domain.User) *TaskEventsRepositoryHandler {
	return &TaskEventsRepositoryHandler{
		ctx:  ctx,
		pool: pool,
		user: user,
	}
}

func taskEventFilters(taskID, taskItemID *string, field *sirkel_domain.TaskEventField) (string, []any) {
	var args []any
	conditions := ""

	if taskID != nil {
		args = append(args, *taskID)
		conditions += fmt.Sprintf(" AND task_id = $%d", len(args))
	}

	if taskItemID != nil {
		args = append(args, *taskItemID)
		conditions += fmt.Sprintf(" AND task_item_id = $%d", len(args))
	}

	if field != nil {
		args = append(args, string(*field))
		conditions += fmt.Sprintf(" AND field = $%d", len(args))
	}

	return conditions, args
}

func (h *TaskEventsRepositoryHandler) GetTaskEvents(params *GetTaskEventsParams) ([]*sirkel_domain.TaskEvent, error) {
	if params == nil {
		params = &GetTaskEventsParams{}
	}

	conditions, args := taskEventFilters(params.TaskID, params.TaskItemID, params.Field)
	query := `
		SELECT id, task_id, task_item_id, field, old_value, new_value, changed_by, changed_at
		FROM sirkel_engine.task_events
		WHERE 1 = 1` + conditions + `
		ORDER BY changed_at DESC, id DESC`

	if params.Limit != nil {
		args = append(args, *params.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	if params.Offset != nil {
		args = append(args, *params.Offset)
		query += fmt.Sprintf(" OFFSET $%d", len(args))
	}

	rows, err := h.pool.Query(h.ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*sirkel_domain.TaskEvent
	for rows.Next() {
		event := &sirkel_domain.TaskEvent{}
		if err := rows.Scan(
			&event.ID,
			&event.TaskID,
			&event.TaskItemID,
			&event.Field,
			&event.OldValue,
			&event.NewValue,
			&event.ChangedBy,
			&event.ChangedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

func (h *TaskEventsRepositoryHandler) CountTaskEvents(params *CountTaskEventsParams) (int, error) {
	if params == nil {
		params = &CountTaskEventsParams{}
	}

	conditions, args := taskEventFilters(params.TaskID, params.TaskItemID, params.Field)

	var count int
	err := h.pool.QueryRow(h.ctx, `SELECT count(*) FROM sirkel_engine.task_events WHERE 1 = 1`+conditions, args...).Scan(&count)
	return count, err
}
