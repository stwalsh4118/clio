package db

import (
	"path/filepath"
	"testing"

	"github.com/stwalsh4118/clio/internal/config"
)

func TestMigrations(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "migrations_test.db")

	cfg := &config.Config{
		Storage: config.StorageConfig{
			DatabasePath: dbPath,
		},
	}

	// Open database (this will run migrations)
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Verify sessions table exists
	var tableExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT name FROM sqlite_master 
			WHERE type='table' AND name='sessions'
		)
	`).Scan(&tableExists)
	if err != nil {
		t.Fatalf("Failed to check sessions table: %v", err)
	}

	if !tableExists {
		t.Error("Sessions table was not created")
	}

	// Verify indexes exist
	indexes := []string{
		"idx_sessions_project",
		"idx_sessions_start_time",
		"idx_sessions_active",
	}

	for _, indexName := range indexes {
		var indexExists bool
		err = db.QueryRow(`
			SELECT EXISTS (
				SELECT name FROM sqlite_master 
				WHERE type='index' AND name=?
			)
		`, indexName).Scan(&indexExists)
		if err != nil {
			t.Fatalf("Failed to check index %s: %v", indexName, err)
		}
		if !indexExists {
			t.Errorf("Index %s was not created", indexName)
		}
	}
}

func TestMigrations_Idempotent(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "migrations_idempotent_test.db")

	cfg := &config.Config{
		Storage: config.StorageConfig{
			DatabasePath: dbPath,
		},
	}

	// Open database (this will run migrations)
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database first time: %v", err)
	}
	db.Close()

	// Open again (should be idempotent)
	db, err = Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database second time: %v", err)
	}
	defer db.Close()

	// Verify sessions table still exists
	var tableExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT name FROM sqlite_master 
			WHERE type='table' AND name='sessions'
		)
	`).Scan(&tableExists)
	if err != nil {
		t.Fatalf("Failed to check sessions table: %v", err)
	}

	if !tableExists {
		t.Error("Sessions table should still exist after second migration run")
	}
}

func TestRollbackMigrations(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "rollback_test.db")

	cfg := &config.Config{
		Storage: config.StorageConfig{
			DatabasePath: dbPath,
		},
	}

	// Open database (this will run migrations)
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Verify sessions table exists
	var tableExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT name FROM sqlite_master 
			WHERE type='table' AND name='sessions'
		)
	`).Scan(&tableExists)
	if err != nil {
		t.Fatalf("Failed to check sessions table: %v", err)
	}

	if !tableExists {
		t.Fatal("Sessions table should exist before rollback")
	}

	// Rollback 1 migration
	newVersion, err := RollbackMigrations(db, 1)
	if err != nil {
		t.Fatalf("Failed to rollback migration: %v", err)
	}

	if newVersion != 0 {
		t.Errorf("Expected version 0 after rollback, got %d", newVersion)
	}

	// Verify sessions table no longer exists
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT name FROM sqlite_master 
			WHERE type='table' AND name='sessions'
		)
	`).Scan(&tableExists)
	if err != nil {
		t.Fatalf("Failed to check sessions table: %v", err)
	}

	if tableExists {
		t.Error("Sessions table should not exist after rollback")
	}
}
