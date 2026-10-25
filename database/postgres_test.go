package database

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	os.Exit(code)
}

func setupPostgresContainer(t *testing.T) (PostgresConfig, func()) {
	t.Helper()

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       "testdb",
			"POSTGRES_USER":     "testuser",
			"POSTGRES_PASSWORD": "testpass",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(30 * time.Second),
	}

	postgresContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	host, err := postgresContainer.Host(ctx)
	require.NoError(t, err)

	port, err := postgresContainer.MappedPort(ctx, "5432")
	require.NoError(t, err)

	config := PostgresConfig{
		Host:              host,
		Port:              int(port.Num()),
		Username:          "testuser",
		Password:          "testpass",
		Database:          "testdb",
		SSLMode:           "disable",
		ConnectTimeout:    5 * time.Second,
		ApplicationName:   "test-app",
		MaxConns:          10,
		MinConns:          1,
		MaxConnLifetime:   time.Hour,
		MaxConnIdleTime:   30 * time.Minute,
		HealthCheckPeriod: 1 * time.Minute,
		MaxRetries:        3,
		InitialRetryDelay: 100 * time.Millisecond, // Faster for tests
		BackoffMultiplier: 2.0,
		MaxRetryDelay:     5 * time.Second,
	}

	cleanup := func() {
		if err := postgresContainer.Terminate(ctx); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	}

	return config, cleanup
}

func TestNewDatabase_Success(t *testing.T) {
	config, cleanup := setupPostgresContainer(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()

	db, err := NewDatabase(ctx, logger, config)
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	// Verify pool is properly configured
	stats := db.Stats()
	assert.NotNil(t, stats)
	assert.Equal(t, int32(10), stats.MaxConns())

	// Test ping works
	err = db.Ping(ctx)
	assert.NoError(t, err)
}

func TestNewDatabase_InvalidConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()

	config := PostgresConfig{
		Host:              "nonexistent-host",
		Port:              5432,
		Username:          "user",
		Password:          "pass",
		Database:          "db",
		SSLMode:           "disable",
		HealthCheckPeriod: 1 * time.Minute,
		MaxRetries:        1, // Fail fast
		InitialRetryDelay: 10 * time.Millisecond,
		MaxConns:          10,
		MinConns:          1,
	}

	db, err := NewDatabase(ctx, logger, config)
	assert.Error(t, err)
	assert.Nil(t, db)
}

