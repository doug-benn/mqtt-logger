package repository

import (
	"context"

	"github.com/doug-benn/mqtt-logger/model"
)

// Store is the database-agnostic seam for telemetry readings: one repository
// covering every message type, so callers depend on this interface and on
// model types, never on a database driver. Method names carry the message
// because the seam spans all of them.
type Store interface {
	CreateRainfall(ctx context.Context, value float64) (model.Rainfall, error)
	ListRainfall(ctx context.Context) ([]model.Rainfall, error)

	// CreateTemperatureBatch writes the whole batch in one insert. Each
	// reading's ReceivedAt rides in — the database must not stamp flush
	// time as arrival time.
	CreateTemperatureBatch(ctx context.Context, batch []model.Temperature) error

	// CreateHumidities writes the whole batch in one insert. Each
	// reading's ReceivedAt rides in — the database must not stamp flush
	// time as arrival time.
	CreateHumidities(ctx context.Context, batch []model.Humidity) error
}
