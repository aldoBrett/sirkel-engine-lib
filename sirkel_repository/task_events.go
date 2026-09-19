package sirkel_repository

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// fieldChange is a single tracked field that changed during a save.
type fieldChange struct {
	field    string
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
