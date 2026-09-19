package sirkel_repository

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

func taskNames(tasks []*sirkel_domain.Task) []string {
	names := make([]string, len(tasks))
	for i, task := range tasks {
		names[i] = task.Name
	}

	return names
}

func assertNames(t *testing.T, got, want []string) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Fatalf("expected order %v, got %v", want, got)
	}
}

// newTaskSortFixture creates a goal with tasks "A" to "D" saved in that order, so they list as D, C, B, A.
func newTaskSortFixture(t *testing.T) (*pgxpool.Pool, *TasksRepositoryHandler, *sirkel_domain.Goal, map[string]*sirkel_domain.Task) {
	t.Helper()

	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	handler := NewTasksRepositoryHandler(context.Background(), pool, nil)

	tasks := map[string]*sirkel_domain.Task{}
	for _, name := range []string{"A", "B", "C", "D"} {
		tasks[name] = mustSaveTask(t, handler, goal.ID, name, sirkel_domain.TaskStateTodo)
	}

	return pool, handler, goal, tasks
}

func listTaskNames(t *testing.T, handler *TasksRepositoryHandler, params *GetTasksParams) []string {
	t.Helper()

	tasks, err := handler.GetTasks(params)
	if err != nil {
		t.Fatalf("GetTasks() error = %v", err)
	}

	return taskNames(tasks)
}

func TestTasksRepositoryHandler_SaveTask_NewTaskGoesToTop(t *testing.T) {
	_, handler, goal, tasks := newTaskSortFixture(t)

	assertNames(t, listTaskNames(t, handler, &GetTasksParams{GoalID: &goal.ID}), []string{"D", "C", "B", "A"})

	// The key above "a0" is "Zz", which only sorts first when compared byte by byte.
	if tasks["B"].SortKey >= tasks["A"].SortKey {
		t.Fatalf("expected %q to sort before %q", tasks["B"].SortKey, tasks["A"].SortKey)
	}
}

func TestTasksRepositoryHandler_SaveTask_UpdateKeepsPosition(t *testing.T) {
	_, handler, goal, tasks := newTaskSortFixture(t)

	before := tasks["B"].SortKey
	tasks["B"].Name = "B2"
	tasks["B"].SortKey = "ignored by the save"
	if err := handler.SaveTask(tasks["B"]); err != nil {
		t.Fatalf("SaveTask() update error = %v", err)
	}

	if tasks["B"].SortKey != before {
		t.Fatalf("expected sort key %q to be kept, got %q", before, tasks["B"].SortKey)
	}
	assertNames(t, listTaskNames(t, handler, &GetTasksParams{GoalID: &goal.ID}), []string{"D", "C", "B2", "A"})
}

