package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/doug-benn/mqtt-logger/database"
	"github.com/doug-benn/mqtt-logger/repository/postgres"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	db, err := database.NewDatabase(context.Background(), logger, database.DefaultConfig())
	if err != nil {
		logger.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	runMQTT(postgres.NewRainfallRepo(db.Pool()))
}
