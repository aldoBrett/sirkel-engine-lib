package sirkel_domain

import "time"

type TaskStateCount struct {
	State TaskState `json:"state"`
	Count int       `json:"count"`
}

type GoalProgress struct {
	GoalID              string               `json:"goal_id"`
	GoalName            string               `json:"goal_name"`
	TaskItemStateCounts []TaskItemStateCount `json:"task_item_state_counts"`
	// TotalItems leaves out cancelled items, so PercentDone is done over what is left to do.
	TotalItems  int     `json:"total_items"`
	DoneItems   int     `json:"done_items"`
	PercentDone float64 `json:"percent_done"`
}

type StaleTask struct {
	Task Task `json:"task"`
	// LastActivityAt is the latest update of the task or any of its items.
	LastActivityAt time.Time `json:"last_activity_at"`
}

// OpenTask is a task that is not done or cancelled, with where it lives and the progress of its items.
type OpenTask struct {
	Task        Task   `json:"task"`
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	GoalName    string `json:"goal_name"`
	// Responsible is true when the requested user is responsible for the task. It is always false without a user.
	Responsible bool `json:"responsible"`
	// TaskItemStateCounts covers every item of the task, whoever they are assigned to.
	TaskItemStateCounts []TaskItemStateCount `json:"task_item_state_counts"`
	// OpenItems are the pending and in-progress items. With a user, all of them when they are responsible for the task,
	// and only the ones assigned to them when they are not.
	OpenItems []TaskItem `json:"open_items"`
}

type UserWorkload struct {
	UserID            string               `json:"user_id"`
	Email             string               `json:"email"`
	ResponsibleTasks  []TaskStateCount     `json:"responsible_tasks"`
	AssignedTaskItems []TaskItemStateCount `json:"assigned_task_items"`
}

type CompletedTasksBucket struct {
	PeriodStart time.Time `json:"period_start"`
	Count       int       `json:"count"`
}

// TaskCycleTimeStats describes the tasks that are done and were completed in the requested period.
// Lead time runs from the task's creation to its completion. Cycle time runs from the first time it
// went in progress to its completion, and is only known for tasks that went through in-progress
// after the history started being recorded.
type TaskCycleTimeStats struct {
	CompletedTasks      int     `json:"completed_tasks"`
	AverageLeadSeconds  float64 `json:"average_lead_seconds"`
	MedianLeadSeconds   float64 `json:"median_lead_seconds"`
	TasksWithCycleTime  int     `json:"tasks_with_cycle_time"`
	AverageCycleSeconds float64 `json:"average_cycle_seconds"`
	MedianCycleSeconds  float64 `json:"median_cycle_seconds"`
}
