package sirkel_repository

import (
	"context"
	"errors"
	"testing"

	"sirkel-engine-lib/sirkel_domain"
)

func taskItemNames(items []*sirkel_domain.TaskItem) []string {
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.Name
	}

	return names
}

// newTaskItemSortFixture creates a task with items "A" to "D" saved in that order, so they list as D, C, B, A.
func newTaskItemSortFixture(t *testing.T) (*TaskItemsRepositoryHandler, *sirkel_domain.Task, map[string]*sirkel_domain.TaskItem) {
	t.Helper()

	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	task := createTestTask(t, NewTasksRepositoryHandler(context.Background(), pool, nil), goal.ID)
	handler := NewTaskItemsRepositoryHandler(context.Background(), pool, nil)

	items := map[string]*sirkel_domain.TaskItem{}
	for _, name := range []string{"A", "B", "C", "D"} {
		items[name] = mustSaveTaskItem(t, handler, task.ID, name, sirkel_domain.TaskItemStatePending)
	}

	return handler, task, items
}

func listTaskItemNames(t *testing.T, handler *TaskItemsRepositoryHandler, params *GetTaskItemsParams) []string {
	t.Helper()

	items, err := handler.GetTaskItems(params)
	if err != nil {
		t.Fatalf("GetTaskItems() error = %v", err)
	}

	return taskItemNames(items)
}

func TestTaskItemsRepositoryHandler_SaveTaskItem_NewItemGoesToTopAndUpdateKeepsPosition(t *testing.T) {
	handler, task, items := newTaskItemSortFixture(t)

	assertNames(t, listTaskItemNames(t, handler, &GetTaskItemsParams{TaskID: &task.ID}), []string{"D", "C", "B", "A"})
	if items["B"].SortKey >= items["A"].SortKey {
		t.Fatalf("expected %q to sort before %q", items["B"].SortKey, items["A"].SortKey)
	}

	before := items["B"].SortKey
	items["B"].Name = "B2"
	if err := handler.SaveTaskItem(items["B"]); err != nil {
		t.Fatalf("SaveTaskItem() update error = %v", err)
	}
	if items["B"].SortKey != before {
		t.Fatalf("expected sort key %q to be kept, got %q", before, items["B"].SortKey)
	}
	assertNames(t, listTaskItemNames(t, handler, &GetTaskItemsParams{TaskID: &task.ID}), []string{"D", "C", "B2", "A"})
}

func TestTaskItemsRepositoryHandler_MoveTaskItem(t *testing.T) {
	handler, task, items := newTaskItemSortFixture(t)
	list := func() []string {
		return listTaskItemNames(t, handler, &GetTaskItemsParams{TaskID: &task.ID})
	}

	if _, err := handler.MoveTaskItem(&MoveTaskItemParams{TaskItemID: items["D"].ID, AfterID: &items["B"].ID, BeforeID: &items["A"].ID}); err != nil {
		t.Fatalf("MoveTaskItem() between error = %v", err)
	}
	assertNames(t, list(), []string{"C", "B", "D", "A"})

	if _, err := handler.MoveTaskItem(&MoveTaskItemParams{TaskItemID: items["C"].ID, AfterID: &items["A"].ID}); err != nil {
		t.Fatalf("MoveTaskItem() after error = %v", err)
	}
	assertNames(t, list(), []string{"B", "D", "A", "C"})

	if _, err := handler.MoveTaskItem(&MoveTaskItemParams{TaskItemID: items["A"].ID, BeforeID: &items["B"].ID}); err != nil {
		t.Fatalf("MoveTaskItem() before error = %v", err)
	}
	assertNames(t, list(), []string{"A", "B", "D", "C"})
}

