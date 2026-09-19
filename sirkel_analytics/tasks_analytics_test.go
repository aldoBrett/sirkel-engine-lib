package sirkel_analytics

import (
	"math"
	"testing"
	"time"

	"sirkel-engine-lib/sirkel_domain"
)

func assertTaskStateCounts(t *testing.T, got []sirkel_domain.TaskStateCount, todo, inProgress, inReview, blocked, done, cancelled int) {
	t.Helper()

	want := []sirkel_domain.TaskStateCount{
		{State: sirkel_domain.TaskStateTodo, Count: todo},
		{State: sirkel_domain.TaskStateInProgress, Count: inProgress},
		{State: sirkel_domain.TaskStateInReview, Count: inReview},
		{State: sirkel_domain.TaskStateBlocked, Count: blocked},
		{State: sirkel_domain.TaskStateDone, Count: done},
		{State: sirkel_domain.TaskStateCancelled, Count: cancelled},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d task state counts, got %d: %+v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("task state count %d: expected %+v, got %+v", i, want[i], got[i])
		}
	}
}

func assertTaskItemStateCounts(t *testing.T, got []sirkel_domain.TaskItemStateCount, pending, inProgress, done, cancelled int) {
	t.Helper()

	want := []sirkel_domain.TaskItemStateCount{
		{State: sirkel_domain.TaskItemStatePending, Count: pending},
		{State: sirkel_domain.TaskItemStateInProgress, Count: inProgress},
		{State: sirkel_domain.TaskItemStateDone, Count: done},
		{State: sirkel_domain.TaskItemStateCancelled, Count: cancelled},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d task item state counts, got %d: %+v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("task item state count %d: expected %+v, got %+v", i, want[i], got[i])
		}
	}
}

func assertSeconds(t *testing.T, name string, got, want float64) {
	t.Helper()

	if math.Abs(got-want) > 0.5 {
		t.Fatalf("expected %s %.0f seconds, got %.0f", name, want, got)
	}
}

func TestTasksAnalytics_CountTasksByState(t *testing.T) {
	pool := testPool(t)
	tn := newTenant(t, pool, "owner@a.test")
	other := newTenant(t, pool, "owner@b.test")

	projectA := tn.project("A")
	projectB := tn.project("B")
	goalA := tn.goal(projectA.ID, "Goal A")
	goalB := tn.goal(projectB.ID, "Goal B")
	tn.task(goalA.ID, "t1", sirkel_domain.TaskStateTodo)
	tn.task(goalA.ID, "t2", sirkel_domain.TaskStateTodo)
	tn.task(goalA.ID, "t3", sirkel_domain.TaskStateInProgress)
	tn.task(goalA.ID, "t4", sirkel_domain.TaskStateDone)
	tn.task(goalB.ID, "t5", sirkel_domain.TaskStateBlocked)

	otherGoal := other.goal(other.project("Other").ID, "Other goal")
	other.task(otherGoal.ID, "o1", sirkel_domain.TaskStateDone)

	counts, err := tn.analytics.Tasks.CountTasksByState(nil)
	if err != nil {
		t.Fatalf("CountTasksByState() error = %v", err)
	}
	assertTaskStateCounts(t, counts, 2, 1, 0, 1, 1, 0)

	counts, err = tn.analytics.Tasks.CountTasksByState(&CountTasksByStateParams{ProjectID: &projectA.ID})
	if err != nil {
		t.Fatalf("CountTasksByState() by project error = %v", err)
	}
	assertTaskStateCounts(t, counts, 2, 1, 0, 0, 1, 0)

	counts, err = tn.analytics.Tasks.CountTasksByState(&CountTasksByStateParams{GoalID: &goalB.ID})
	if err != nil {
		t.Fatalf("CountTasksByState() by goal error = %v", err)
	}
	assertTaskStateCounts(t, counts, 0, 0, 0, 1, 0, 0)

	// A goal of another organization is invisible.
	counts, err = other.analytics.Tasks.CountTasksByState(&CountTasksByStateParams{GoalID: &goalA.ID})
	if err != nil {
		t.Fatalf("CountTasksByState() other organization error = %v", err)
	}
	assertTaskStateCounts(t, counts, 0, 0, 0, 0, 0, 0)
}

