package sirkel_analytics

import (
	"context"
	"fmt"
	"time"

	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultStaleAfter is how long a task can go without activity before it is considered stale.
const DefaultStaleAfter = 7 * 24 * time.Hour

type Interval string

const (
	IntervalDay   Interval = "day"
	IntervalWeek  Interval = "week"
	IntervalMonth Interval = "month"
)

// Every analytic is limited to the organization the user is acting in, and can be narrowed to a project or goal.
// Tasks without a goal don't belong to any organization, so they are left out.

type CountTasksByStateParams struct {
	ProjectID *string
	GoalID    *string
}

type GoalsProgressParams struct {
	ProjectID *string
}

type OpenTasksParams struct {
	ProjectID *string
	GoalID    *string
	// UserID limits the tasks to the ones the user is responsible for or has open items assigned in.
	UserID *string
	Offset *int
	Limit  *int
}

type StaleTasksParams struct {
	ProjectID *string
	GoalID    *string
	// StaleAfter defaults to DefaultStaleAfter.
	StaleAfter time.Duration
	Offset     *int
	Limit      *int
}

type UsersWorkloadParams struct {
	ProjectID *string
	GoalID    *string
}

type CompletedTasksOverTimeParams struct {
	ProjectID *string
	GoalID    *string
	Interval  Interval
	// From is inclusive and To exclusive. Both are required.
	From time.Time
	To   time.Time
}

type TaskCycleTimeParams struct {
	ProjectID *string
	GoalID    *string
	// CompletedFrom is inclusive and CompletedTo exclusive. Either can be left out.
	CompletedFrom *time.Time
	CompletedTo   *time.Time
}

type TasksAnalytics interface {
	// CountTasksByState returns one count per task state, in workflow order, with zeros for empty states.
	CountTasksByState(params *CountTasksByStateParams) ([]sirkel_domain.TaskStateCount, error)
	// GetGoalsProgress returns the task item progress of each goal, including goals without items.
	GetGoalsProgress(params *GoalsProgressParams) ([]*sirkel_domain.GoalProgress, error)
	// GetOpenTasks returns the todo, in-progress, in-review and blocked tasks, blocked first and then by workflow order, latest updated first.
	// With a user it returns the tasks they are responsible for, with all their open items, and the tasks where they only have
	// pending or in-progress items assigned, with just those items.
	GetOpenTasks(params *OpenTasksParams) ([]*sirkel_domain.OpenTask, error)
	// GetStaleTasks returns the in-progress, in-review and blocked tasks without activity for StaleAfter, stalest first.
	GetStaleTasks(params *StaleTasksParams) ([]*sirkel_domain.StaleTask, error)
	// GetUsersWorkload returns every user of the organization with the tasks they are responsible for and the items assigned to them.
	GetUsersWorkload(params *UsersWorkloadParams) ([]*sirkel_domain.UserWorkload, error)
	// GetCompletedTasksOverTime counts the tasks that are done, by when they were completed. Empty periods are included.
	GetCompletedTasksOverTime(params *CompletedTasksOverTimeParams) ([]sirkel_domain.CompletedTasksBucket, error)
	// GetTaskCycleTime measures how long the completed tasks took, from the recorded task history.
	GetTaskCycleTime(params *TaskCycleTimeParams) (*sirkel_domain.TaskCycleTimeStats, error)
}

type TasksAnalyticsHandler struct {
	ctx  context.Context
	pool *pgxpool.Pool
	user *sirkel_domain.User
}

func NewTasksAnalyticsHandler(ctx context.Context, pool *pgxpool.Pool, user *sirkel_domain.User) *TasksAnalyticsHandler {
	return &TasksAnalyticsHandler{
		ctx:  ctx,
		pool: pool,
		user: user,
	}
}

const tasksInScope = `
	FROM sirkel_engine.tasks t
	JOIN sirkel_engine.goals g ON g.id = t.goal_id
	JOIN sirkel_engine.projects p ON p.id = g.project_id
`

// scopeConditions returns the WHERE conditions, and their arguments, for a query over tasks (t), goals (g) and projects (p).
func (h *TasksAnalyticsHandler) scopeConditions(projectID, goalID *string) (string, []any, error) {
	if h.user == nil || h.user.OrganizationID == "" {
		return "", nil, ErrOrganizationRequired
	}

	args := []any{h.user.OrganizationID}
	conditions := "p.organization_id = $1"

	if projectID != nil {
		args = append(args, *projectID)
		conditions += fmt.Sprintf(" AND p.id = $%d", len(args))
	}

	if goalID != nil {
		args = append(args, *goalID)
		conditions += fmt.Sprintf(" AND g.id = $%d", len(args))
	}

	return conditions, args, nil
}

func (h *TasksAnalyticsHandler) CountTasksByState(params *CountTasksByStateParams) ([]sirkel_domain.TaskStateCount, error) {
	if params == nil {
		params = &CountTasksByStateParams{}
	}

	conditions, args, err := h.scopeConditions(params.ProjectID, params.GoalID)
	if err != nil {
		return nil, err
	}

	rows, err := h.pool.Query(h.ctx, `SELECT t.state, count(*) `+tasksInScope+` WHERE `+conditions+` GROUP BY t.state`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[sirkel_domain.TaskState]int, len(sirkel_domain.TaskStates))
	for rows.Next() {
		var state sirkel_domain.TaskState
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return nil, err
		}
		counts[state] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return taskStateCounts(counts), nil
}

func (h *TasksAnalyticsHandler) GetGoalsProgress(params *GoalsProgressParams) ([]*sirkel_domain.GoalProgress, error) {
	if params == nil {
		params = &GoalsProgressParams{}
	}

	conditions, args, err := h.scopeConditions(params.ProjectID, nil)
	if err != nil {
		return nil, err
	}

	rows, err := h.pool.Query(h.ctx, `
		SELECT g.id, g.name, ti.state, count(ti.id)
		FROM sirkel_engine.goals g
		JOIN sirkel_engine.projects p ON p.id = g.project_id
		LEFT JOIN sirkel_engine.tasks t ON t.goal_id = g.id
		LEFT JOIN sirkel_engine.task_items ti ON ti.task_id = t.id
		WHERE `+conditions+`
		GROUP BY g.id, ti.state
		ORDER BY g.created_at DESC, g.id
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var goals []*sirkel_domain.GoalProgress
	countsByGoal := map[string]map[sirkel_domain.TaskItemState]int{}
	for rows.Next() {
		var goalID, goalName string
		var state *sirkel_domain.TaskItemState
		var count int
		if err := rows.Scan(&goalID, &goalName, &state, &count); err != nil {
			return nil, err
		}

		counts, seen := countsByGoal[goalID]
		if !seen {
			counts = map[sirkel_domain.TaskItemState]int{}
			countsByGoal[goalID] = counts
			goals = append(goals, &sirkel_domain.GoalProgress{GoalID: goalID, GoalName: goalName})
		}
		if state != nil {
			counts[*state] = count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, goal := range goals {
		counts := countsByGoal[goal.GoalID]
		goal.TaskItemStateCounts = taskItemStateCounts(counts)
		goal.DoneItems = counts[sirkel_domain.TaskItemStateDone]
		goal.TotalItems = goal.DoneItems + counts[sirkel_domain.TaskItemStatePending] + counts[sirkel_domain.TaskItemStateInProgress]
		if goal.TotalItems > 0 {
			goal.PercentDone = float64(goal.DoneItems) / float64(goal.TotalItems) * 100
		}
	}

	return goals, nil
}

// openTaskStates are the task states where there is still work to do, and openItemStates the same for task items.
const (
	openTaskStates = `('todo', 'in-progress', 'in-review', 'blocked')`
	openItemStates = `('pending', 'in-progress')`
)

func (h *TasksAnalyticsHandler) GetOpenTasks(params *OpenTasksParams) ([]*sirkel_domain.OpenTask, error) {
	if params == nil {
		params = &OpenTasksParams{}
	}

	conditions, args, err := h.scopeConditions(params.ProjectID, params.GoalID)
	if err != nil {
		return nil, err
	}

	conditions += " AND t.state IN " + openTaskStates
	if params.UserID != nil {
		args = append(args, *params.UserID)
		conditions += fmt.Sprintf(` AND (t.responsible_user_id = $%[1]d OR EXISTS (
			SELECT 1 FROM sirkel_engine.task_items ai
			WHERE ai.task_id = t.id AND ai.assigned_user_id = $%[1]d AND ai.state IN `+openItemStates+`
		))`, len(args))
	}

	query := `
		SELECT t.id, t.goal_id, t.name, t.description, t.state, t.sort_key, t.responsible_user_id, t.created_by, t.updated_by, t.created_at, t.updated_at,
			p.id, p.name, g.name
		` + tasksInScope + `
		WHERE ` + conditions + `
		ORDER BY CASE t.state WHEN 'blocked' THEN 0 WHEN 'in-progress' THEN 1 WHEN 'in-review' THEN 2 ELSE 3 END, t.updated_at DESC, t.id
	`

	if params.Limit != nil {
		args = append(args, *params.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	if params.Offset != nil {
		args = append(args, *params.Offset)
		query += fmt.Sprintf(" OFFSET $%d", len(args))
	}

	rows, err := h.pool.Query(h.ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var openTasks []*sirkel_domain.OpenTask
	taskIDs := []string{}
	byID := map[string]*sirkel_domain.OpenTask{}
	for rows.Next() {
		openTask := &sirkel_domain.OpenTask{}
		task := &openTask.Task
		if err := rows.Scan(
			&task.ID,
			&task.GoalID,
			&task.Name,
			&task.Description,
			&task.State,
			&task.SortKey,
			&task.ResponsibleUserID,
			&task.CreatedBy,
			&task.UpdatedBy,
			&task.CreatedAt,
			&task.UpdatedAt,
			&openTask.ProjectID,
			&openTask.ProjectName,
			&openTask.GoalName,
		); err != nil {
			return nil, err
		}
		openTask.Responsible = params.UserID != nil && task.ResponsibleUserID != nil && *task.ResponsibleUserID == *params.UserID
		openTask.OpenItems = []sirkel_domain.TaskItem{}

		openTasks = append(openTasks, openTask)
		taskIDs = append(taskIDs, task.ID)
		byID[task.ID] = openTask
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	if len(openTasks) == 0 {
		return openTasks, nil
	}

	countRows, err := h.pool.Query(h.ctx, `
		SELECT task_id, state, count(*) FROM sirkel_engine.task_items
		WHERE task_id = ANY($1::uuid[])
		GROUP BY task_id, state
	`, taskIDs)
	if err != nil {
		return nil, err
	}
	defer countRows.Close()

	countsByTask := map[string]map[sirkel_domain.TaskItemState]int{}
	for countRows.Next() {
		var taskID string
		var state sirkel_domain.TaskItemState
		var count int
		if err := countRows.Scan(&taskID, &state, &count); err != nil {
			return nil, err
		}
		if countsByTask[taskID] == nil {
			countsByTask[taskID] = map[sirkel_domain.TaskItemState]int{}
		}
		countsByTask[taskID][state] = count
	}
	if err := countRows.Err(); err != nil {
		return nil, err
	}
	countRows.Close()

	for _, openTask := range openTasks {
		openTask.TaskItemStateCounts = taskItemStateCounts(countsByTask[openTask.Task.ID])
	}

	// On a task the user is not responsible for, they only see their own items.
	itemQuery := `
		SELECT ti.id, ti.task_id, ti.name, ti.description, ti.state, ti.sort_key, ti.assigned_user_id, ti.created_by, ti.updated_by, ti.created_at, ti.updated_at
		FROM sirkel_engine.task_items ti
		JOIN sirkel_engine.tasks t ON t.id = ti.task_id
		WHERE ti.task_id = ANY($1::uuid[]) AND ti.state IN ` + openItemStates
	itemArgs := []any{taskIDs}
	if params.UserID != nil {
		itemQuery += " AND (t.responsible_user_id = $2 OR ti.assigned_user_id = $2)"
		itemArgs = append(itemArgs, *params.UserID)
	}
	itemQuery += " ORDER BY ti.sort_key, ti.id"

	itemRows, err := h.pool.Query(h.ctx, itemQuery, itemArgs...)
	if err != nil {
		return nil, err
	}
	defer itemRows.Close()

	for itemRows.Next() {
		var item sirkel_domain.TaskItem
		if err := itemRows.Scan(
			&item.ID,
			&item.TaskID,
			&item.Name,
			&item.Description,
			&item.State,
			&item.SortKey,
			&item.AssignedUserID,
			&item.CreatedBy,
			&item.UpdatedBy,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		byID[item.TaskID].OpenItems = append(byID[item.TaskID].OpenItems, item)
	}

	return openTasks, itemRows.Err()
}

func (h *TasksAnalyticsHandler) GetStaleTasks(params *StaleTasksParams) ([]*sirkel_domain.StaleTask, error) {
	if params == nil {
		params = &StaleTasksParams{}
	}

	conditions, args, err := h.scopeConditions(params.ProjectID, params.GoalID)
	if err != nil {
		return nil, err
	}

	staleAfter := params.StaleAfter
	if staleAfter <= 0 {
		staleAfter = DefaultStaleAfter
	}
	args = append(args, staleAfter.Seconds())

	query := `
		SELECT t.id, t.goal_id, t.name, t.description, t.state, t.responsible_user_id, t.created_by, t.updated_by, t.created_at, t.updated_at,
			GREATEST(t.updated_at, MAX(ti.updated_at)) AS last_activity_at
		` + tasksInScope + `
		LEFT JOIN sirkel_engine.task_items ti ON ti.task_id = t.id
		WHERE ` + conditions + ` AND t.state IN ('in-progress', 'in-review', 'blocked')
		GROUP BY t.id
		HAVING GREATEST(t.updated_at, MAX(ti.updated_at)) < now() - make_interval(secs => $` + fmt.Sprint(len(args)) + `)
		ORDER BY last_activity_at ASC, t.id
	`

	if params.Limit != nil {
		args = append(args, *params.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	if params.Offset != nil {
		args = append(args, *params.Offset)
		query += fmt.Sprintf(" OFFSET $%d", len(args))
	}

	rows, err := h.pool.Query(h.ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var staleTasks []*sirkel_domain.StaleTask
	for rows.Next() {
		staleTask := &sirkel_domain.StaleTask{}
		task := &staleTask.Task
		if err := rows.Scan(
			&task.ID,
			&task.GoalID,
			&task.Name,
			&task.Description,
			&task.State,
			&task.ResponsibleUserID,
			&task.CreatedBy,
			&task.UpdatedBy,
			&task.CreatedAt,
			&task.UpdatedAt,
			&staleTask.LastActivityAt,
		); err != nil {
			return nil, err
		}
		staleTasks = append(staleTasks, staleTask)
	}

	return staleTasks, rows.Err()
}

func (h *TasksAnalyticsHandler) GetUsersWorkload(params *UsersWorkloadParams) ([]*sirkel_domain.UserWorkload, error) {
	if params == nil {
		params = &UsersWorkloadParams{}
	}

	conditions, args, err := h.scopeConditions(params.ProjectID, params.GoalID)
	if err != nil {
		return nil, err
	}

	userRows, err := h.pool.Query(h.ctx, `SELECT id, email FROM sirkel_engine.users WHERE organization_id = $1 ORDER BY email, id`, args[0])
	if err != nil {
		return nil, err
	}
	defer userRows.Close()

	var users []*sirkel_domain.UserWorkload
	for userRows.Next() {
		user := &sirkel_domain.UserWorkload{}
		if err := userRows.Scan(&user.UserID, &user.Email); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := userRows.Err(); err != nil {
		return nil, err
	}

	tasksByUser, err := h.countByUserAndState(`
		SELECT t.responsible_user_id, t.state, count(*)
		`+tasksInScope+`
		WHERE `+conditions+` AND t.responsible_user_id IS NOT NULL
		GROUP BY t.responsible_user_id, t.state
	`, args)
	if err != nil {
		return nil, err
	}

	itemsByUser, err := h.countByUserAndState(`
		SELECT ti.assigned_user_id, ti.state, count(*)
		`+tasksInScope+`
		JOIN sirkel_engine.task_items ti ON ti.task_id = t.id
		WHERE `+conditions+` AND ti.assigned_user_id IS NOT NULL
		GROUP BY ti.assigned_user_id, ti.state
	`, args)
	if err != nil {
		return nil, err
	}

	for _, user := range users {
		taskCounts := make(map[sirkel_domain.TaskState]int, len(sirkel_domain.TaskStates))
		for state, count := range tasksByUser[user.UserID] {
			taskCounts[sirkel_domain.TaskState(state)] = count
		}
		itemCounts := make(map[sirkel_domain.TaskItemState]int, len(sirkel_domain.TaskItemStates))
		for state, count := range itemsByUser[user.UserID] {
			itemCounts[sirkel_domain.TaskItemState(state)] = count
		}

		user.ResponsibleTasks = taskStateCounts(taskCounts)
		user.AssignedTaskItems = taskItemStateCounts(itemCounts)
	}

	return users, nil
}

// countByUserAndState runs a query returning (user id, state, count) rows.
func (h *TasksAnalyticsHandler) countByUserAndState(query string, args []any) (map[string]map[string]int, error) {
	rows, err := h.pool.Query(h.ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]map[string]int{}
	for rows.Next() {
		var userID, state string
		var count int
		if err := rows.Scan(&userID, &state, &count); err != nil {
			return nil, err
		}
		if counts[userID] == nil {
			counts[userID] = map[string]int{}
		}
		counts[userID][state] = count
	}

	return counts, rows.Err()
}

// completionsCTE finds when each task that is done was completed: the last time it moved to done.
// Tasks completed before the history started being recorded have no such event, so they are left out.
func completionsCTE(conditions string) string {
	return `completions AS (
		SELECT e.task_id, max(e.changed_at) AS completed_at
		FROM sirkel_engine.task_events e
		JOIN sirkel_engine.tasks t ON t.id = e.task_id AND t.state = 'done'
		JOIN sirkel_engine.goals g ON g.id = t.goal_id
		JOIN sirkel_engine.projects p ON p.id = g.project_id
		WHERE e.task_item_id IS NULL AND e.field = 'state' AND e.new_value = 'done' AND ` + conditions + `
		GROUP BY e.task_id
	)`
}

func (h *TasksAnalyticsHandler) GetCompletedTasksOverTime(params *CompletedTasksOverTimeParams) ([]sirkel_domain.CompletedTasksBucket, error) {
	if params == nil {
		params = &CompletedTasksOverTimeParams{}
	}

	switch params.Interval {
	case IntervalDay, IntervalWeek, IntervalMonth:
	default:
		return nil, ErrInvalidInterval
	}
	if params.From.IsZero() || params.To.IsZero() || !params.From.Before(params.To) {
		return nil, ErrInvalidRange
	}

	conditions, args, err := h.scopeConditions(params.ProjectID, params.GoalID)
	if err != nil {
		return nil, err
	}
	args = append(args, params.From, params.To)
	from, to := len(args)-1, len(args)

	// Interval is one of the values checked above, so it is safe to put in the query.
	unit := string(params.Interval)
	rows, err := h.pool.Query(h.ctx, fmt.Sprintf(`
		WITH %[1]s
		SELECT b.period_start, count(c.task_id)
		FROM generate_series(
			date_trunc('%[2]s', $%[3]d::timestamptz AT TIME ZONE 'UTC'),
			date_trunc('%[2]s', ($%[4]d::timestamptz - interval '1 microsecond') AT TIME ZONE 'UTC'),
			interval '1 %[2]s'
		) AS b(period_start)
		LEFT JOIN completions c
			ON c.completed_at >= $%[3]d AND c.completed_at < $%[4]d
			AND date_trunc('%[2]s', c.completed_at AT TIME ZONE 'UTC') = b.period_start
		GROUP BY b.period_start
		ORDER BY b.period_start
	`, completionsCTE(conditions), unit, from, to), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var buckets []sirkel_domain.CompletedTasksBucket
	for rows.Next() {
		var bucket sirkel_domain.CompletedTasksBucket
		if err := rows.Scan(&bucket.PeriodStart, &bucket.Count); err != nil {
			return nil, err
		}
		buckets = append(buckets, bucket)
	}

	return buckets, rows.Err()
}

func (h *TasksAnalyticsHandler) GetTaskCycleTime(params *TaskCycleTimeParams) (*sirkel_domain.TaskCycleTimeStats, error) {
	if params == nil {
		params = &TaskCycleTimeParams{}
	}

	conditions, args, err := h.scopeConditions(params.ProjectID, params.GoalID)
	if err != nil {
		return nil, err
	}

	measuredConditions := "TRUE"
	if params.CompletedFrom != nil {
		args = append(args, *params.CompletedFrom)
		measuredConditions += fmt.Sprintf(" AND c.completed_at >= $%d", len(args))
	}
	if params.CompletedTo != nil {
		args = append(args, *params.CompletedTo)
		measuredConditions += fmt.Sprintf(" AND c.completed_at < $%d", len(args))
	}

	stats := &sirkel_domain.TaskCycleTimeStats{}
	err = h.pool.QueryRow(h.ctx, `
		WITH `+completionsCTE(conditions)+`,
		started AS (
			SELECT task_id, min(changed_at) AS started_at
			FROM sirkel_engine.task_events
			WHERE task_item_id IS NULL AND field = 'state' AND new_value = 'in-progress'
			GROUP BY task_id
		),
		measured AS (
			SELECT extract(epoch FROM c.completed_at - t.created_at)::float8 AS lead_seconds,
				CASE WHEN s.started_at <= c.completed_at
					THEN extract(epoch FROM c.completed_at - s.started_at)::float8
				END AS cycle_seconds
			FROM completions c
			JOIN sirkel_engine.tasks t ON t.id = c.task_id
			LEFT JOIN started s ON s.task_id = c.task_id
			WHERE `+measuredConditions+`
		)
		SELECT count(*),
			COALESCE(avg(lead_seconds), 0)::float8,
			COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY lead_seconds), 0)::float8,
			count(cycle_seconds),
			COALESCE(avg(cycle_seconds), 0)::float8,
			COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY cycle_seconds), 0)::float8
		FROM measured
	`, args...).Scan(
		&stats.CompletedTasks,
		&stats.AverageLeadSeconds,
		&stats.MedianLeadSeconds,
		&stats.TasksWithCycleTime,
		&stats.AverageCycleSeconds,
		&stats.MedianCycleSeconds,
	)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

func taskStateCounts(counts map[sirkel_domain.TaskState]int) []sirkel_domain.TaskStateCount {
	stateCounts := make([]sirkel_domain.TaskStateCount, len(sirkel_domain.TaskStates))
	for i, state := range sirkel_domain.TaskStates {
		stateCounts[i] = sirkel_domain.TaskStateCount{State: state, Count: counts[state]}
	}

	return stateCounts
}

func taskItemStateCounts(counts map[sirkel_domain.TaskItemState]int) []sirkel_domain.TaskItemStateCount {
	stateCounts := make([]sirkel_domain.TaskItemStateCount, len(sirkel_domain.TaskItemStates))
	for i, state := range sirkel_domain.TaskItemStates {
		stateCounts[i] = sirkel_domain.TaskItemStateCount{State: state, Count: counts[state]}
	}

	return stateCounts
}