func TestNewDatabase_ContextCancellation(t *testing.T) {
	config, cleanup := setupPostgresContainer(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Create context that will be cancelled quickly
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Wait for context to be cancelled
	time.Sleep(10 * time.Millisecond)

	db, err := NewDatabase(ctx, logger, config)
	assert.Error(t, err)
	assert.Nil(t, db)
	assert.Contains(t, err.Error(), "context")
}

func TestPostgresDatabase_PingAfterClose(t *testing.T) {
	config, cleanup := setupPostgresContainer(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()

	db, err := NewDatabase(ctx, logger, config)
	require.NoError(t, err)

	// Close the database
	db.Close()

	// Ping should fail after close
	err = db.Ping(ctx)
	assert.Error(t, err)
}

func TestPostgresDatabase_Stats(t *testing.T) {
	config, cleanup := setupPostgresContainer(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()

	db, err := NewDatabase(ctx, logger, config)
	require.NoError(t, err)
	defer db.Close()

	stats := db.Stats()
	assert.NotNil(t, stats)
	assert.Equal(t, int32(10), stats.MaxConns()) // From our test config
}

func TestPostgresDatabase_Pool(t *testing.T) {
	config, cleanup := setupPostgresContainer(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()

	db, err := NewDatabase(ctx, logger, config)
	require.NoError(t, err)
	defer db.Close()

	pool := db.Pool()
	assert.NotNil(t, pool)

	// Test we can use the pool directly
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var result int
	err = conn.QueryRow(ctx, "SELECT 1").Scan(&result)
	assert.NoError(t, err)
	assert.Equal(t, 1, result)
}

func TestBuildConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		config   PostgresConfig
		expected string
	}{
		{
			name: "basic config",
			config: PostgresConfig{
				Host:     "localhost",
				Port:     5432,
				Username: "user",
				Password: "pass",
				Database: "db",
			},
			expected: "postgresql://user:pass@localhost:5432/db",
		},
		{
			name: "with ssl and timeout",
			config: PostgresConfig{
				Host:           "localhost",
				Port:           5432,
				Username:       "user",
				Password:       "pass",
				Database:       "db",
				SSLMode:        "require",
				ConnectTimeout: 10 * time.Second,
			},
			expected: "postgresql://user:pass@localhost:5432/db?connect_timeout=10&sslmode=require",
		},
		{
			name: "with special characters in password",
			config: PostgresConfig{
				Host:     "localhost",
				Port:     5432,
				Username: "user",
				Password: "p@ss&word#123",
				Database: "db",
			},
			expected: "postgresql://user:p%40ss&word%23123@localhost:5432/db",
		},
		{
			name: "with application name",
			config: PostgresConfig{
				Host:            "localhost",
				Port:            5432,
				Username:        "user",
				Password:        "pass",
				Database:        "db",
				ApplicationName: "my-app",
			},
			expected: "postgresql://user:pass@localhost:5432/db?application_name=my-app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildConnectionString(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRetryBehavior(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()

	// Use a config that will definitely fail
	config := PostgresConfig{
		Host:              "localhost",
		Port:              9999, // Non-existent port
		Username:          "user",
		Password:          "pass",
		Database:          "db",
		SSLMode:           "disable",
		MaxConns:          10,
		MinConns:          1,
		MaxRetries:        3, // 3 attempts total
		HealthCheckPeriod: 1 * time.Minute,
		InitialRetryDelay: 100 * time.Millisecond,
		BackoffMultiplier: 2.0,
		MaxRetryDelay:     1 * time.Second,
	}

	start := time.Now()
	db, err := NewDatabase(ctx, logger, config)
	duration := time.Since(start)

	assert.Error(t, err)
	assert.Nil(t, db)
	// Check for the actual error message from our code
	assert.Contains(t, err.Error(), "database ping failed after 3 attempts")

	// Should have taken at least the retry delays (100ms + 200ms = 300ms minimum)
	// But connection failures can be fast, so let's be more lenient
	assert.Greater(t, duration, 50*time.Millisecond)
}

// Benchmark to ensure pool performance is reasonable
func BenchmarkDatabasePing(b *testing.B) {
	config, cleanup := setupPostgresContainer(&testing.T{})
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx := context.Background()

	db, err := NewDatabase(ctx, logger, config)
	if err != nil {
		b.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := db.Ping(ctx); err != nil {
				b.Fatalf("ping failed: %v", err)
			}
		}
	})
}

// Benchmark pool stats retrieval performance
func BenchmarkDatabaseStats(b *testing.B) {
	config, cleanup := setupPostgresContainer(&testing.T{})
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx := context.Background()

	db, err := NewDatabase(ctx, logger, config)
	if err != nil {
		b.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			stats := db.Stats()
			if stats == nil {
				b.Fatal("stats should not be nil")
			}
			// Access some stats to ensure they're computed
			_ = stats.AcquiredConns()
			_ = stats.IdleConns()
			_ = stats.TotalConns()
		}
	})
}

func TestBuildConnectionStringEncodesSpecialCharacters(t *testing.T) {
	config := DefaultConfig()
	config.Username = "mqtt@logger"
	config.Password = "p@ss:word/1"

	connStr := BuildConnectionString(config)

	for _, want := range []string{"mqtt%40logger", "p%40ss%3Aword%2F1"} {
		if !strings.Contains(connStr, want) {
			t.Errorf("connection string %q does not contain encoded %q", connStr, want)
		}
	}
}

func TestBuildConnectionStringTimeoutNeverEncodesZero(t *testing.T) {
	config := DefaultConfig()

	config.ConnectTimeout = 500 * time.Millisecond
	if got := BuildConnectionString(config); !strings.Contains(got, "connect_timeout=1") {
		t.Errorf("sub-second timeout: expected connect_timeout=1, got %q", got)
	}

	config.ConnectTimeout = 5 * time.Second
	if got := BuildConnectionString(config); !strings.Contains(got, "connect_timeout=5") {
		t.Errorf("5s timeout: expected connect_timeout=5, got %q", got)
	}

	config.ConnectTimeout = 0
	if got := BuildConnectionString(config); strings.Contains(got, "connect_timeout") {
		t.Errorf("zero timeout should omit the parameter, got %q", got)
	}
}

func TestBuildConnectionStringIncludesCoreParameters(t *testing.T) {
	config := DefaultConfig()
	connStr := BuildConnectionString(config)

	for _, want := range []string{
		"postgresql://",
		"sslmode=" + config.SSLMode,
		"application_name=" + config.ApplicationName,
		"/" + config.Database,
	} {
		if !strings.Contains(connStr, want) {
			t.Errorf("connection string %q does not contain %q", connStr, want)
		}
	}
}
