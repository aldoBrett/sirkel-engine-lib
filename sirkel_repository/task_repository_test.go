package sirkel_repository

import (
	"context"
	"fmt"
	"testing"

	"sirkel-engine-lib/sirkel_domain"
)

func createTestGoal(t *testing.T, handler *GoalsRepositoryHandler, projectID string) *sirkel_domain.Goal {
	t.Helper()

	return mustSaveGoal(t, handler, projectID, "Test Goal", sirkel_domain.GoalStateActive)
}

func mustSaveTask(t *testing.T, handler *TasksRepositoryHandler, goalID, name string, state sirkel_domain.TaskState) *sirkel_domain.Task {
	t.Helper()

	description := "test description"
	task := &sirkel_domain.Task{
		GoalID:      goalID,
		Name:        name,
		Description: &description,
		State:       state,
	}
	if err := handler.SaveTask(task); err != nil {
		t.Fatalf("SaveTask() error = %v", err)
	}

	return task
}

func TestTasksRepositoryHandler_SaveTask_Insert(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	handler := NewTasksRepositoryHandler(context.Background(), pool, nil)

	description := "Assemble the rocket"
	task := &sirkel_domain.Task{
		GoalID:      goal.ID,
		Name:        "Assembly",
		Description: &description,
		State:       sirkel_domain.TaskStateTodo,
	}

	if err := handler.SaveTask(task); err != nil {
		t.Fatalf("SaveTask() error = %v", err)
	}

	if task.ID == "" {
		t.Fatal("expected task ID to be set after insert")
	}
	if task.CreatedAt.IsZero() {
		t.Fatal("expected created_at to be set after insert")
	}
	if task.UpdatedAt.IsZero() {
		t.Fatal("expected updated_at to be set after insert")
	}
}

func TestTasksRepositoryHandler_SaveTask_Update(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	handler := NewTasksRepositoryHandler(context.Background(), pool, nil)

	task := mustSaveTask(t, handler, goal.ID, "Assembly", sirkel_domain.TaskStateTodo)

	task.Name = "Assembly v2"
	task.State = sirkel_domain.TaskStateInProgress

	if err := handler.SaveTask(task); err != nil {
		t.Fatalf("SaveTask() update error = %v", err)
	}

	fetched, err := handler.GetTaskByID(&task.ID)
	if err != nil {
		t.Fatalf("GetTaskByID() error = %v", err)
	}
	if fetched == nil {
		t.Fatal("expected task to exist after update")
	}
	if fetched.Name != "Assembly v2" {
		t.Fatalf("expected name %q, got %q", "Assembly v2", fetched.Name)
	}
	if fetched.State != sirkel_domain.TaskStateInProgress {
		t.Fatalf("expected state %q, got %q", sirkel_domain.TaskStateInProgress, fetched.State)
	}
}

func TestTasksRepositoryHandler_SaveTask_NullDescription(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	handler := NewTasksRepositoryHandler(context.Background(), pool, nil)

	task := &sirkel_domain.Task{
		GoalID: goal.ID,
		Name:   "No description",
		State:  sirkel_domain.TaskStateTodo,
	}

	if err := handler.SaveTask(task); err != nil {
		t.Fatalf("SaveTask() error = %v", err)
	}

	fetched, err := handler.GetTaskByID(&task.ID)
	if err != nil {
		t.Fatalf("GetTaskByID() error = %v", err)
	}
	if fetched == nil {
		t.Fatal("expected task to exist")
	}
	if fetched.Description != nil {
		t.Fatalf("expected nil description, got %v", *fetched.Description)
	}
}

func TestTasksRepositoryHandler_GetTaskByID_NotFound(t *testing.T) {
	pool := testPool(t)
	handler := NewTasksRepositoryHandler(context.Background(), pool, nil)

	missingID := "00000000-0000-0000-0000-000000000000"
	task, err := handler.GetTaskByID(&missingID)
	if err != nil {
		t.Fatalf("GetTaskByID() error = %v", err)
	}
	if task != nil {
		t.Fatalf("expected nil task, got %+v", task)
	}
}

func TestTasksRepositoryHandler_GetTasks_FiltersByGoal(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goalsHandler := NewGoalsRepositoryHandler(context.Background(), pool, nil)
	goalA := createTestGoal(t, goalsHandler, project.ID)
	goalB := createTestGoal(t, goalsHandler, project.ID)
	handler := NewTasksRepositoryHandler(context.Background(), pool, nil)

	mustSaveTask(t, handler, goalA.ID, "A1", sirkel_domain.TaskStateTodo)
	mustSaveTask(t, handler, goalA.ID, "A2", sirkel_domain.TaskStateTodo)
	mustSaveTask(t, handler, goalB.ID, "B1", sirkel_domain.TaskStateTodo)

	tasks, err := handler.GetTasks(&GetTasksParams{GoalID: &goalA.ID})
	if err != nil {
		t.Fatalf("GetTasks() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks for goal A, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.GoalID != goalA.ID {
			t.Fatalf("expected task to belong to goal %q, got %q", goalA.ID, task.GoalID)
		}
	}
}

func TestTasksRepositoryHandler_GetTasks_Pagination(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	handler := NewTasksRepositoryHandler(context.Background(), pool, nil)

	for i := range 3 {
		mustSaveTask(t, handler, goal.ID, fmt.Sprintf("Task %d", i), sirkel_domain.TaskStateTodo)
	}

	limit := 2
	offset := 1
	tasks, err := handler.GetTasks(&GetTasksParams{
		GoalID: &goal.ID,
		Limit:  &limit,
		Offset: &offset,
	})
	if err != nil {
		t.Fatalf("GetTasks() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks with limit/offset, got %d", len(tasks))
	}
}

func TestTasksRepositoryHandler_CountTasks(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goalsHandler := NewGoalsRepositoryHandler(context.Background(), pool, nil)
	goalA := createTestGoal(t, goalsHandler, project.ID)
	goalB := createTestGoal(t, goalsHandler, project.ID)
	handler := NewTasksRepositoryHandler(context.Background(), pool, nil)

	mustSaveTask(t, handler, goalA.ID, "A1", sirkel_domain.TaskStateTodo)
	mustSaveTask(t, handler, goalA.ID, "A2", sirkel_domain.TaskStateTodo)
	mustSaveTask(t, handler, goalB.ID, "B1", sirkel_domain.TaskStateTodo)

	count, err := handler.CountTasks(&CountTasksParams{GoalID: &goalA.ID})
	if err != nil {
		t.Fatalf("CountTasks() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}

	total, err := handler.CountTasks(&CountTasksParams{})
	if err != nil {
		t.Fatalf("CountTasks() error = %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total count 3, got %d", total)
	}
}

func TestTasksRepositoryHandler_DeleteTask(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	handler := NewTasksRepositoryHandler(context.Background(), pool, nil)

	task := mustSaveTask(t, handler, goal.ID, "To delete", sirkel_domain.TaskStateTodo)

	if err := handler.DeleteTask(&task.ID); err != nil {
		t.Fatalf("DeleteTask() error = %v", err)
	}

	fetched, err := handler.GetTaskByID(&task.ID)
	if err != nil {
		t.Fatalf("GetTaskByID() error = %v", err)
	}
	if fetched != nil {
		t.Fatalf("expected task to be deleted, got %+v", fetched)
	}
}
