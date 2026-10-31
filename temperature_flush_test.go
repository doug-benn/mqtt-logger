package main

import (
	"context"
	"testing"
	"time"

	"github.com/doug-benn/mqtt-logger/database"
	"github.com/doug-benn/mqtt-logger/handlers"
	"github.com/doug-benn/mqtt-logger/repository"
	"github.com/doug-benn/mqtt-logger/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

// openTestStore dials the dev stack for wiring tests, or skips when it is
// down — the same contract the repository tests use.
func openTestStore(t *testing.T) (repository.Store, *pgxpool.Pool) {
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
	return postgres.NewStore(pool), pool
}

// TestTemperatureFlushReachesPostgres crosses the whole temperature wiring
// seam — route chain, buffer, flush func, store — against live Postgres:
// readings must land as rows with their decode-time arrival stamps, and the
// malformed payload between them must not.
func TestTemperatureFlushReachesPostgres(t *testing.T) {
	store, pool := openTestStore(t)
	ctx := context.Background()

	before := time.Now()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			"DELETE FROM temperature WHERE received_at >= $1", before)
	})

	// Composed exactly as runMQTT wires it — the constructor carries the
	// real flush (decode product → store rows), so the conversion and the
	// log-and-drop path are what this exercises.
	routes, closeBuffers := buildRoutes(store, discardLogger())

	for _, payload := range []string{"21.5", "not-a-number", "22.75"} {
		routes["temperature"](ctx, handlers.RawMessage{Topic: "temperature", Payload: []byte(payload)})
	}
	// Closing flushes the partial batch; the rows must be visible when
	// close returns.
	closeBuffers()

	rows, err := pool.Query(ctx,
		"SELECT value FROM temperature WHERE received_at >= $1 ORDER BY value", before)
	if err != nil {
		t.Fatalf("query written rows: %v", err)
	}
	defer rows.Close()

	var got []float64
	for rows.Next() {
		var value float64
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	want := []float64{21.5, 22.75} // sorted ascending; the malformed payload dropped
	if len(got) != len(want) {
		t.Fatalf("rows written = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d value = %v, want %v", i, got[i], want[i])
		}
	}
}
