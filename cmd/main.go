package main

import (
	"log/slog"
	"os"
	// Embed the timezone database: the alpine runtime image has none and
	// APP_TIMEZONE (default Asia/Jakarta) must resolve.
	_ "time/tzdata"

	"hearable/backend/config"
	"hearable/backend/database"
	"hearable/backend/route"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "env.yaml"
	}
	cfg, err := config.New(configPath)
	if err != nil {
		logger.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	db, err := database.New(cfg.GetDatabaseURL())
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	route.Start(cfg, db, logger)
}
