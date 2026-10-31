// Package postgres binds repository.Store to Postgres via the sqlc-generated
// queries. Database drivers stay confined to this package.
package postgres

import (
	"context"
	"time"

	"github.com/doug-benn/mqtt-logger/database/queries"
	"github.com/doug-benn/mqtt-logger/model"
	"github.com/doug-benn/mqtt-logger/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type store struct {
	queries *queries.Queries
}

// NewStore binds repository.Store to a Postgres connection pool.
func NewStore(pool *pgxpool.Pool) repository.Store {
	return &store{queries: queries.New(pool)}
}

func (s *store) CreateRainfall(ctx context.Context, value float64) (model.Rainfall, error) {
	return s.queries.CreateRainfall(ctx, value)
}

func (s *store) ListRainfall(ctx context.Context) ([]model.Rainfall, error) {
	return s.queries.ListRainfall(ctx)
}

// splitBatch projects a batch into the parallel arrays the unnest inserts
// take: the pairs evaluate in lockstep, so a mismatched slice would NULL-pad
// and violate the NOT NULL column.
func splitBatch[T any](batch []T, value func(T) float64, at func(T) time.Time) ([]float64, []time.Time) {
	values := make([]float64, len(batch))
	receivedAt := make([]time.Time, len(batch))
	for i, reading := range batch {
		values[i] = value(reading)
		receivedAt[i] = at(reading)
	}
	return values, receivedAt
}

func (s *store) CreateTemperatureBatch(ctx context.Context, batch []model.Temperature) error {
	if len(batch) == 0 {
		return nil // nothing to exec
	}
	values, receivedAt := splitBatch(batch,
		func(r model.Temperature) float64 { return r.Value },
		func(r model.Temperature) time.Time { return r.ReceivedAt },
	)
	return s.queries.CreateTemperatures(ctx, queries.CreateTemperaturesParams{
		Values:     values,
		ReceivedAt: receivedAt,
	})
}

func (s *store) CreateHumidities(ctx context.Context, batch []model.Humidity) error {
	if len(batch) == 0 {
		return nil // nothing to exec
	}
	values, receivedAt := splitBatch(batch,
		func(r model.Humidity) float64 { return r.Value },
		func(r model.Humidity) time.Time { return r.ReceivedAt },
	)
	return s.queries.CreateHumidities(ctx, queries.CreateHumiditiesParams{
		Values:     values,
		ReceivedAt: receivedAt,
	})
}
