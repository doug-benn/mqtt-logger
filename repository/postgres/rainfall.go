// Package postgres binds repository interfaces to Postgres via the
// sqlc-generated queries. Database drivers stay confined to this package.
package postgres

import (
	"context"

	"github.com/doug-benn/mqtt-logger/database/queries"
	"github.com/doug-benn/mqtt-logger/model"
	"github.com/doug-benn/mqtt-logger/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type rainfallRepo struct {
	queries *queries.Queries
}

// NewRainfallRepo binds repository.RainfallRepo to a Postgres connection pool.
func NewRainfallRepo(pool *pgxpool.Pool) repository.RainfallRepo {
	return &rainfallRepo{queries: queries.New(pool)}
}

func (r *rainfallRepo) Create(ctx context.Context, value float64) (model.Rainfall, error) {
	return r.queries.CreateRainfall(ctx, value)
}

func (r *rainfallRepo) List(ctx context.Context) ([]model.Rainfall, error) {
	return r.queries.ListRainfall(ctx)
}