func TestTasksAnalytics_RequireOrganization(t *testing.T) {
	pool := testPool(t)
	tn := newTenant(t, pool, "owner@a.test")
	noOrganization := &sirkel_domain.User{ID: tn.user.ID, Role: sirkel_domain.RoleSuperAdmin}
	from, to := date(2026, time.January, 1), date(2026, time.January, 8)

	for name, user := range map[string]*sirkel_domain.User{"nil user": nil, "no acting organization": noOrganization} {
		t.Run(name, func(t *testing.T) {
			handler := NewTasksAnalyticsHandler(t.Context(), pool, user)

			_, err := handler.CountTasksByState(nil)
			requireError(t, err, ErrOrganizationRequired)
			_, err = handler.GetGoalsProgress(nil)
			requireError(t, err, ErrOrganizationRequired)
			_, err = handler.GetStaleTasks(nil)
			requireError(t, err, ErrOrganizationRequired)
			_, err = handler.GetUsersWorkload(nil)
			requireError(t, err, ErrOrganizationRequired)
			_, err = handler.GetCompletedTasksOverTime(&CompletedTasksOverTimeParams{Interval: IntervalDay, From: from, To: to})
			requireError(t, err, ErrOrganizationRequired)
			_, err = handler.GetTaskCycleTime(nil)
			requireError(t, err, ErrOrganizationRequired)
		})
	}
}

func TestTasksAnalytics_GetGoalsProgress(t *testing.T) {
	pool := testPool(t)
	tn := newTenant(t, pool, "owner@a.test")
	other := newTenant(t, pool, "owner@b.test")

	projectA := tn.project("A")
	projectB := tn.project("B")
	goalWithItems := tn.goal(projectA.ID, "With items")
	goalWithoutItems := tn.goal(projectA.ID, "Without items")
	goalOfB := tn.goal(projectB.ID, "Of B")

	taskOne := tn.task(goalWithItems.ID, "t1", sirkel_domain.TaskStateInProgress)
	taskTwo := tn.task(goalWithItems.ID, "t2", sirkel_domain.TaskStateTodo)
	tn.taskItem(taskOne.ID, "i1", sirkel_domain.TaskItemStatePending, nil)
	tn.taskItem(taskOne.ID, "i2", sirkel_domain.TaskItemStateInProgress, nil)
	tn.taskItem(taskOne.ID, "i3", sirkel_domain.TaskItemStateDone, nil)
	tn.taskItem(taskTwo.ID, "i4", sirkel_domain.TaskItemStateDone, nil)
	tn.taskItem(taskTwo.ID, "i5", sirkel_domain.TaskItemStateCancelled, nil)

	otherGoal := other.goal(other.project("Other").ID, "Other goal")
	other.taskItem(other.task(otherGoal.ID, "o1", sirkel_domain.TaskStateTodo).ID, "oi", sirkel_domain.TaskItemStateDone, nil)

	goals, err := tn.analytics.Tasks.GetGoalsProgress(&GoalsProgressParams{ProjectID: &projectA.ID})
	if err != nil {
		t.Fatalf("GetGoalsProgress() error = %v", err)
	}
	if len(goals) != 2 {
		t.Fatalf("expected 2 goals, got %d: %+v", len(goals), goals)
	}

	// Most recent goal first.
	empty, withItems := goals[0], goals[1]
	if empty.GoalID != goalWithoutItems.ID || withItems.GoalID != goalWithItems.ID {
		t.Fatalf("unexpected goal order: %q, %q", empty.GoalID, withItems.GoalID)
	}
	if empty.GoalName != "Without items" {
		t.Fatalf("expected goal name %q, got %q", "Without items", empty.GoalName)
	}
	assertTaskItemStateCounts(t, empty.TaskItemStateCounts, 0, 0, 0, 0)
	if empty.TotalItems != 0 || empty.DoneItems != 0 || empty.PercentDone != 0 {
		t.Fatalf("expected no progress for a goal without items, got %+v", empty)
	}

	assertTaskItemStateCounts(t, withItems.TaskItemStateCounts, 1, 1, 2, 1)
	if withItems.TotalItems != 4 || withItems.DoneItems != 2 {
		t.Fatalf("expected 2 of 4 items done (cancelled left out), got %+v", withItems)
	}
	if withItems.PercentDone != 50 {
		t.Fatalf("expected 50%% done, got %v", withItems.PercentDone)
	}

	all, err := tn.analytics.Tasks.GetGoalsProgress(nil)
	if err != nil {
		t.Fatalf("GetGoalsProgress() without project error = %v", err)
	}
	if len(all) != 3 || all[0].GoalID != goalOfB.ID {
		t.Fatalf("expected the organization's 3 goals, newest first, got %+v", all)
	}
}

