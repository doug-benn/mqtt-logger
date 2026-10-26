package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/doug-benn/mqtt-logger/database"
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

func TestRainfallRepo(t *testing.T) {
	pool := openTestPool(t)
	repo := NewRainfallRepo(pool)
	ctx := context.Background()

	created, err := repo.Create(ctx, 1.5)
	if err != nil {
		t.Fatalf("Create: %v", err)
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

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, r := range list {
		if r.ID == created.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("List does not contain created reading %d", created.ID)
	}
}