func TestTasksRepositoryHandler_MoveTask(t *testing.T) {
	tests := []struct {
		name  string
		move  func(tasks map[string]*sirkel_domain.Task) *MoveTaskParams
		order []string
	}{
		{
			name: "between two tasks",
			move: func(tasks map[string]*sirkel_domain.Task) *MoveTaskParams {
				return &MoveTaskParams{TaskID: tasks["D"].ID, AfterID: &tasks["B"].ID, BeforeID: &tasks["A"].ID}
			},
			order: []string{"C", "B", "D", "A"},
		},
		{
			name: "after a task, which is the end of the list",
			move: func(tasks map[string]*sirkel_domain.Task) *MoveTaskParams {
				return &MoveTaskParams{TaskID: tasks["D"].ID, AfterID: &tasks["A"].ID}
			},
			order: []string{"C", "B", "A", "D"},
		},
		{
			name: "before a task, which is the top of the list",
			move: func(tasks map[string]*sirkel_domain.Task) *MoveTaskParams {
				return &MoveTaskParams{TaskID: tasks["A"].ID, BeforeID: &tasks["D"].ID}
			},
			order: []string{"A", "D", "C", "B"},
		},
		{
			name: "after a task that is followed by the task being moved",
			move: func(tasks map[string]*sirkel_domain.Task) *MoveTaskParams {
				return &MoveTaskParams{TaskID: tasks["C"].ID, AfterID: &tasks["D"].ID}
			},
			order: []string{"D", "C", "B", "A"},
		},
		{
			name: "before a task that follows the task being moved",
			move: func(tasks map[string]*sirkel_domain.Task) *MoveTaskParams {
				return &MoveTaskParams{TaskID: tasks["C"].ID, BeforeID: &tasks["B"].ID}
			},
			order: []string{"D", "C", "B", "A"},
		},
		{
			name: "after a task in the middle of the list",
			move: func(tasks map[string]*sirkel_domain.Task) *MoveTaskParams {
				return &MoveTaskParams{TaskID: tasks["A"].ID, AfterID: &tasks["D"].ID}
			},
			order: []string{"D", "A", "C", "B"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, handler, goal, tasks := newTaskSortFixture(t)
			params := tt.move(tasks)

			moved, err := handler.MoveTask(params)
			if err != nil {
				t.Fatalf("MoveTask() error = %v", err)
			}
			if moved.ID != params.TaskID {
				t.Fatalf("expected task %q, got %q", params.TaskID, moved.ID)
			}

			list, err := handler.GetTasks(&GetTasksParams{GoalID: &goal.ID})
			if err != nil {
				t.Fatalf("GetTasks() error = %v", err)
			}
			assertNames(t, taskNames(list), tt.order)
			for _, task := range list {
				if task.ID == moved.ID && task.SortKey != moved.SortKey {
					t.Fatalf("expected the returned task to carry the new sort key %q, got %q", task.SortKey, moved.SortKey)
				}
			}
		})
	}
}

func TestTasksRepositoryHandler_MoveTask_ManyMovesToTheSameSpotKeepTheOrder(t *testing.T) {
	_, handler, goal, _ := newTaskSortFixture(t)

	list, err := handler.GetTasks(&GetTasksParams{GoalID: &goal.ID})
	if err != nil {
		t.Fatalf("GetTasks() error = %v", err)
	}
	order := make([]string, len(list))
	for i, task := range list {
		order[i] = task.ID
	}

	// Moving the last task between the first two, over and over, squeezes every new key next to the previous one,
	// which is what makes keys longer.
	for i := range 40 {
		moved, err := handler.MoveTask(&MoveTaskParams{TaskID: order[3], AfterID: &order[0], BeforeID: &order[1]})
		if err != nil {
			t.Fatalf("MoveTask() #%d error = %v", i, err)
		}
		order = []string{order[0], order[3], order[1], order[2]}

		list, err := handler.GetTasks(&GetTasksParams{GoalID: &goal.ID})
		if err != nil {
			t.Fatalf("GetTasks() error = %v", err)
		}
		for j, task := range list {
			if task.ID != order[j] {
				t.Fatalf("move #%d: expected %v at position %d, got %v (key %q)", i, order[j], j, task.ID, moved.SortKey)
			}
		}
	}
}

