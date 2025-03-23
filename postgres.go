package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	_ "github.com/joho/godotenv/autoload"
	_ "github.com/lib/pq"
)

//Postgres Docker Command
//docker run --name postgres -e POSTGRES_PASSWORD=password -e POSTGRES_DB=test_db -p 5432:5432 -d postgres

//TODO Turn the db start/stop into a controller
//TODO Split out the service

var (
	host     = os.Getenv("POSTGRES_HOST")
	port     = os.Getenv("POSTGRES_PORT")
	username = os.Getenv("POSTGRES_USER")
	password = os.Getenv("POSTGRES_PASSWORD")
	database = os.Getenv("POSTGRES_DB")
)

// Database is the Postgres implementation of the database store.
type PostgresDatabase struct {
	startStopMutex sync.Mutex
	running        bool
	Sql            *sql.DB
}

// NewDatabase creates a database connection pool in DB and pings the database.
func NewDatabase(connLimits bool, idleLimits bool) (*PostgresDatabase, error) {
	connStr := "postgresql://" + username + ":" + password +
		"@" + host + ":" + port + "/" + database + "?sslmode=disable&connect_timeout=1"

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	if connLimits {
		db.SetMaxOpenConns(5)
	}

	if idleLimits {
		db.SetMaxIdleConns(5)
	}

	return &PostgresDatabase{
		Sql: db,
	}, nil
}

// Start pings the database, and if it fails, retries up to 3 times
// before returning a start error.
// TODO Remove this and an the logic to the "new service"
func (db *PostgresDatabase) Start(ctx context.Context) (runError <-chan error, err error) {
	db.startStopMutex.Lock()
	defer db.startStopMutex.Unlock()

	if db.running {
		return nil, fmt.Errorf("%s", "database service is already running")
	}

	fails := 0
	const maxFails = 3
	const sleepDuration = 200 * time.Millisecond
	var totalTryTime time.Duration
	for {
		err = db.Sql.PingContext(ctx)
		if err == nil {
			break
		} else if ctx.Err() != nil {
			return nil, fmt.Errorf("pinging database: %w", err)
		}
		fails++
		if fails == maxFails {
			return nil, fmt.Errorf("failed connecting to database after %d tries in %s: %w", fails, totalTryTime, err)
		}
		time.Sleep(sleepDuration)
		totalTryTime += sleepDuration
	}

	db.running = true

	// TODO have periodic ping to check connection is still alive and signal through the run error channel.
	return nil, nil
}

// Stop stops the database and closes the connection.
func (db *PostgresDatabase) Stop() (err error) {
	db.startStopMutex.Lock()
	defer db.startStopMutex.Unlock()
	if !db.running {
		fmt.Println("database is not running")
		return fmt.Errorf("%s", "database is not running")
	}

	err = db.Sql.Close()
	if err != nil {
		fmt.Println("closing database connection")
		return fmt.Errorf("closing database connection: %w", err)
	}

	db.running = false
	return nil
}

// Health checks the health of the database connection by pinging the database.
// It returns a map with keys indicating various health statistics.
func (db *PostgresDatabase) Health() map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	stats := make(map[string]string)

	// Ping the database
	err := db.Sql.PingContext(ctx)
	if err != nil {
		stats["status"] = "down"
		stats["error"] = fmt.Sprintf("db down: %v", err)
		log.Fatalf("db down: %v", err) // Log the error and terminate the program
		return stats
	}

	// Database is up, add more statistics
	stats["status"] = "up"
	stats["message"] = "It's healthy"

	// Get database stats (like open connections, in use, idle, etc.)
	dbStats := db.Sql.Stats()
	stats["open_connections"] = strconv.Itoa(dbStats.OpenConnections)
	stats["in_use"] = strconv.Itoa(dbStats.InUse)
	stats["idle"] = strconv.Itoa(dbStats.Idle)
	stats["wait_count"] = strconv.FormatInt(dbStats.WaitCount, 10)
	stats["wait_duration"] = dbStats.WaitDuration.String()
	stats["max_idle_closed"] = strconv.FormatInt(dbStats.MaxIdleClosed, 10)
	stats["max_lifetime_closed"] = strconv.FormatInt(dbStats.MaxLifetimeClosed, 10)

	// Evaluate stats to provide a health message
	if dbStats.OpenConnections > 40 { // Assuming 50 is the max for this example
		stats["message"] = "The database is experiencing heavy load."
	}

	if dbStats.WaitCount > 1000 {
		stats["message"] = "The database has a high number of wait events, indicating potential bottlenecks."
	}

	if dbStats.MaxIdleClosed > int64(dbStats.OpenConnections)/2 {
		stats["message"] = "Many idle connections are being closed, consider revising the connection pool settings."
	}

	if dbStats.MaxLifetimeClosed > int64(dbStats.OpenConnections)/2 {
		stats["message"] = "Many connections are being closed due to max lifetime, consider increasing max lifetime or revising the connection usage pattern."
	}

	return stats
}

// func (db *postgresDatabase) GetAllComments() ([]models.Comment, error) {
// 	comments := []models.Comment{}
// 	fmt.Println("Gettings All Data")

// 	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
// 	defer cancel()

// 	rows, err := db.Sql.QueryContext(ctx, `SELECT * FROM comments;`)
// 	if err != nil {
// 		fmt.Println(err)
// 		return nil, err
// 	}
// 	defer rows.Close()

// 	for rows.Next() {
// 		comment := models.Comment{}

// 		if err := rows.Scan(
// 			&comment.ID,
// 			&comment.Comment,
// 		); err != nil {
// 			fmt.Println(err)
// 			return nil, err
// 		}
// 		fmt.Println(comment)

// 		comments = append(comments, comment)
// 	}

// 	fmt.Printf("Data has value %+v\n\n", comments)
// 	return comments, nil

// }
