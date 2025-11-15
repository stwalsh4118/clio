package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/stwalsh4118/clio/internal/config"
	_ "modernc.org/sqlite" // SQLite driver
)

// Open opens a database connection and runs migrations
func Open(cfg *config.Config) (*sql.DB, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Get database path from config (already expanded by config loader)
	dbPath := cfg.Storage.DatabasePath
	if dbPath == "" {
		return nil, fmt.Errorf("database path not configured")
	}

	// Ensure database directory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Run migrations
	if err := RunMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}
