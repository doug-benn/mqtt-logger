package database

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	"github.com/doug-benn/mqtt-logger/utilities"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresConfig holds database configuration parameters
type PostgresConfig struct {
	Host              string
	Port              int
	Username          string
	Password          string
	Database          string
	SSLMode           string
	ConnectTimeout    time.Duration
	ApplicationName   string
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	MaxRetries        int
	InitialRetryDelay time.Duration
	BackoffMultiplier float64
	MaxRetryDelay     time.Duration
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() PostgresConfig {
	port, err := strconv.Atoi(utilities.GetEnvOrDefault("POSTGRES_PORT", "5435"))
	if err != nil {
		port = 5435
	}

	return PostgresConfig{
		Host:              utilities.GetEnvOrDefault("POSTGRES_HOST", "localhost"),
		Port:              port,
		Username:          utilities.GetEnvOrDefault("POSTGRES_USER", "postgres"),
		Password:          utilities.GetEnvOrDefault("POSTGRES_PASSWORD", "postgres"),
		Database:          utilities.GetEnvOrDefault("POSTGRES_DB", "testdb"),
		SSLMode:           utilities.GetEnvOrDefault("POSTGRES_SSL_MODE", "disable"),
		ConnectTimeout:    5 * time.Second,
		ApplicationName:   utilities.GetEnvOrDefault("APPLICATION_NAME", "mqtt-listener"),
		MaxConns:          30,
		MinConns:          1,
		MaxConnLifetime:   time.Hour,
		MaxConnIdleTime:   30 * time.Minute,
		HealthCheckPeriod: 1 * time.Minute,
		MaxRetries:        3,
		InitialRetryDelay: 1 * time.Second,
		BackoffMultiplier: 2.0,
		MaxRetryDelay:     30 * time.Second,
	}
}

// PostgresDatabase is the Postgres implementation of the database store.
type PostgresDatabase struct {
	pool   *pgxpool.Pool
	config PostgresConfig
	logger *slog.Logger
}

// NewDatabase creates a database connection pool and tests the connection.
func NewDatabase(ctx context.Context, logger *slog.Logger, config PostgresConfig) (*PostgresDatabase, error) {
	db := &PostgresDatabase{
		config: config,
		logger: logger,
	}

	// Initialize the connection pool
	pool, err := db.initPool(ctx)
	if err != nil {
		logger.Error("failed to initialize database pool", "error", err)
		return nil, fmt.Errorf("failed to initialize database pool: %w", err)
	}

	db.pool = pool

	// Test the connection with retry logic
	if err := db.pingWithRetry(ctx); err != nil {
		// Clean up the pool if ping fails
		db.pool.Close()
		logger.Error("failed to establish database connection", "error", err)
		return nil, err
	}

	// Log successful connection without sensitive information
	logger.Info("successfully connected to database",
		"host", config.Host,
		"port", config.Port,
		"database", config.Database,
		"max_conns", config.MaxConns,
		"min_conns", config.MinConns,
	)

	return db, nil
}

// initPool creates and configures the connection pool
func (db *PostgresDatabase) initPool(ctx context.Context) (*pgxpool.Pool, error) {
	connStr := BuildConnectionString(db.config)
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection config: %w", err)
	}

	// Apply pool settings to the parsed config
	config.MaxConns = db.config.MaxConns
	config.MinConns = db.config.MinConns
	config.MaxConnLifetime = db.config.MaxConnLifetime
	config.MaxConnIdleTime = db.config.MaxConnIdleTime
	config.HealthCheckPeriod = db.config.HealthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	return pool, nil
}

// pingWithRetry attempts to ping the database with exponential backoff
func (db *PostgresDatabase) pingWithRetry(ctx context.Context) error {
	// Zero retries must still ping once, otherwise NewDatabase reports success
	// without ever talking to the database
	attempts := max(db.config.MaxRetries, 1)
	delay := db.config.InitialRetryDelay
	var lastErr error

	for attempt := 1; attempt <= attempts; attempt++ {
		lastErr = db.pool.Ping(ctx)
		if lastErr == nil {
			if attempt > 1 {
				db.logger.Info("database ping succeeded after retries", "attempt", attempt)
			}
			return nil
		}

		if attempt == attempts {
			break
		}

		db.logger.Warn("database ping failed, retrying",
			"attempt", attempt,
			"max_attempts", attempts,
			"retry_delay", delay,
		)

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}

		// Calculate next delay with exponential backoff
		delay = min(time.Duration(float64(delay)*db.config.BackoffMultiplier), db.config.MaxRetryDelay)
	}

	return fmt.Errorf("database ping failed after %d attempts: %w", attempts, lastErr)
}

// Pool returns the underlying connection pool for direct access when needed
func (db *PostgresDatabase) Pool() *pgxpool.Pool {
	return db.pool
}

// Ping tests the database connection
func (db *PostgresDatabase) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// Stats returns a copy of connection pool statistics
func (db *PostgresDatabase) Stats() *pgxpool.Stat {
	return db.pool.Stat()
}

// Close closes the connection pool gracefully
func (db *PostgresDatabase) Close() {
	db.logger.Info("closing database connection pool")
	db.pool.Close()
	db.logger.Info("database connection pool closed successfully")
}

// BuildConnectionString builds a connection string for external use (testing, migrations, etc.)
// Properly URL-encodes components to handle special characters
func BuildConnectionString(config PostgresConfig) string {
	u := &url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(config.Username, config.Password),
		Host:   fmt.Sprintf("%s:%d", config.Host, config.Port),
		Path:   config.Database,
	}

	query := url.Values{}
	if config.SSLMode != "" {
		query.Add("sslmode", config.SSLMode)
	}
	if config.ConnectTimeout > 0 {
		// Seconds() truncates, and a sub-second value would encode as 0 —
		// which Postgres treats as no timeout at all
		query.Add("connect_timeout", strconv.Itoa(max(1, int(config.ConnectTimeout.Seconds()))))
	}
	if config.ApplicationName != "" {
		query.Add("application_name", config.ApplicationName)
	}

	u.RawQuery = query.Encode()
	return u.String()
}
