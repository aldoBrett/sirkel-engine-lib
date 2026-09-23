package sirkel_auth

import (
	"sirkel-engine-lib/sirkel_domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SirkelAuthHandler struct {
	Pool *pgxpool.Pool
	User *sirkel_domain.User
}

type SirkelAuthHandlerParams struct {
	Pool *pgxpool.Pool
	User *sirkel_domain.User
}

func NewSirkelAuthHandler(params SirkelAuthHandlerParams) *SirkelAuthHandler {
	return &SirkelAuthHandler{
		Pool: params.Pool,
		User: params.User,
	}
}