func TestTaskItemsRepositoryHandler_MoveTaskItem_ChangesStateAndRecordsIt(t *testing.T) {
	handler, task, items := newTaskItemSortFixture(t)
	events := NewTaskEventsRepositoryHandler(context.Background(), handler.pool, nil)
	countEvents := func() int {
		count, err := events.CountTaskEvents(&CountTaskEventsParams{TaskItemID: &items["A"].ID})
		if err != nil {
			t.Fatalf("CountTaskEvents() error = %v", err)
		}
		return count
	}
	initialEvents := countEvents()

	if _, err := handler.MoveTaskItem(&MoveTaskItemParams{TaskItemID: items["A"].ID, BeforeID: &items["D"].ID}); err != nil {
		t.Fatalf("MoveTaskItem() error = %v", err)
	}
	if got := countEvents(); got != initialEvents {
		t.Fatalf("expected a reorder to record no events, got %d new", got-initialEvents)
	}

	state := sirkel_domain.TaskItemStateDone
	moved, err := handler.MoveTaskItem(&MoveTaskItemParams{TaskItemID: items["A"].ID, State: &state})
	if err != nil {
		t.Fatalf("MoveTaskItem() error = %v", err)
	}
	if moved.State != state {
		t.Fatalf("expected state %q, got %q", state, moved.State)
	}
	if got := countEvents(); got != initialEvents+1 {
		t.Fatalf("expected the state change to record 1 event, got %d", got-initialEvents)
	}

	assertNames(t, listTaskItemNames(t, handler, &GetTaskItemsParams{TaskID: &task.ID, State: &state}), []string{"A"})
}

func TestTaskItemsRepositoryHandler_MoveTaskItem_Errors(t *testing.T) {
	handler, _, items := newTaskItemSortFixture(t)
	otherTask := createTestTask(t, NewTasksRepositoryHandler(context.Background(), handler.pool, nil), func() string {
		var goalID string
		if err := handler.pool.QueryRow(context.Background(), `SELECT goal_id FROM sirkel_engine.tasks LIMIT 1`).Scan(&goalID); err != nil {
			t.Fatalf("unable to read goal: %v", err)
		}
		return goalID
	}())
	foreign := mustSaveTaskItem(t, handler, otherTask.ID, "Foreign", sirkel_domain.TaskItemStatePending)
	missingID := testUUID(t)

	tests := []struct {
		name   string
		params *MoveTaskItemParams
		want   error
	}{
		{"nothing to do", &MoveTaskItemParams{TaskItemID: items["A"].ID}, ErrMoveTargetRequired},
		{"unknown item", &MoveTaskItemParams{TaskItemID: missingID, AfterID: &items["A"].ID}, ErrNotFound},
		{"neighbor from another task", &MoveTaskItemParams{TaskItemID: items["A"].ID, BeforeID: &foreign.ID}, ErrNotFound},
		{"next to itself", &MoveTaskItemParams{TaskItemID: items["A"].ID, BeforeID: &items["A"].ID}, ErrInvalidMove},
		{"neighbors out of order", &MoveTaskItemParams{TaskItemID: items["A"].ID, AfterID: &items["B"].ID, BeforeID: &items["C"].ID}, ErrInvalidMove},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := handler.MoveTaskItem(tt.params); !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

func TestTaskItemsRepositoryHandler_GetTaskItems_AfterCursorAndOffsetConflict(t *testing.T) {
	handler, task, items := newTaskItemSortFixture(t)

	limit := 2
	first, err := handler.GetTaskItems(&GetTaskItemsParams{TaskID: &task.ID, Limit: &limit})
	if err != nil {
		t.Fatalf("GetTaskItems() error = %v", err)
	}
	assertNames(t, taskItemNames(first), []string{"D", "C"})

	mustSaveTaskItem(t, handler, task.ID, "E", sirkel_domain.TaskItemStatePending)

	second := listTaskItemNames(t, handler, &GetTaskItemsParams{TaskID: &task.ID, Limit: &limit, After: first[1].Cursor()})
	assertNames(t, second, []string{"B", "A"})

	offset := 1
	_, err = handler.GetTaskItems(&GetTaskItemsParams{TaskID: &task.ID, Offset: &offset, After: items["D"].Cursor()})
	if !errors.Is(err, ErrConflictingPagination) {
		t.Fatalf("expected ErrConflictingPagination, got %v", err)
	}
}

func TestTaskItemsRepositoryHandler_CountTaskItems_FiltersByState(t *testing.T) {
	handler, task, items := newTaskItemSortFixture(t)

	done := sirkel_domain.TaskItemStateDone
	if _, err := handler.MoveTaskItem(&MoveTaskItemParams{TaskItemID: items["B"].ID, State: &done}); err != nil {
		t.Fatalf("MoveTaskItem() error = %v", err)
	}

	count, err := handler.CountTaskItems(&CountTaskItemsParams{TaskID: &task.ID, State: &done})
	if err != nil {
		t.Fatalf("CountTaskItems() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 done item, got %d", count)
	}
}
