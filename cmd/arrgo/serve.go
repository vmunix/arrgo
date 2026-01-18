package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"

	"github.com/arrgo/arrgo/internal/config"
	"github.com/arrgo/arrgo/internal/migrations"
)

func runServe(configPath string) error {
	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Ensure database directory exists
	dbDir := filepath.Dir(cfg.Database.Path)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite3", cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	// Run migrations
	if _, err := db.Exec(migrations.InitialSQL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	fmt.Printf("arrgo starting on %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	return nil
}