func TestTasksRepositoryHandler_MoveTask_ChangesStateAndRecordsIt(t *testing.T) {
	pool, _, goal, _ := newTaskSortFixture(t)
	organizationID := createTestOrganization(t, pool)
	actor := createTestUser(t, pool, organizationID)
	handler := NewTasksRepositoryHandler(context.Background(), pool, actor)

	tasks, err := handler.GetTasks(&GetTasksParams{GoalID: &goal.ID})
	if err != nil {
		t.Fatalf("GetTasks() error = %v", err)
	}
	moving, other := tasks[0], tasks[3]
	events := NewTaskEventsRepositoryHandler(context.Background(), pool, nil)
	countEvents := func() int {
		count, err := events.CountTaskEvents(&CountTaskEventsParams{TaskID: &moving.ID})
		if err != nil {
			t.Fatalf("CountTaskEvents() error = %v", err)
		}
		return count
	}
	initialEvents := countEvents()

	// A plain reorder is not a change worth recording.
	if _, err := handler.MoveTask(&MoveTaskParams{TaskID: moving.ID, AfterID: &other.ID}); err != nil {
		t.Fatalf("MoveTask() error = %v", err)
	}
	if got := countEvents(); got != initialEvents {
		t.Fatalf("expected a reorder to record no events, got %d new", got-initialEvents)
	}
	reordered, err := handler.GetTaskByID(&moving.ID)
	if err != nil {
		t.Fatalf("GetTaskByID() error = %v", err)
	}
	if reordered.UpdatedBy != nil {
		t.Fatalf("expected a reorder not to touch updated_by, got %q", *reordered.UpdatedBy)
	}

	state := sirkel_domain.TaskStateInProgress
	moved, err := handler.MoveTask(&MoveTaskParams{TaskID: moving.ID, BeforeID: &other.ID, State: &state})
	if err != nil {
		t.Fatalf("MoveTask() error = %v", err)
	}
	if moved.State != state {
		t.Fatalf("expected state %q, got %q", state, moved.State)
	}
	assertUserID(t, "updated_by", moved.UpdatedBy, actor.ID)
	if got := countEvents(); got != initialEvents+1 {
		t.Fatalf("expected the state change to record 1 event, got %d", got-initialEvents)
	}
}

func TestTasksRepositoryHandler_MoveTask_StateOnlyKeepsPosition(t *testing.T) {
	_, handler, goal, tasks := newTaskSortFixture(t)

	state := sirkel_domain.TaskStateDone
	moved, err := handler.MoveTask(&MoveTaskParams{TaskID: tasks["C"].ID, State: &state})
	if err != nil {
		t.Fatalf("MoveTask() error = %v", err)
	}
	if moved.State != state || moved.SortKey != tasks["C"].SortKey {
		t.Fatalf("expected state %q and key %q, got %q and %q", state, tasks["C"].SortKey, moved.State, moved.SortKey)
	}
	assertNames(t, listTaskNames(t, handler, &GetTasksParams{GoalID: &goal.ID}), []string{"D", "C", "B", "A"})
}