func TestTasksAnalytics_GetStaleTasks(t *testing.T) {
	pool := testPool(t)
	tn := newTenant(t, pool, "owner@a.test")
	other := newTenant(t, pool, "owner@b.test")
	goal := tn.goal(tn.project("A").ID, "Goal")
	otherGoal := other.goal(other.project("Other").ID, "Other goal")

	day := 24 * time.Hour
	blocked := tn.task(goal.ID, "blocked 20d", sirkel_domain.TaskStateBlocked)
	inProgress := tn.task(goal.ID, "in progress 10d", sirkel_domain.TaskStateInProgress)
	inReview := tn.task(goal.ID, "in review 8d", sirkel_domain.TaskStateInReview)
	tn.backdateTask(blocked.ID, 20*day)
	tn.backdateTask(inProgress.ID, 10*day)
	tn.backdateTask(inReview.ID, 8*day)

	// Not stale: recent, not started, finished, or old but with recent activity on its items.
	recent := tn.task(goal.ID, "recent", sirkel_domain.TaskStateInProgress)
	todo := tn.task(goal.ID, "todo 10d", sirkel_domain.TaskStateTodo)
	done := tn.task(goal.ID, "done 10d", sirkel_domain.TaskStateDone)
	active := tn.task(goal.ID, "old task with active item", sirkel_domain.TaskStateInProgress)
	tn.backdateTask(recent.ID, day)
	tn.backdateTask(todo.ID, 10*day)
	tn.backdateTask(done.ID, 10*day)
	tn.backdateTask(active.ID, 10*day)
	tn.taskItem(active.ID, "fresh item", sirkel_domain.TaskItemStateInProgress, nil)

	otherStale := other.task(otherGoal.ID, "other org", sirkel_domain.TaskStateBlocked)
	other.backdateTask(otherStale.ID, 30*day)

	stale, err := tn.analytics.Tasks.GetStaleTasks(nil)
	if err != nil {
		t.Fatalf("GetStaleTasks() error = %v", err)
	}
	if len(stale) != 3 {
		t.Fatalf("expected 3 stale tasks, got %d: %+v", len(stale), stale)
	}
	for i, want := range []*sirkel_domain.Task{blocked, inProgress, inReview} {
		if stale[i].Task.ID != want.ID {
			t.Fatalf("stale task %d: expected %q, got %q", i, want.Name, stale[i].Task.Name)
		}
	}
	if age := time.Since(stale[0].LastActivityAt); age < 20*day-time.Minute || age > 20*day+time.Minute {
		t.Fatalf("expected last activity 20 days ago, got %v ago", age)
	}

	stale, err = tn.analytics.Tasks.GetStaleTasks(&StaleTasksParams{StaleAfter: 15 * day})
	if err != nil {
		t.Fatalf("GetStaleTasks() custom threshold error = %v", err)
	}
	if len(stale) != 1 || stale[0].Task.ID != blocked.ID {
		t.Fatalf("expected only the blocked task past 15 days, got %+v", stale)
	}

	limit, offset := 1, 1
	stale, err = tn.analytics.Tasks.GetStaleTasks(&StaleTasksParams{Limit: &limit, Offset: &offset})
	if err != nil {
		t.Fatalf("GetStaleTasks() page error = %v", err)
	}
	if len(stale) != 1 || stale[0].Task.ID != inProgress.ID {
		t.Fatalf("expected the second stalest task, got %+v", stale)
	}

	stale, err = tn.analytics.Tasks.GetStaleTasks(&StaleTasksParams{GoalID: &otherGoal.ID})
	if err != nil {
		t.Fatalf("GetStaleTasks() other organization goal error = %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("expected no tasks for a goal of another organization, got %+v", stale)
	}
}

func TestTasksAnalytics_GetUsersWorkload(t *testing.T) {
	pool := testPool(t)
	tn := newTenant(t, pool, "owner@a.test")
	other := newTenant(t, pool, "owner@b.test")
	alice := createTestUser(t, pool, tn.organizationID, "alice@a.test")
	bob := createTestUser(t, pool, tn.organizationID, "bob@a.test")

	project := tn.project("A")
	goalOne := tn.goal(project.ID, "One")
	goalTwo := tn.goal(project.ID, "Two")

	responsible := func(goalID, name string, state sirkel_domain.TaskState, userID string) *sirkel_domain.Task {
		task := tn.task(goalID, name, state)
		task.ResponsibleUserID = &userID
		if err := tn.repositories.Tasks.SaveTask(task); err != nil {
			t.Fatalf("SaveTask() error = %v", err)
		}
		return task
	}
	aliceInProgress := responsible(goalOne.ID, "t1", sirkel_domain.TaskStateInProgress, alice.ID)
	responsible(goalOne.ID, "t2", sirkel_domain.TaskStateTodo, alice.ID)
	bobTask := responsible(goalTwo.ID, "t3", sirkel_domain.TaskStateDone, bob.ID)
	tn.task(goalOne.ID, "unassigned", sirkel_domain.TaskStateTodo)

	tn.taskItem(aliceInProgress.ID, "i1", sirkel_domain.TaskItemStatePending, &alice.ID)
	tn.taskItem(aliceInProgress.ID, "i2", sirkel_domain.TaskItemStateDone, &alice.ID)
	tn.taskItem(bobTask.ID, "i3", sirkel_domain.TaskItemStateInProgress, &bob.ID)
	tn.taskItem(bobTask.ID, "unassigned item", sirkel_domain.TaskItemStatePending, nil)

	other.goal(other.project("Other").ID, "Other goal")

	workloads, err := tn.analytics.Tasks.GetUsersWorkload(nil)
	if err != nil {
		t.Fatalf("GetUsersWorkload() error = %v", err)
	}
	if len(workloads) != 3 {
		t.Fatalf("expected the 3 users of the organization, got %d: %+v", len(workloads), workloads)
	}

	// Ordered by email, and users without work are included.
	aliceWork, bobWork, ownerWork := workloads[0], workloads[1], workloads[2]
	if aliceWork.UserID != alice.ID || bobWork.UserID != bob.ID || ownerWork.UserID != tn.user.ID {
		t.Fatalf("unexpected users: %q, %q, %q", aliceWork.Email, bobWork.Email, ownerWork.Email)
	}
	assertTaskStateCounts(t, aliceWork.ResponsibleTasks, 1, 1, 0, 0, 0, 0)
	assertTaskItemStateCounts(t, aliceWork.AssignedTaskItems, 1, 0, 1, 0)
	assertTaskStateCounts(t, bobWork.ResponsibleTasks, 0, 0, 0, 0, 1, 0)
	assertTaskItemStateCounts(t, bobWork.AssignedTaskItems, 0, 1, 0, 0)
	assertTaskStateCounts(t, ownerWork.ResponsibleTasks, 0, 0, 0, 0, 0, 0)
	assertTaskItemStateCounts(t, ownerWork.AssignedTaskItems, 0, 0, 0, 0)

	workloads, err = tn.analytics.Tasks.GetUsersWorkload(&UsersWorkloadParams{GoalID: &goalTwo.ID})
	if err != nil {
		t.Fatalf("GetUsersWorkload() by goal error = %v", err)
	}
	if len(workloads) != 3 {
		t.Fatalf("expected all 3 users even when scoped, got %d", len(workloads))
	}
	assertTaskStateCounts(t, workloads[0].ResponsibleTasks, 0, 0, 0, 0, 0, 0)
	assertTaskStateCounts(t, workloads[1].ResponsibleTasks, 0, 0, 0, 0, 1, 0)
	assertTaskItemStateCounts(t, workloads[1].AssignedTaskItems, 0, 1, 0, 0)
}

func TestTasksAnalytics_GetCompletedTasksOverTime(t *testing.T) {
	pool := testPool(t)
	tn := newTenant(t, pool, "owner@a.test")
	other := newTenant(t, pool, "owner@b.test")
	project := tn.project("A")
	goal := tn.goal(project.ID, "Goal")
	otherGoalInProject := tn.goal(project.ID, "Other goal")

	complete := func(goalID, name string, at time.Time) *sirkel_domain.Task {
		task := tn.task(goalID, name, sirkel_domain.TaskStateTodo)
		tn.setTaskState(task, sirkel_domain.TaskStateDone)
		tn.setTaskHistory(task.ID, at.Add(-48*time.Hour), map[sirkel_domain.TaskState]time.Time{sirkel_domain.TaskStateDone: at})
		return task
	}
	complete(goal.ID, "t1", date(2026, time.January, 10))
	complete(goal.ID, "t2", date(2026, time.January, 10))
	complete(goal.ID, "t3", date(2026, time.January, 12))
	complete(otherGoalInProject.ID, "in other goal", date(2026, time.January, 10))

	// Completed, reopened and completed again: counted once, when it was last completed.
	twice := tn.task(goal.ID, "done twice", sirkel_domain.TaskStateDone)
	tn.setTaskState(twice, sirkel_domain.TaskStateInProgress)
	tn.setTaskState(twice, sirkel_domain.TaskStateDone)
	tn.exec(`UPDATE sirkel_engine.task_events SET changed_at = $2 WHERE id = (SELECT min(id) FROM sirkel_engine.task_events WHERE task_id = $1 AND new_value = 'done')`, twice.ID, date(2026, time.January, 11))
	tn.exec(`UPDATE sirkel_engine.task_events SET changed_at = $2 WHERE id = (SELECT max(id) FROM sirkel_engine.task_events WHERE task_id = $1 AND new_value = 'done')`, twice.ID, date(2026, time.January, 13))

	// Completed and then reopened: not done anymore, so not counted.
	reopened := complete(goal.ID, "reopened", date(2026, time.January, 12))
	tn.setTaskState(reopened, sirkel_domain.TaskStateInProgress)

	otherGoalOfB := other.goal(other.project("Other").ID, "Other org goal")
	otherTask := other.task(otherGoalOfB.ID, "other org", sirkel_domain.TaskStateTodo)
	other.setTaskState(otherTask, sirkel_domain.TaskStateDone)
	other.setTaskHistory(otherTask.ID, date(2026, time.January, 1), map[sirkel_domain.TaskState]time.Time{sirkel_domain.TaskStateDone: date(2026, time.January, 10)})

	assertBuckets := func(name string, got []sirkel_domain.CompletedTasksBucket, err error, want []sirkel_domain.CompletedTasksBucket) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: error = %v", name, err)
		}
		if len(got) != len(want) {
			t.Fatalf("%s: expected %d buckets, got %d: %+v", name, len(want), len(got), got)
		}
		for i := range want {
			if !got[i].PeriodStart.Equal(want[i].PeriodStart) || got[i].Count != want[i].Count {
				t.Fatalf("%s: bucket %d: expected %+v, got %+v", name, i, want[i], got[i])
			}
		}
	}
	midnight := func(month time.Month, day int) time.Time { return time.Date(2026, month, day, 0, 0, 0, 0, time.UTC) }

	buckets, err := tn.analytics.Tasks.GetCompletedTasksOverTime(&CompletedTasksOverTimeParams{
		Interval: IntervalDay, From: midnight(time.January, 9), To: midnight(time.January, 15),
	})
	assertBuckets("daily", buckets, err, []sirkel_domain.CompletedTasksBucket{
		{PeriodStart: midnight(time.January, 9), Count: 0},
		{PeriodStart: midnight(time.January, 10), Count: 3},
		{PeriodStart: midnight(time.January, 11), Count: 0},
		{PeriodStart: midnight(time.January, 12), Count: 1},
		{PeriodStart: midnight(time.January, 13), Count: 1},
		{PeriodStart: midnight(time.January, 14), Count: 0},
	})

	buckets, err = tn.analytics.Tasks.GetCompletedTasksOverTime(&CompletedTasksOverTimeParams{
		Interval: IntervalWeek, From: midnight(time.January, 5), To: midnight(time.January, 19),
	})
	assertBuckets("weekly", buckets, err, []sirkel_domain.CompletedTasksBucket{
		{PeriodStart: midnight(time.January, 5), Count: 3},
		{PeriodStart: midnight(time.January, 12), Count: 2},
	})

	buckets, err = tn.analytics.Tasks.GetCompletedTasksOverTime(&CompletedTasksOverTimeParams{
		Interval: IntervalMonth, From: midnight(time.December, 1).AddDate(-1, 0, 0), To: midnight(time.February, 1),
	})
	assertBuckets("monthly", buckets, err, []sirkel_domain.CompletedTasksBucket{
		{PeriodStart: midnight(time.December, 1).AddDate(-1, 0, 0), Count: 0},
		{PeriodStart: midnight(time.January, 1), Count: 5},
	})

	buckets, err = tn.analytics.Tasks.GetCompletedTasksOverTime(&CompletedTasksOverTimeParams{
		GoalID: &goal.ID, Interval: IntervalWeek, From: midnight(time.January, 5), To: midnight(time.January, 19),
	})
	assertBuckets("weekly by goal", buckets, err, []sirkel_domain.CompletedTasksBucket{
		{PeriodStart: midnight(time.January, 5), Count: 2},
		{PeriodStart: midnight(time.January, 12), Count: 2},
	})

	valid := CompletedTasksOverTimeParams{Interval: IntervalDay, From: midnight(time.January, 1), To: midnight(time.January, 2)}
	_, err = tn.analytics.Tasks.GetCompletedTasksOverTime(nil)
	requireError(t, err, ErrInvalidInterval)
	invalidInterval := valid
	invalidInterval.Interval = "year"
	_, err = tn.analytics.Tasks.GetCompletedTasksOverTime(&invalidInterval)
	requireError(t, err, ErrInvalidInterval)
	missingFrom := valid
	missingFrom.From = time.Time{}
	_, err = tn.analytics.Tasks.GetCompletedTasksOverTime(&missingFrom)
	requireError(t, err, ErrInvalidRange)
	backwards := valid
	backwards.From, backwards.To = valid.To, valid.From
	_, err = tn.analytics.Tasks.GetCompletedTasksOverTime(&backwards)
	requireError(t, err, ErrInvalidRange)
}

