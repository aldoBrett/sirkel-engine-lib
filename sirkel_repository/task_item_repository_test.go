package sirkel_repository

import (
	"context"
	"fmt"
	"testing"

	"sirkel-engine-lib/sirkel_domain"
)

func createTestTask(t *testing.T, handler *TasksRepositoryHandler, goalID string) *sirkel_domain.Task {
	t.Helper()

	return mustSaveTask(t, handler, goalID, "Test Task", sirkel_domain.TaskStateTodo)
}

func mustSaveTaskItem(t *testing.T, handler *TaskItemsRepositoryHandler, taskID, name string, state sirkel_domain.TaskItemState) *sirkel_domain.TaskItem {
	t.Helper()

	description := "test description"
	taskItem := &sirkel_domain.TaskItem{
		TaskID:      taskID,
		Name:        name,
		Description: &description,
		State:       state,
	}
	if err := handler.SaveTaskItem(taskItem); err != nil {
		t.Fatalf("SaveTaskItem() error = %v", err)
	}

	return taskItem
}

func TestTaskItemsRepositoryHandler_SaveTaskItem_Insert(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	task := createTestTask(t, NewTasksRepositoryHandler(context.Background(), pool, nil), goal.ID)
	handler := NewTaskItemsRepositoryHandler(context.Background(), pool, nil)

	description := "Tighten the bolts"
	taskItem := &sirkel_domain.TaskItem{
		TaskID:      task.ID,
		Name:        "Bolts",
		Description: &description,
		State:       sirkel_domain.TaskItemStatePending,
	}

	if err := handler.SaveTaskItem(taskItem); err != nil {
		t.Fatalf("SaveTaskItem() error = %v", err)
	}

	if taskItem.ID == "" {
		t.Fatal("expected task item ID to be set after insert")
	}
	if taskItem.CreatedAt.IsZero() {
		t.Fatal("expected created_at to be set after insert")
	}
	if taskItem.UpdatedAt.IsZero() {
		t.Fatal("expected updated_at to be set after insert")
	}
}

func TestTaskItemsRepositoryHandler_SaveTaskItem_Update(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	task := createTestTask(t, NewTasksRepositoryHandler(context.Background(), pool, nil), goal.ID)
	handler := NewTaskItemsRepositoryHandler(context.Background(), pool, nil)

	taskItem := mustSaveTaskItem(t, handler, task.ID, "Bolts", sirkel_domain.TaskItemStatePending)

	taskItem.Name = "Bolts v2"
	taskItem.State = sirkel_domain.TaskItemStateInProgress

	if err := handler.SaveTaskItem(taskItem); err != nil {
		t.Fatalf("SaveTaskItem() update error = %v", err)
	}

	fetched, err := handler.GetTaskItemByID(&taskItem.ID)
	if err != nil {
		t.Fatalf("GetTaskItemByID() error = %v", err)
	}
	if fetched == nil {
		t.Fatal("expected task item to exist after update")
	}
	if fetched.Name != "Bolts v2" {
		t.Fatalf("expected name %q, got %q", "Bolts v2", fetched.Name)
	}
	if fetched.State != sirkel_domain.TaskItemStateInProgress {
		t.Fatalf("expected state %q, got %q", sirkel_domain.TaskItemStateInProgress, fetched.State)
	}
}

func TestTaskItemsRepositoryHandler_SaveTaskItem_NullDescription(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	task := createTestTask(t, NewTasksRepositoryHandler(context.Background(), pool, nil), goal.ID)
	handler := NewTaskItemsRepositoryHandler(context.Background(), pool, nil)

	taskItem := &sirkel_domain.TaskItem{
		TaskID: task.ID,
		Name:   "No description",
		State:  sirkel_domain.TaskItemStatePending,
	}

	if err := handler.SaveTaskItem(taskItem); err != nil {
		t.Fatalf("SaveTaskItem() error = %v", err)
	}

	fetched, err := handler.GetTaskItemByID(&taskItem.ID)
	if err != nil {
		t.Fatalf("GetTaskItemByID() error = %v", err)
	}
	if fetched == nil {
		t.Fatal("expected task item to exist")
	}
	if fetched.Description != nil {
		t.Fatalf("expected nil description, got %v", *fetched.Description)
	}
}