func TestTasksRepositoryHandler_MoveTask_Errors(t *testing.T) {
	pool, handler, goal, tasks := newTaskSortFixture(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	otherGoal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	foreign := mustSaveTask(t, handler, otherGoal.ID, "Foreign", sirkel_domain.TaskStateTodo)
	missingID := testUUID(t)

	tests := []struct {
		name   string
		params *MoveTaskParams
		want   error
	}{
		{"nothing to do", &MoveTaskParams{TaskID: tasks["A"].ID}, ErrMoveTargetRequired},
		{"unknown task", &MoveTaskParams{TaskID: missingID, AfterID: &tasks["A"].ID}, ErrNotFound},
		{"unknown neighbor", &MoveTaskParams{TaskID: tasks["A"].ID, AfterID: &missingID}, ErrNotFound},
		{"neighbor from another goal", &MoveTaskParams{TaskID: tasks["A"].ID, BeforeID: &foreign.ID}, ErrNotFound},
		{"next to itself", &MoveTaskParams{TaskID: tasks["A"].ID, AfterID: &tasks["A"].ID}, ErrInvalidMove},
		{"neighbors out of order", &MoveTaskParams{TaskID: tasks["A"].ID, AfterID: &tasks["B"].ID, BeforeID: &tasks["C"].ID}, ErrInvalidMove},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := handler.MoveTask(tt.params); !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}

	assertNames(t, listTaskNames(t, handler, &GetTasksParams{GoalID: &goal.ID}), []string{"D", "C", "B", "A"})
}

func TestTasksRepositoryHandler_GetTasks_AfterCursorIsStableWhileTheListChanges(t *testing.T) {
	_, handler, goal, tasks := newTaskSortFixture(t)

	limit := 2
	first, err := handler.GetTasks(&GetTasksParams{GoalID: &goal.ID, Limit: &limit})
	if err != nil {
		t.Fatalf("GetTasks() error = %v", err)
	}
	assertNames(t, taskNames(first), []string{"D", "C"})

	// A new task on top would shift an offset by one and repeat C. The cursor does not care.
	mustSaveTask(t, handler, goal.ID, "E", sirkel_domain.TaskStateTodo)

	second, err := handler.GetTasks(&GetTasksParams{GoalID: &goal.ID, Limit: &limit, After: first[len(first)-1].Cursor()})
	if err != nil {
		t.Fatalf("GetTasks() error = %v", err)
	}
	assertNames(t, taskNames(second), []string{"B", "A"})

	// Moving a task from the loaded part to below the cursor makes it show up on the next page, once.
	if _, err := handler.MoveTask(&MoveTaskParams{TaskID: tasks["D"].ID, AfterID: &tasks["B"].ID, BeforeID: &tasks["A"].ID}); err != nil {
		t.Fatalf("MoveTask() error = %v", err)
	}
	third, err := handler.GetTasks(&GetTasksParams{GoalID: &goal.ID, Limit: &limit, After: first[len(first)-1].Cursor()})
	if err != nil {
		t.Fatalf("GetTasks() error = %v", err)
	}
	assertNames(t, taskNames(third), []string{"B", "D"})

	last, err := handler.GetTasks(&GetTasksParams{GoalID: &goal.ID, Limit: &limit, After: third[len(third)-1].Cursor()})
	if err != nil {
		t.Fatalf("GetTasks() error = %v", err)
	}
	assertNames(t, taskNames(last), []string{"A"})
}

func TestTasksRepositoryHandler_GetTasks_RejectsOffsetWithAfter(t *testing.T) {
	_, handler, goal, tasks := newTaskSortFixture(t)

	offset := 1
	_, err := handler.GetTasks(&GetTasksParams{GoalID: &goal.ID, Offset: &offset, After: tasks["D"].Cursor()})
	if !errors.Is(err, ErrConflictingPagination) {
		t.Fatalf("expected ErrConflictingPagination, got %v", err)
	}
}

func TestTasksRepositoryHandler_GetTasks_FiltersByState(t *testing.T) {
	_, handler, goal, tasks := newTaskSortFixture(t)

	done := sirkel_domain.TaskStateDone
	for _, name := range []string{"A", "C"} {
		if _, err := handler.MoveTask(&MoveTaskParams{TaskID: tasks[name].ID, State: &done}); err != nil {
			t.Fatalf("MoveTask() error = %v", err)
		}
	}

	assertNames(t, listTaskNames(t, handler, &GetTasksParams{GoalID: &goal.ID, State: &done}), []string{"C", "A"})

	limit := 1
	firstDone, err := handler.GetTasks(&GetTasksParams{GoalID: &goal.ID, State: &done, Limit: &limit})
	if err != nil {
		t.Fatalf("GetTasks() error = %v", err)
	}
	next := listTaskNames(t, handler, &GetTasksParams{GoalID: &goal.ID, State: &done, Limit: &limit, After: firstDone[0].Cursor()})
	assertNames(t, next, []string{"A"})

	count, err := handler.CountTasks(&CountTasksParams{GoalID: &goal.ID, State: &done})
	if err != nil {
		t.Fatalf("CountTasks() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 done tasks, got %d", count)
	}
}

// Fractional keys only order correctly when compared byte by byte. Many databases default to a locale collation that
// ignores case, so the columns must say so themselves, whatever the test database happens to default to.
func TestMigration_SortKeyColumnsUseTheCCollation(t *testing.T) {
	pool := testPool(t)

	rows, err := pool.Query(context.Background(), `
		SELECT table_name, collation_name
		FROM information_schema.columns
		WHERE table_schema = 'sirkel_engine' AND column_name = 'sort_key'
	`)
	if err != nil {
		t.Fatalf("unable to read columns: %v", err)
	}
	defer rows.Close()

	found := 0
	for rows.Next() {
		var table string
		var collation *string
		if err := rows.Scan(&table, &collation); err != nil {
			t.Fatalf("unable to scan column: %v", err)
		}
		if collation == nil || *collation != "C" {
			t.Fatalf("expected %s.sort_key to use the C collation, got %v", table, collation)
		}
		found++
	}
	if found != 2 {
		t.Fatalf("expected sort_key on tasks and task_items, found %d columns", found)
	}
}

// The migration runs on every start, so it has to backfill rows created before the column existed and leave
// rows that already have a key alone.
func TestMigration_SortKeysBackfillKeepsCreatedAtDescOrder(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(ctx, pool, nil), organizationID)
	goalA := createTestGoal(t, NewGoalsRepositoryHandler(ctx, pool, nil), project.ID)
	goalB := createTestGoal(t, NewGoalsRepositoryHandler(ctx, pool, nil), project.ID)

	for _, statement := range []string{
		`ALTER TABLE sirkel_engine.tasks DROP COLUMN sort_key`,
		`ALTER TABLE sirkel_engine.task_items DROP COLUMN sort_key`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("unable to drop the column: %v", err)
		}
	}

	// 70 rows in goal A, enough to need a second base62 digit, and one in goal B.
	for i := range 70 {
		_, err := pool.Exec(ctx, `
			INSERT INTO sirkel_engine.tasks (goal_id, name, created_at)
			VALUES ($1, $2, now() - make_interval(secs => $3))
		`, goalA.ID, fmt.Sprintf("A%02d", i), 70-i)
		if err != nil {
			t.Fatalf("unable to insert task: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sirkel_engine.tasks (goal_id, name) VALUES ($1, 'B')`, goalB.ID); err != nil {
		t.Fatalf("unable to insert task: %v", err)
	}
	var taskID string
	if err := pool.QueryRow(ctx, `SELECT id FROM sirkel_engine.tasks WHERE name = 'B'`).Scan(&taskID); err != nil {
		t.Fatalf("unable to read task: %v", err)
	}
	for i := range 3 {
		_, err := pool.Exec(ctx, `
			INSERT INTO sirkel_engine.task_items (task_id, name, created_at)
			VALUES ($1, $2, now() - make_interval(secs => $3))
		`, taskID, fmt.Sprintf("I%d", i), 3-i)
		if err != nil {
			t.Fatalf("unable to insert task item: %v", err)
		}
	}

	applyMigrations(t, pool)
	applyMigrations(t, pool) // running it again must change nothing

	handler := NewTasksRepositoryHandler(ctx, pool, nil)
	tasks, err := handler.GetTasks(&GetTasksParams{GoalID: &goalA.ID})
	if err != nil {
		t.Fatalf("GetTasks() error = %v", err)
	}
	if len(tasks) != 70 {
		t.Fatalf("expected 70 tasks, got %d", len(tasks))
	}
	for i, task := range tasks {
		// created_at DESC: the last inserted row (A69) comes first.
		if want := fmt.Sprintf("A%02d", 69-i); task.Name != want {
			t.Fatalf("position %d: expected %s, got %s", i, want, task.Name)
		}
	}

	items, err := NewTaskItemsRepositoryHandler(ctx, pool, nil).GetTaskItems(&GetTaskItemsParams{TaskID: &taskID})
	if err != nil {
		t.Fatalf("GetTaskItems() error = %v", err)
	}
	if got := taskItemNames(items); !slices.Equal(got, []string{"I2", "I1", "I0"}) {
		t.Fatalf("expected items I2, I1, I0, got %v", got)
	}

	// The backfilled keys are valid fractional indexes, so tasks can be inserted and moved among them.
	created := mustSaveTask(t, handler, goalA.ID, "New", sirkel_domain.TaskStateTodo)
	if created.SortKey >= tasks[0].SortKey {
		t.Fatalf("expected the new task key %q to sort before %q", created.SortKey, tasks[0].SortKey)
	}
	if _, err := handler.MoveTask(&MoveTaskParams{TaskID: created.ID, AfterID: &tasks[30].ID, BeforeID: &tasks[31].ID}); err != nil {
		t.Fatalf("MoveTask() error = %v", err)
	}
}
