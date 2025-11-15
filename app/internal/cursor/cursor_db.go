package cursor

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/stwalsh4118/clio/internal/config"
	_ "modernc.org/sqlite" // SQLite driver
)

// OpenCursorDatabase opens the Cursor global state.vscdb database in read-only mode
// This is a shared helper function used by parser, updater, and other components
// that need to access Cursor's conversation database.
func OpenCursorDatabase(cfg *config.Config) (*sql.DB, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Construct database path
	dbPath := filepath.Join(cfg.Cursor.LogPath, "globalStorage", "state.vscdb")

	// Open database in read-only mode to avoid locking issues with Cursor
	dsn := fmt.Sprintf("file:%s?mode=ro", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open Cursor database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping Cursor database: %w", err)
	}

	return db, nil
}
