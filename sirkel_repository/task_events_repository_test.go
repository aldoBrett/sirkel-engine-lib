package sirkel_repository

import (
	"context"
	"testing"

	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

type recordedEvent struct {
	TaskID     string
	TaskItemID *string
	Field      string
	OldValue   *string
	NewValue   *string
	ChangedBy  *string
}

func getTaskEvents(t *testing.T, pool *pgxpool.Pool, taskID string) []recordedEvent {
	t.Helper()

	rows, err := pool.Query(context.Background(), `
		SELECT task_id, task_item_id, field, old_value, new_value, changed_by
		FROM sirkel_engine.task_events
		WHERE task_id = $1
		ORDER BY id
	`, taskID)
	if err != nil {
		t.Fatalf("unable to query task events: %v", err)
	}
	defer rows.Close()

	var events []recordedEvent
	for rows.Next() {
		var event recordedEvent
		if err := rows.Scan(&event.TaskID, &event.TaskItemID, &event.Field, &event.OldValue, &event.NewValue, &event.ChangedBy); err != nil {
			t.Fatalf("unable to scan task event: %v", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("unable to read task events: %v", err)
	}

	return events
}

func describeValue(value *string) string {
	if value == nil {
		return "<nil>"
	}

	return *value
}

func assertEvent(t *testing.T, event recordedEvent, field string, oldValue, newValue *string, changedBy string) {
	t.Helper()

	if event.Field != field {
		t.Fatalf("expected event for field %q, got %q", field, event.Field)
	}
	if !equalStrings(event.OldValue, oldValue) {
		t.Fatalf("field %q: expected old value %s, got %s", field, describeValue(oldValue), describeValue(event.OldValue))
	}
	if !equalStrings(event.NewValue, newValue) {
		t.Fatalf("field %q: expected new value %s, got %s", field, describeValue(newValue), describeValue(event.NewValue))
	}
	assertUserID(t, "changed_by", event.ChangedBy, changedBy)
}

func assertEventCount(t *testing.T, events []recordedEvent, want int) {
	t.Helper()

	if len(events) != want {
		t.Fatalf("expected %d events, got %d: %+v", want, len(events), events)
	}
}

func TestTasksRepositoryHandler_SaveTask_RecordsCreationEvents(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	creator := createTestUser(t, pool, organizationID)
	responsible := createTestUser(t, pool, organizationID)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	handler := NewTasksRepositoryHandler(context.Background(), pool, creator)

	task := &sirkel_domain.Task{
		ID:                testUUID(t),
		GoalID:            goal.ID,
		Name:              "Assembly",
		State:             sirkel_domain.TaskStateTodo,
		ResponsibleUserID: &responsible.ID,
	}
	if err := handler.SaveTask(task); err != nil {
		t.Fatalf("SaveTask() error = %v", err)
	}

	events := getTaskEvents(t, pool, task.ID)
	assertEventCount(t, events, 2)
	assertEvent(t, events[0], "state", nil, strPtr("todo"), creator.ID)
	assertEvent(t, events[1], "responsible_user_id", nil, &responsible.ID, creator.ID)
	for _, event := range events {
		if event.TaskItemID != nil {
			t.Fatalf("expected task events to have no task item, got %q", *event.TaskItemID)
		}
	}
}

func TestTasksRepositoryHandler_SaveTask_RecordsOnlyStateOnCreationWithoutResponsible(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	creator := createTestUser(t, pool, organizationID)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)

	task := mustSaveTask(t, NewTasksRepositoryHandler(context.Background(), pool, creator), goal.ID, "Assembly", sirkel_domain.TaskStateTodo)

	events := getTaskEvents(t, pool, task.ID)
	assertEventCount(t, events, 1)
	assertEvent(t, events[0], "state", nil, strPtr("todo"), creator.ID)
}

func TestTasksRepositoryHandler_SaveTask_RecordsEditedFields(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	creator := createTestUser(t, pool, organizationID)
	editor := createTestUser(t, pool, organizationID)
	responsible := createTestUser(t, pool, organizationID)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)

	task := &sirkel_domain.Task{
		ID:     testUUID(t),
		GoalID: goal.ID,
		Name:   "Assembly",
		State:  sirkel_domain.TaskStateTodo,
	}
	if err := NewTasksRepositoryHandler(context.Background(), pool, creator).SaveTask(task); err != nil {
		t.Fatalf("SaveTask() error = %v", err)
	}

	description := "Assemble the rocket"
	task.Name = "Assembly v2"
	task.Description = &description
	task.State = sirkel_domain.TaskStateInProgress
	task.ResponsibleUserID = &responsible.ID
	if err := NewTasksRepositoryHandler(context.Background(), pool, editor).SaveTask(task); err != nil {
		t.Fatalf("SaveTask() update error = %v", err)
	}

	events := getTaskEvents(t, pool, task.ID)
	assertEventCount(t, events, 5)
	assertEvent(t, events[0], "state", nil, strPtr("todo"), creator.ID)
	assertEvent(t, events[1], "name", strPtr("Assembly"), strPtr("Assembly v2"), editor.ID)
	assertEvent(t, events[2], "description", nil, &description, editor.ID)
	assertEvent(t, events[3], "state", strPtr("todo"), strPtr("in-progress"), editor.ID)
	assertEvent(t, events[4], "responsible_user_id", nil, &responsible.ID, editor.ID)

	task.ResponsibleUserID = nil
	task.Description = nil
	if err := NewTasksRepositoryHandler(context.Background(), pool, editor).SaveTask(task); err != nil {
		t.Fatalf("SaveTask() clear error = %v", err)
	}

	events = getTaskEvents(t, pool, task.ID)
	assertEventCount(t, events, 7)
	assertEvent(t, events[5], "description", &description, nil, editor.ID)
	assertEvent(t, events[6], "responsible_user_id", &responsible.ID, nil, editor.ID)
}

func TestTasksRepositoryHandler_SaveTask_RecordsNothingWhenUnchanged(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	creator := createTestUser(t, pool, organizationID)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	handler := NewTasksRepositoryHandler(context.Background(), pool, creator)

	task := mustSaveTask(t, handler, goal.ID, "Assembly", sirkel_domain.TaskStateTodo)
	if err := handler.SaveTask(task); err != nil {
		t.Fatalf("SaveTask() second save error = %v", err)
	}

	assertEventCount(t, getTaskEvents(t, pool, task.ID), 1)
}

func TestTasksRepositoryHandler_SaveTask_RecordsEventsWithoutActor(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)

	task := mustSaveTask(t, NewTasksRepositoryHandler(context.Background(), pool, nil), goal.ID, "Assembly", sirkel_domain.TaskStateTodo)

	events := getTaskEvents(t, pool, task.ID)
	assertEventCount(t, events, 1)
	if events[0].ChangedBy != nil {
		t.Fatalf("expected no changed_by, got %q", *events[0].ChangedBy)
	}
}

