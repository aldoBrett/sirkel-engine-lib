package sirkel_repository

import (
	"context"
	"errors"

	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAssigneeNotInOrganization = errors.New("assigned user does not belong to the organization")

// actorID returns the ID of the user performing the operation, or nil when there is none.
func actorID(user *sirkel_domain.User) *string {
	if user == nil || user.ID == "" {
		return nil
	}

	return &user.ID
}

func userBelongsToGoalOrganization(ctx context.Context, pool *pgxpool.Pool, userID, goalID string) error {
	var belongs bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM auth.users u
			JOIN sirkel_engine.goals g ON g.id = $2
			JOIN sirkel_engine.projects p ON p.id = g.project_id
			WHERE u.id = $1 AND u.organization_id = p.organization_id
		)
	`, userID, goalID).Scan(&belongs)
	if err != nil {
		return err
	}
	if !belongs {
		return ErrAssigneeNotInOrganization
	}

	return nil
}

func userBelongsToTaskOrganization(ctx context.Context, pool *pgxpool.Pool, userID, taskID string) error {
	var belongs bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM auth.users u
			JOIN sirkel_engine.tasks t ON t.id = $2
			JOIN sirkel_engine.goals g ON g.id = t.goal_id
			JOIN sirkel_engine.projects p ON p.id = g.project_id
			WHERE u.id = $1 AND u.organization_id = p.organization_id
		)
	`, userID, taskID).Scan(&belongs)
	if err != nil {
		return err
	}
	if !belongs {
		return ErrAssigneeNotInOrganization
	}

	return nil
}

type Repositories struct {
	Tasks     TasksRepository
	TaskItems TaskItemsRepository
	Goals     GoalsRepository
	Projects  ProjectsRepository
}

func NewRepositories(ctx context.Context, pool *pgxpool.Pool, user *sirkel_domain.User) *Repositories {
	return &Repositories{
		Tasks:     NewTasksRepositoryHandler(ctx, pool, user),
		TaskItems: NewTaskItemsRepositoryHandler(ctx, pool, user),
		Goals:     NewGoalsRepositoryHandler(ctx, pool, user),
		Projects:  NewProjectsRepositoryHandler(ctx, pool, user),
	}
}
