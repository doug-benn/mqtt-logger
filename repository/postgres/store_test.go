package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/doug-benn/mqtt-logger/database"
	"github.com/doug-benn/mqtt-logger/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(database.BuildConnectionString(database.DefaultConfig()))
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("postgres not reachable — start it with `docker compose up -d`: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestStoreCreateRainfall(t *testing.T) {
	pool := openTestPool(t)
	repo := NewStore(pool)
	ctx := context.Background()

	created, err := repo.CreateRainfall(ctx, 1.5)
	if err != nil {
		t.Fatalf("CreateRainfall: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM rainfall WHERE id = $1", created.ID)
	})

	if created.ID == 0 {
		t.Error("expected a server-assigned id, got 0")
	}
	if created.Value != 1.5 {
		t.Errorf("Value = %v, want 1.5", created.Value)
	}
	if created.ReceivedAt.IsZero() {
		t.Error("ReceivedAt is zero; expected the server-side timestamp")
	}

	list, err := repo.ListRainfall(ctx)
	if err != nil {
		t.Fatalf("ListRainfall: %v", err)
	}
	found := false
	for _, r := range list {
		if r.ID == created.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ListRainfall does not contain created reading %d", created.ID)
	}
}

func TestStoreCreateTemperatureBatch(t *testing.T) {
	pool := openTestPool(t)
	repo := NewStore(pool)
	ctx := context.Background()

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			"DELETE FROM temperature WHERE received_at >= $1", time.Now().Add(-time.Minute))
	})

	// Arrival stamps precede the flush; the point of the seam is that they
	// survive to the database unrounded by insert time. Truncate to the
	// timestamp Postgres stores (microsecond precision).
	now := time.Now().Truncate(time.Microsecond)
	batch := []model.Temperature{
		{Value: 21.5, ReceivedAt: now.Add(-3 * time.Second)},
		{Value: 22.25, ReceivedAt: now.Add(-2 * time.Second)},
		{Value: 19.0, ReceivedAt: now.Add(-time.Second)},
	}

	if err := repo.CreateTemperatureBatch(ctx, batch); err != nil {
		t.Fatalf("CreateTemperatureBatch: %v", err)
	}

	rows, err := pool.Query(ctx,
		"SELECT value, received_at FROM temperature WHERE received_at >= $1 ORDER BY received_at",
		now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("query written rows: %v", err)
	}
	defer rows.Close()

	var got []model.Temperature
	for rows.Next() {
		var r model.Temperature
		if err := rows.Scan(&r.Value, &r.ReceivedAt); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	if len(got) != len(batch) {
		t.Fatalf("wrote %d rows, want %d", len(got), len(batch))
	}
	for i, want := range batch {
		if got[i].Value != want.Value {
			t.Errorf("row %d Value = %v, want %v", i, got[i].Value, want.Value)
		}
		if !got[i].ReceivedAt.Equal(want.ReceivedAt) {
			t.Errorf("row %d ReceivedAt = %v, want %v (arrival stamp must ride the batch)",
				i, got[i].ReceivedAt, want.ReceivedAt)
		}
	}
}

func TestStoreCreateHumidities(t *testing.T) {
	pool := openTestPool(t)
	repo := NewStore(pool)
	ctx := context.Background()

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			"DELETE FROM humidity WHERE received_at >= $1", time.Now().Add(-time.Minute))
	})

	// Arrival stamps precede the flush and must survive to the database
	// unrounded by insert time. Truncate to the timestamp Postgres stores
	// (microsecond precision).
	now := time.Now().Truncate(time.Microsecond)
	batch := []model.Humidity{
		{Value: 48.2, ReceivedAt: now.Add(-2 * time.Second)},
		{Value: 51.75, ReceivedAt: now.Add(-time.Second)},
	}

	if err := repo.CreateHumidities(ctx, batch); err != nil {
		t.Fatalf("CreateHumidities: %v", err)
	}
	if err := repo.CreateHumidities(ctx, nil); err != nil {
		t.Fatalf("CreateHumidities(nil): %v", err)
	}

	rows, err := pool.Query(ctx,
		"SELECT value, received_at FROM humidity WHERE received_at >= $1 ORDER BY received_at",
		now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("query written rows: %v", err)
	}
	defer rows.Close()

	var got []model.Humidity
	for rows.Next() {
		var r model.Humidity
		if err := rows.Scan(&r.Value, &r.ReceivedAt); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}

	if len(got) != len(batch) {
		t.Fatalf("wrote %d rows, want %d", len(got), len(batch))
	}
	for i, want := range batch {
		if got[i].Value != want.Value {
			t.Errorf("row %d Value = %v, want %v", i, got[i].Value, want.Value)
		}
		if !got[i].ReceivedAt.Equal(want.ReceivedAt) {
			t.Errorf("row %d ReceivedAt = %v, want %v (arrival stamp must ride the batch)",
				i, got[i].ReceivedAt, want.ReceivedAt)
		}
	}
}

func TestStoreCreateTemperatureBatchEmpty(t *testing.T) {
	pool := openTestPool(t)
	repo := NewStore(pool)

	if err := repo.CreateTemperatureBatch(context.Background(), nil); err != nil {
		t.Fatalf("CreateTemperatureBatch(nil): %v", err)
	}
}