func TestTasksRepositoryHandler_DeleteTask_RemovesEvents(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	handler := NewTasksRepositoryHandler(context.Background(), pool, nil)
	itemsHandler := NewTaskItemsRepositoryHandler(context.Background(), pool, nil)

	task := mustSaveTask(t, handler, goal.ID, "Assembly", sirkel_domain.TaskStateTodo)
	mustSaveTaskItem(t, itemsHandler, task.ID, "Bolt", sirkel_domain.TaskItemStatePending)
	assertEventCount(t, getTaskEvents(t, pool, task.ID), 2)

	if err := handler.DeleteTask(&task.ID); err != nil {
		t.Fatalf("DeleteTask() error = %v", err)
	}

	assertEventCount(t, getTaskEvents(t, pool, task.ID), 0)
}

func TestTaskItemsRepositoryHandler_SaveTaskItem_RecordsCreationAndEditedFields(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	creator := createTestUser(t, pool, organizationID)
	editor := createTestUser(t, pool, organizationID)
	assignee := createTestUser(t, pool, organizationID)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	task := createTestTask(t, NewTasksRepositoryHandler(context.Background(), pool, nil), goal.ID)

	taskItem := &sirkel_domain.TaskItem{
		ID:     testUUID(t),
		TaskID: task.ID,
		Name:   "Bolt",
		State:  sirkel_domain.TaskItemStatePending,
	}
	if err := NewTaskItemsRepositoryHandler(context.Background(), pool, creator).SaveTaskItem(taskItem); err != nil {
		t.Fatalf("SaveTaskItem() error = %v", err)
	}

	taskItem.Name = "Bolt v2"
	taskItem.State = sirkel_domain.TaskItemStateInProgress
	taskItem.AssignedUserID = &assignee.ID
	if err := NewTaskItemsRepositoryHandler(context.Background(), pool, editor).SaveTaskItem(taskItem); err != nil {
		t.Fatalf("SaveTaskItem() update error = %v", err)
	}

	// The task itself was created without an actor, so its own creation event comes first.
	events := getTaskEvents(t, pool, task.ID)
	assertEventCount(t, events, 5)
	taskItemEvents := events[1:]
	assertEvent(t, taskItemEvents[0], "state", nil, strPtr("pending"), creator.ID)
	assertEvent(t, taskItemEvents[1], "name", strPtr("Bolt"), strPtr("Bolt v2"), editor.ID)
	assertEvent(t, taskItemEvents[2], "state", strPtr("pending"), strPtr("in-progress"), editor.ID)
	assertEvent(t, taskItemEvents[3], "assigned_user_id", nil, &assignee.ID, editor.ID)
	for _, event := range taskItemEvents {
		if event.TaskItemID == nil || *event.TaskItemID != taskItem.ID {
			t.Fatalf("expected event for task item %q, got %v", taskItem.ID, event.TaskItemID)
		}
	}

	if err := NewTaskItemsRepositoryHandler(context.Background(), pool, editor).SaveTaskItem(taskItem); err != nil {
		t.Fatalf("SaveTaskItem() unchanged error = %v", err)
	}
	assertEventCount(t, getTaskEvents(t, pool, task.ID), 5)
}

func TestTaskItemsRepositoryHandler_SaveTaskItem_RecordsCreationWithAssignee(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	creator := createTestUser(t, pool, organizationID)
	assignee := createTestUser(t, pool, organizationID)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	goal := createTestGoal(t, NewGoalsRepositoryHandler(context.Background(), pool, nil), project.ID)
	task := createTestTask(t, NewTasksRepositoryHandler(context.Background(), pool, nil), goal.ID)

	taskItem := &sirkel_domain.TaskItem{
		ID:             testUUID(t),
		TaskID:         task.ID,
		Name:           "Bolt",
		State:          sirkel_domain.TaskItemStatePending,
		AssignedUserID: &assignee.ID,
	}
	if err := NewTaskItemsRepositoryHandler(context.Background(), pool, creator).SaveTaskItem(taskItem); err != nil {
		t.Fatalf("SaveTaskItem() error = %v", err)
	}

	events := getTaskEvents(t, pool, task.ID)
	assertEventCount(t, events, 3)
	assertEvent(t, events[1], "state", nil, strPtr("pending"), creator.ID)
	assertEvent(t, events[2], "assigned_user_id", nil, &assignee.ID, creator.ID)
}
