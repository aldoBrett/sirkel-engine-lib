package sirkel_analytics

import (
	"context"
	"errors"

	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrOrganizationRequired = errors.New("organization is required")
	ErrInvalidRange         = errors.New("from and to are required, and from must be before to")
	ErrInvalidInterval      = errors.New("interval must be day, week or month")
)

type Analytics struct {
	Tasks TasksAnalytics
}

func NewAnalytics(ctx context.Context, pool *pgxpool.Pool, user *sirkel_domain.User) *Analytics {
	return &Analytics{
		Tasks: NewTasksAnalyticsHandler(ctx, pool, user),
	}
}
