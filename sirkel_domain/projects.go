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

type Project struct {
	ID             string       `json:"id"`
	OrganizationID string       `json:"organization_id"`
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	State          ProjectState `json:"state"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

type Goal struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	State       GoalState `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Task struct {
	ID          string    `json:"id"`
	GoalID      string    `json:"goal_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	State       TaskState `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TaskItem struct {
	ID          string        `json:"id"`
	TaskID      string        `json:"task_id"`
	Name        string        `json:"name"`
	Description *string       `json:"description,omitempty"`
	State       TaskItemState `json:"state"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}
