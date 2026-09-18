package sirkel_repository

import (
	"context"

	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