func TestTaskItemsRepositoryHandler_GetTaskItemByID_NotFound(t *testing.T) {
	pool := testPool(t)
	handler := NewTaskItemsRepositoryHandler(context.Background(), pool, nil)

	missingID := "00000000-0000-0000-0000-000000000000"
	taskItem, err := handler.GetTaskItemByID(&missingID)
	if err != nil {
		t.Fatalf("GetTaskItemByID() error = %v", err)
	}
	if taskItem != nil {
		t.Fatalf("expected nil task item, got %+v", taskItem)
	}
}

func TestTaskItemsRepositoryHandler_GetTaskItems_FiltersByTask(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	tasksHandler := NewTasksRepositoryHandler(context.Background(), pool, nil)
	taskA := createTestTask(t, tasksHandler, goal.ID)
	taskB := createTestTask(t, tasksHandler, goal.ID)
	handler := NewTaskItemsRepositoryHandler(context.Background(), pool, nil)

	mustSaveTaskItem(t, handler, taskA.ID, "A1", sirkel_domain.TaskItemStatePending)
	mustSaveTaskItem(t, handler, taskA.ID, "A2", sirkel_domain.TaskItemStatePending)
	mustSaveTaskItem(t, handler, taskB.ID, "B1", sirkel_domain.TaskItemStatePending)

	taskItems, err := handler.GetTaskItems(&GetTaskItemsParams{TaskID: &taskA.ID})
	if err != nil {
		t.Fatalf("GetTaskItems() error = %v", err)
	}
	if len(taskItems) != 2 {
		t.Fatalf("expected 2 task items for task A, got %d", len(taskItems))
	}
	for _, item := range taskItems {
		if item.TaskID != taskA.ID {
			t.Fatalf("expected task item to belong to task %q, got %q", taskA.ID, item.TaskID)
		}
	}
}

func TestTaskItemsRepositoryHandler_GetTaskItems_Pagination(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	task := createTestTask(t, NewTasksRepositoryHandler(context.Background(), pool, nil), goal.ID)
	handler := NewTaskItemsRepositoryHandler(context.Background(), pool, nil)

	for i := range 3 {
		mustSaveTaskItem(t, handler, task.ID, fmt.Sprintf("Item %d", i), sirkel_domain.TaskItemStatePending)
	}

	limit := 2
	offset := 1
	taskItems, err := handler.GetTaskItems(&GetTaskItemsParams{
		TaskID: &task.ID,
		Limit:  &limit,
		Offset: &offset,
	})
	if err != nil {
		t.Fatalf("GetTaskItems() error = %v", err)
	}
	if len(taskItems) != 2 {
		t.Fatalf("expected 2 task items with limit/offset, got %d", len(taskItems))
	}
}

func TestTaskItemsRepositoryHandler_CountTaskItems(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	tasksHandler := NewTasksRepositoryHandler(context.Background(), pool, nil)
	taskA := createTestTask(t, tasksHandler, goal.ID)
	taskB := createTestTask(t, tasksHandler, goal.ID)
	handler := NewTaskItemsRepositoryHandler(context.Background(), pool, nil)

	mustSaveTaskItem(t, handler, taskA.ID, "A1", sirkel_domain.TaskItemStatePending)
	mustSaveTaskItem(t, handler, taskA.ID, "A2", sirkel_domain.TaskItemStatePending)
	mustSaveTaskItem(t, handler, taskB.ID, "B1", sirkel_domain.TaskItemStatePending)

	count, err := handler.CountTaskItems(&CountTaskItemsParams{TaskID: &taskA.ID})
	if err != nil {
		t.Fatalf("CountTaskItems() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}

	total, err := handler.CountTaskItems(&CountTaskItemsParams{})
	if err != nil {
		t.Fatalf("CountTaskItems() error = %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total count 3, got %d", total)
	}
}

func TestTaskItemsRepositoryHandler_DeleteTaskItem(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	task := createTestTask(t, NewTasksRepositoryHandler(context.Background(), pool, nil), goal.ID)
	handler := NewTaskItemsRepositoryHandler(context.Background(), pool, nil)

	taskItem := mustSaveTaskItem(t, handler, task.ID, "To delete", sirkel_domain.TaskItemStatePending)

	if err := handler.DeleteTaskItem(&taskItem.ID); err != nil {
		t.Fatalf("DeleteTaskItem() error = %v", err)
	}

	fetched, err := handler.GetTaskItemByID(&taskItem.ID)
	if err != nil {
		t.Fatalf("GetTaskItemByID() error = %v", err)
	}
	if fetched != nil {
		t.Fatalf("expected task item to be deleted, got %+v", fetched)
	}
}
