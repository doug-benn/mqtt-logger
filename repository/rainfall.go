package repository

import (
	"context"

	"github.com/doug-benn/mqtt-logger/model"
)

// RainfallRepo is the database-agnostic seam for rainfall readings: callers
// depend on this interface and on model types, never on a database driver.
type RainfallRepo interface {
	Create(ctx context.Context, value float64) (model.Rainfall, error)
	List(ctx context.Context) ([]model.Rainfall, error)
}
