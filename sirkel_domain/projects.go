package sirkel_domain

import "time"

type ProjectState string

const (
	ProjectStatePlanning  ProjectState = "planning"
	ProjectStateActive    ProjectState = "active"
	ProjectStateOnHold    ProjectState = "on-hold"
	ProjectStateCompleted ProjectState = "completed"
	ProjectStateArchived  ProjectState = "archived"
)

type GoalState string

const (
	GoalStateActive    GoalState = "active"
	GoalStateAchieved  GoalState = "achieved"
	GoalStateOnHold    GoalState = "on-hold"
	GoalStateAbandoned GoalState = "abandoned"
)

type TaskState string

const (
	TaskStateTodo       TaskState = "todo"
	TaskStateInProgress TaskState = "in-progress"
	TaskStateBlocked    TaskState = "blocked"
	TaskStateDone       TaskState = "done"
	TaskStateCancelled  TaskState = "cancelled"
	TaskStateInReview   TaskState = "in-review"
)

type TaskItemState string

const (
	TaskItemStatePending    TaskItemState = "pending"
	TaskItemStateInProgress TaskItemState = "in-progress"
	TaskItemStateDone       TaskItemState = "done"
	TaskItemStateCancelled  TaskItemState = "cancelled"
)

// TaskEventField is a task or task item field whose changes are recorded in the task history.
type TaskEventField string

const (
	TaskEventFieldName              TaskEventField = "name"
	TaskEventFieldDescription       TaskEventField = "description"
	TaskEventFieldState             TaskEventField = "state"
	TaskEventFieldResponsibleUserID TaskEventField = "responsible_user_id"
	TaskEventFieldAssignedUserID    TaskEventField = "assigned_user_id"
)

// TaskStates lists every task state in workflow order.
var TaskStates = []TaskState{
	TaskStateTodo,
	TaskStateInProgress,
	TaskStateInReview,
	TaskStateBlocked,
	TaskStateDone,
	TaskStateCancelled,
}

// TaskItemStates lists every task item state in workflow order.
var TaskItemStates = []TaskItemState{
	TaskItemStatePending,
	TaskItemStateInProgress,
	TaskItemStateDone,
	TaskItemStateCancelled,
}

type Project struct {
	ID             string       `json:"id"`
	OrganizationID string       `json:"organization_id"`
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	State          ProjectState `json:"state"`
	CreatedBy      *string      `json:"created_by,omitempty"`
	UpdatedBy      *string      `json:"updated_by,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

type Goal struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	State       GoalState `json:"state"`
	CreatedBy   *string   `json:"created_by,omitempty"`
	UpdatedBy   *string   `json:"updated_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Task struct {
	ID                string    `json:"id"`
	GoalID            string    `json:"goal_id"`
	Name              string    `json:"name"`
	Description       *string   `json:"description,omitempty"`
	State             TaskState `json:"state"`
	ResponsibleUserID *string   `json:"responsible_user_id,omitempty"`
	CreatedBy         *string   `json:"created_by,omitempty"`
	UpdatedBy         *string   `json:"updated_by,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type TaskItem struct {
	ID             string        `json:"id"`
	TaskID         string        `json:"task_id"`
	Name           string        `json:"name"`
	Description    *string       `json:"description,omitempty"`
	State          TaskItemState `json:"state"`
	AssignedUserID *string       `json:"assigned_user_id,omitempty"`
	CreatedBy      *string       `json:"created_by,omitempty"`
	UpdatedBy      *string       `json:"updated_by,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type TaskItemStateCount struct {
	State TaskItemState `json:"state"`
	Count int           `json:"count"`
}

// TaskEvent is one recorded change of a task or, when TaskItemID is set, of one of its items.
// A nil OldValue means the field was created or had no value, and a nil NewValue means it was cleared.
type TaskEvent struct {
	ID         int64          `json:"id"`
	TaskID     string         `json:"task_id"`
	TaskItemID *string        `json:"task_item_id,omitempty"`
	Field      TaskEventField `json:"field"`
	OldValue   *string        `json:"old_value"`
	NewValue   *string        `json:"new_value"`
	ChangedBy  *string        `json:"changed_by,omitempty"`
	ChangedAt  time.Time      `json:"changed_at"`
}

type TaskForIndex struct {
	Task                Task                 `json:"task"`
	TaskItemStateCounts []TaskItemStateCount `json:"task_item_state_counts"`
}