func TestTasksAnalytics_GetTaskCycleTime(t *testing.T) {
	pool := testPool(t)
	tn := newTenant(t, pool, "owner@a.test")
	other := newTenant(t, pool, "owner@b.test")
	goal := tn.goal(tn.project("A").ID, "Goal")

	day := 24 * time.Hour
	finish := func(name string, createdAt time.Time, startedAt *time.Time, completedAt time.Time) {
		task := tn.task(goal.ID, name, sirkel_domain.TaskStateTodo)
		changes := map[sirkel_domain.TaskState]time.Time{sirkel_domain.TaskStateDone: completedAt}
		if startedAt != nil {
			tn.setTaskState(task, sirkel_domain.TaskStateInProgress)
			changes[sirkel_domain.TaskStateInProgress] = *startedAt
		}
		tn.setTaskState(task, sirkel_domain.TaskStateDone)
		tn.setTaskHistory(task.ID, createdAt, changes)
	}
	jan := func(day int) time.Time { return date(2026, time.January, day) }
	started := func(at time.Time) *time.Time { return &at }

	finish("x", jan(1), started(jan(2)), jan(4)) // lead 3d, cycle 2d
	finish("y", jan(2), started(jan(3)), jan(8)) // lead 6d, cycle 5d
	finish("z", jan(1), nil, jan(5))             // lead 4d, never in progress
	tn.task(goal.ID, "still open", sirkel_domain.TaskStateInProgress)

	otherGoal := other.goal(other.project("Other").ID, "Other goal")
	otherTask := other.task(otherGoal.ID, "other org", sirkel_domain.TaskStateTodo)
	other.setTaskState(otherTask, sirkel_domain.TaskStateDone)
	other.setTaskHistory(otherTask.ID, jan(1), map[sirkel_domain.TaskState]time.Time{sirkel_domain.TaskStateDone: jan(2)})

	seconds := func(d time.Duration) float64 { return d.Seconds() }

	stats, err := tn.analytics.Tasks.GetTaskCycleTime(nil)
	if err != nil {
		t.Fatalf("GetTaskCycleTime() error = %v", err)
	}
	if stats.CompletedTasks != 3 || stats.TasksWithCycleTime != 2 {
		t.Fatalf("expected 3 completed tasks and 2 with cycle time, got %+v", stats)
	}
	assertSeconds(t, "average lead", stats.AverageLeadSeconds, seconds((3*day+6*day+4*day)/3))
	assertSeconds(t, "median lead", stats.MedianLeadSeconds, seconds(4*day))
	assertSeconds(t, "average cycle", stats.AverageCycleSeconds, seconds((2*day+5*day)/2))
	assertSeconds(t, "median cycle", stats.MedianCycleSeconds, seconds((2*day+5*day)/2))

	midnightJan5 := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)
	stats, err = tn.analytics.Tasks.GetTaskCycleTime(&TaskCycleTimeParams{CompletedFrom: &midnightJan5})
	if err != nil {
		t.Fatalf("GetTaskCycleTime() from error = %v", err)
	}
	if stats.CompletedTasks != 2 || stats.TasksWithCycleTime != 1 {
		t.Fatalf("expected 2 completed tasks and 1 with cycle time from Jan 5, got %+v", stats)
	}
	assertSeconds(t, "average cycle", stats.AverageCycleSeconds, seconds(5*day))

	stats, err = tn.analytics.Tasks.GetTaskCycleTime(&TaskCycleTimeParams{CompletedTo: &midnightJan5})
	if err != nil {
		t.Fatalf("GetTaskCycleTime() to error = %v", err)
	}
	if stats.CompletedTasks != 1 || stats.TasksWithCycleTime != 1 {
		t.Fatalf("expected 1 completed task before Jan 5, got %+v", stats)
	}
	assertSeconds(t, "average lead", stats.AverageLeadSeconds, seconds(3*day))
	assertSeconds(t, "average cycle", stats.AverageCycleSeconds, seconds(2*day))

	midnightFeb1 := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	stats, err = tn.analytics.Tasks.GetTaskCycleTime(&TaskCycleTimeParams{CompletedFrom: &midnightFeb1})
	if err != nil {
		t.Fatalf("GetTaskCycleTime() empty period error = %v", err)
	}
	if *stats != (sirkel_domain.TaskCycleTimeStats{}) {
		t.Fatalf("expected empty stats for a period without completions, got %+v", stats)
	}
}
