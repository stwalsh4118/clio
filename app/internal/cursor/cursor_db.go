package cursor

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/stwalsh4118/clio/internal/config"
	_ "modernc.org/sqlite" // SQLite driver
)

var (
	// connectionCounter tracks how many Cursor database connections have been created
	connectionCounter int64
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

	// Track connection creation for diagnostics
	connNum := atomic.AddInt64(&connectionCounter, 1)

	// Get caller information for diagnostics
	pc, file, line, ok := runtime.Caller(1)
	caller := "unknown"
	if ok {
		fn := runtime.FuncForPC(pc)
		if fn != nil {
			caller = fmt.Sprintf("%s:%d (%s)", filepath.Base(file), line, fn.Name())
		}
	}

	// Open database in read-only mode to avoid locking issues with Cursor
	// Add busy_timeout to handle concurrent access (5 seconds = 5000ms)
	// This allows SQLite to retry when the database is locked by Cursor or other processes
	dsn := fmt.Sprintf("file:%s?mode=ro&_busy_timeout=5000", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open Cursor database: %w", err)
	}

	// Configure connection pool settings to help diagnose issues
	db.SetMaxOpenConns(1) // SQLite doesn't handle concurrent connections well
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0) // Keep connections alive

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping Cursor database: %w", err)
	}

	// Log connection creation for diagnostics (only log first few and then periodically)
	if connNum <= 5 || connNum%10 == 0 {
		stats := db.Stats()
		fmt.Printf("[DIAG] Cursor DB connection #%d created by %s (OpenConns: %d, InUse: %d, Idle: %d)\n",
			connNum, caller, stats.OpenConnections, stats.InUse, stats.Idle)
	}

	return db, nil
}

// IsSQLiteBusyError checks if an error is a SQLite busy/locked error
func IsSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "SQLITE_BUSY") || strings.Contains(errStr, "database is locked")
}

// LogSQLiteBusyDiagnostics logs diagnostic information when a SQLITE_BUSY error occurs
func LogSQLiteBusyDiagnostics(err error, component string, operation string) {
	if !IsSQLiteBusyError(err) {
		return
	}

	// Get stack trace (limit to 10 frames)
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	stack := string(buf[:n])

	// Extract relevant goroutine info
	lines := strings.Split(stack, "\n")
	var goroutineInfo []string
	for i, line := range lines {
		if i < 20 { // Limit to first 20 lines
			goroutineInfo = append(goroutineInfo, line)
		} else {
			break
		}
	}

	fmt.Printf("[DIAG] SQLITE_BUSY error detected:\n")
	fmt.Printf("  Component: %s\n", component)
	fmt.Printf("  Operation: %s\n", operation)
	fmt.Printf("  Error: %v\n", err)
	fmt.Printf("  Total connections created: %d\n", atomic.LoadInt64(&connectionCounter))
	fmt.Printf("  Stack trace:\n%s\n", strings.Join(goroutineInfo, "\n"))
}
