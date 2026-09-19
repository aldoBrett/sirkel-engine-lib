package sirkel_repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"roci.dev/fracdex"
)

var (
	// ErrNotFound is returned when the row to move, or a neighbor to move it next to, does not exist in the same list.
	ErrNotFound = errors.New("not found")

	// ErrInvalidMove is returned when the neighbors of a move are the moved row itself or are out of order.
	ErrInvalidMove = errors.New("invalid move")

	// ErrMoveTargetRequired is returned when a move gives neither a neighbor nor a new state.
	ErrMoveTargetRequired = errors.New("a neighbor or a new state is required")

	// ErrConflictingPagination is returned when both Offset and After are given.
	ErrConflictingPagination = errors.New("offset and after cannot be used together")
)

// sortScope describes a table whose rows are ordered by (sort_key, id) within the rows sharing scopeColumn.
type sortScope struct {
	table       string
	scopeColumn string
}

var (
	taskSortScope     = sortScope{table: "sirkel_engine.tasks", scopeColumn: "goal_id"}
	taskItemSortScope = sortScope{table: "sirkel_engine.task_items", scopeColumn: "task_id"}
)

// sortKeyBetween returns a key that sorts between lower and upper. An empty bound means the start or end of the list.
func sortKeyBetween(lower, upper string) (string, error) {
	key, err := fracdex.KeyBetween(lower, upper)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidMove, err)
	}

	return key, nil
}

// lockSortScope serializes the writers that pick a sort key within one list until the transaction ends.
// Without it two of them could read the same neighbors and end up with the same key.
func lockSortScope(ctx context.Context, tx pgx.Tx, scopeID string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, scopeID)
	return err
}

// topKey returns a key that sorts before every row of the list.
func (s sortScope) topKey(ctx context.Context, tx pgx.Tx, scopeID string) (string, error) {
	var first string
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT sort_key
		FROM %s
		WHERE %s = $1
		ORDER BY sort_key, id
		LIMIT 1
	`, s.table, s.scopeColumn), scopeID).Scan(&first)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	return sortKeyBetween("", first)
}

// positionKey returns the key that places movedID between the rows afterID (above it) and beforeID (below it).
// When only one neighbor is given, the row goes right next to it, ahead of the row that follows it in the whole list.
func (s sortScope) positionKey(ctx context.Context, tx pgx.Tx, scopeID, movedID string, afterID, beforeID *string) (string, error) {
	if (afterID != nil && *afterID == movedID) || (beforeID != nil && *beforeID == movedID) {
		return "", fmt.Errorf("%w: a row cannot be moved next to itself", ErrInvalidMove)
	}

	var lower, upper string
	var err error
	switch {
	case afterID != nil && beforeID != nil:
		if lower, err = s.neighborKey(ctx, tx, scopeID, *afterID); err != nil {
			return "", err
		}
		if upper, err = s.neighborKey(ctx, tx, scopeID, *beforeID); err != nil {
			return "", err
		}
		if lower >= upper {
			return "", fmt.Errorf("%w: the row to place it after is not above the row to place it before", ErrInvalidMove)
		}
	case afterID != nil:
		if lower, err = s.neighborKey(ctx, tx, scopeID, *afterID); err != nil {
			return "", err
		}
		if upper, err = s.adjacentKey(ctx, tx, scopeID, movedID, *afterID, lower, true); err != nil {
			return "", err
		}
	default:
		if upper, err = s.neighborKey(ctx, tx, scopeID, *beforeID); err != nil {
			return "", err
		}
		if lower, err = s.adjacentKey(ctx, tx, scopeID, movedID, *beforeID, upper, false); err != nil {
			return "", err
		}
	}

	return sortKeyBetween(lower, upper)
}

// neighborKey returns the sort key of a row of the list.
func (s sortScope) neighborKey(ctx context.Context, tx pgx.Tx, scopeID, id string) (string, error) {
	var key string
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT sort_key
		FROM %s
		WHERE id = $1 AND %s = $2
	`, s.table, s.scopeColumn), id, scopeID).Scan(&key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}

	return key, nil
}

// adjacentKey returns the key of the row right after (or before) the row at (key, id), skipping movedID.
// It is empty when there is none.
func (s sortScope) adjacentKey(ctx context.Context, tx pgx.Tx, scopeID, movedID, id, key string, after bool) (string, error) {
	comparison, direction := ">", "ASC"
	if !after {
		comparison, direction = "<", "DESC"
	}

	var adjacent string
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT sort_key
		FROM %[1]s
		WHERE %[2]s = $1 AND id <> $2 AND (sort_key, id) %[3]s ($3, $4)
		ORDER BY sort_key %[4]s, id %[4]s
		LIMIT 1
	`, s.table, s.scopeColumn, comparison, direction), scopeID, movedID, key, id).Scan(&adjacent)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}

	return adjacent, nil
}
