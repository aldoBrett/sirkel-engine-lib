package sirkel_repository

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

// runConcurrently starts n goroutines together and waits for all of them.
func runConcurrently(n int, fn func(i int)) {
	var ready, done sync.WaitGroup
	start := make(chan struct{})
	ready.Add(n)
	done.Add(n)
	for i := range n {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			fn(i)
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()
}

// assertDistinctSortKeys checks that no two rows of the list share a key, which is what keeps a later move between
// any two neighbors possible.
func assertDistinctSortKeys(t *testing.T, pool *pgxpool.Pool, table, scopeColumn, scopeID string, wantRows int) {
	t.Helper()

	var rows, distinct int
	err := pool.QueryRow(context.Background(), fmt.Sprintf(`
		SELECT count(*), count(DISTINCT sort_key)
		FROM %s
		WHERE %s = $1
	`, table, scopeColumn), scopeID).Scan(&rows, &distinct)
	if err != nil {
		t.Fatalf("unable to count sort keys: %v", err)
	}
	if rows != wantRows {
		t.Fatalf("expected %d rows, got %d", wantRows, rows)
	}
	if distinct != rows {
		t.Fatalf("expected %d distinct sort keys, got %d", rows, distinct)
	}
}

func TestTasksRepositoryHandler_SaveTask_ConcurrentInsertsGetDistinctKeys(t *testing.T) {
	pool, handler, goal, _ := newTaskSortFixture(t)

	const writers = 30
	runConcurrently(writers, func(i int) {
		task := &sirkel_domain.Task{ID: testUUID(t), GoalID: goal.ID, Name: fmt.Sprintf("Concurrent %d", i), State: sirkel_domain.TaskStateTodo}
		if err := handler.SaveTask(task); err != nil {
			t.Errorf("SaveTask() #%d error = %v", i, err)
		}
	})

	assertDistinctSortKeys(t, pool, "sirkel_engine.tasks", "goal_id", goal.ID, 4+writers)
}

func TestTaskItemsRepositoryHandler_SaveTaskItem_ConcurrentInsertsGetDistinctKeys(t *testing.T) {
	handler, task, _ := newTaskItemSortFixture(t)

	const writers = 30
	runConcurrently(writers, func(i int) {
		item := &sirkel_domain.TaskItem{ID: testUUID(t), TaskID: task.ID, Name: fmt.Sprintf("Concurrent %d", i), State: sirkel_domain.TaskItemStatePending}
		if err := handler.SaveTaskItem(item); err != nil {
			t.Errorf("SaveTaskItem() #%d error = %v", i, err)
		}
	})

	assertDistinctSortKeys(t, handler.pool, "sirkel_engine.task_items", "task_id", task.ID, 4+writers)
}

// Two clients can drop a task into the same gap: the second one has to land inside the gap too, not on top of the first.
func TestTasksRepositoryHandler_MoveTask_TasksMovedIntoTheSameGapGetDistinctKeys(t *testing.T) {
	pool, handler, goal, tasks := newTaskSortFixture(t)

	// The list is D, C, B, A. Move A and then B into the gap between D and C.
	for _, name := range []string{"A", "B"} {
		if _, err := handler.MoveTask(&MoveTaskParams{TaskID: tasks[name].ID, AfterID: &tasks["D"].ID, BeforeID: &tasks["C"].ID}); err != nil {
			t.Fatalf("MoveTask() %s error = %v", name, err)
		}
	}

	assertDistinctSortKeys(t, pool, "sirkel_engine.tasks", "goal_id", goal.ID, 4)
	assertNames(t, listTaskNames(t, handler, &GetTasksParams{GoalID: &goal.ID}), []string{"D", "B", "A", "C"})

	// Both still have room around them, so a further move between the two of them works.
	if _, err := handler.MoveTask(&MoveTaskParams{TaskID: tasks["C"].ID, AfterID: &tasks["B"].ID, BeforeID: &tasks["A"].ID}); err != nil {
		t.Fatalf("MoveTask() between the two error = %v", err)
	}
	assertNames(t, listTaskNames(t, handler, &GetTasksParams{GoalID: &goal.ID}), []string{"D", "B", "C", "A"})
}

func TestTasksRepositoryHandler_MoveTask_ConcurrentMovesIntoTheSameGapGetDistinctKeys(t *testing.T) {
	pool, handler, goal, _ := newTaskSortFixture(t)

	// 20 more tasks to move, all into the gap between D and C.
	movers := make([]*sirkel_domain.Task, 20)
	for i := range movers {
		movers[i] = mustSaveTask(t, handler, goal.ID, fmt.Sprintf("Mover %d", i), sirkel_domain.TaskStateTodo)
	}
	list, err := handler.GetTasks(&GetTasksParams{GoalID: &goal.ID})
	if err != nil {
		t.Fatalf("GetTasks() error = %v", err)
	}
	// The movers are on top now, so use the two oldest as the neighbors.
	above, below := list[len(list)-2].ID, list[len(list)-1].ID

	runConcurrently(len(movers), func(i int) {
		if _, err := handler.MoveTask(&MoveTaskParams{TaskID: movers[i].ID, AfterID: &above, BeforeID: &below}); err != nil {
			t.Errorf("MoveTask() #%d error = %v", i, err)
		}
	})

	assertDistinctSortKeys(t, pool, "sirkel_engine.tasks", "goal_id", goal.ID, 4+len(movers))
}

// Inserts, moves and state changes in the same list at once must neither deadlock nor lose rows.
func TestTasksRepositoryHandler_ConcurrentInsertsAndMovesDoNotDeadlock(t *testing.T) {
	pool, handler, goal, tasks := newTaskSortFixture(t)

	ids := []string{tasks["A"].ID, tasks["B"].ID, tasks["C"].ID, tasks["D"].ID}
	const writers = 40
	runConcurrently(writers, func(i int) {
		switch i % 3 {
		case 0:
			task := &sirkel_domain.Task{ID: testUUID(t), GoalID: goal.ID, Name: fmt.Sprintf("Insert %d", i), State: sirkel_domain.TaskStateTodo}
			if err := handler.SaveTask(task); err != nil {
				t.Errorf("SaveTask() #%d error = %v", i, err)
			}
		case 1:
			moving, neighbor := ids[i%len(ids)], ids[(i+1)%len(ids)]
			if _, err := handler.MoveTask(&MoveTaskParams{TaskID: moving, AfterID: &neighbor}); err != nil {
				t.Errorf("MoveTask() #%d error = %v", i, err)
			}
		default:
			state := sirkel_domain.TaskStateInProgress
			moving, neighbor := ids[i%len(ids)], ids[(i+2)%len(ids)]
			if _, err := handler.MoveTask(&MoveTaskParams{TaskID: moving, BeforeID: &neighbor, State: &state}); err != nil {
				t.Errorf("MoveTask() #%d error = %v", i, err)
			}
		}
	})

	var inserts int
	for i := range writers {
		if i%3 == 0 {
			inserts++
		}
	}
	assertDistinctSortKeys(t, pool, "sirkel_engine.tasks", "goal_id", goal.ID, 4+inserts)
}
