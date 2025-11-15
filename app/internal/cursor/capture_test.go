package cursor

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stwalsh4118/clio/internal/config"
	"github.com/stwalsh4118/clio/internal/db"
	_ "modernc.org/sqlite"
)

func TestNewCaptureService(t *testing.T) {
	// Create test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	testDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer testDB.Close()

	// Run migrations
	if err := db.RunMigrations(testDB); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
		Storage: config.StorageConfig{
			DatabasePath: dbPath,
		},
		Session: config.SessionConfig{
			InactivityTimeoutMinutes: 30,
		},
	}

	// Create cursor directory structure
	cursorDir := filepath.Join(tmpDir, "globalStorage")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatalf("Failed to create cursor directory: %v", err)
	}

	service, err := NewCaptureService(cfg, testDB)
	if err != nil {
		t.Fatalf("NewCaptureService() error = %v, want nil", err)
	}

	if service == nil {
		t.Fatal("NewCaptureService() returned nil service")
	}
}

func TestNewCaptureService_NilConfig(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	testDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer testDB.Close()

	_, err = NewCaptureService(nil, testDB)
	if err == nil {
		t.Error("NewCaptureService(nil, db) expected error, got nil")
	}
}

func TestNewCaptureService_NilDatabase(t *testing.T) {
	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: "/tmp/test",
		},
	}

	_, err := NewCaptureService(cfg, nil)
	if err == nil {
		t.Error("NewCaptureService(cfg, nil) expected error, got nil")
	}
}

func TestNewCaptureService_NoCursorLogPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	testDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer testDB.Close()

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: "", // Empty log path
		},
	}

	_, err = NewCaptureService(cfg, testDB)
	if err == nil {
		t.Error("NewCaptureService() with empty log path expected error, got nil")
	}
}

func TestCaptureService_StartStop(t *testing.T) {
	// Create test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	testDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer testDB.Close()

	// Run migrations
	if err := db.RunMigrations(testDB); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create cursor directory structure
	cursorDir := filepath.Join(tmpDir, "globalStorage")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatalf("Failed to create cursor directory: %v", err)
	}

	// Create empty database file for watcher
	dbFile := filepath.Join(cursorDir, "state.vscdb")
	if err := os.WriteFile(dbFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test database file: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
		Storage: config.StorageConfig{
			DatabasePath: dbPath,
		},
		Session: config.SessionConfig{
			InactivityTimeoutMinutes: 30,
		},
	}

	service, err := NewCaptureService(cfg, testDB)
	if err != nil {
		t.Fatalf("NewCaptureService() error = %v", err)
	}

	// Start service
	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop service
	if err := service.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestCaptureService_StartTwice(t *testing.T) {
	// Create test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	testDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer testDB.Close()

	// Run migrations
	if err := db.RunMigrations(testDB); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create cursor directory structure
	cursorDir := filepath.Join(tmpDir, "globalStorage")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatalf("Failed to create cursor directory: %v", err)
	}

	// Create empty database file for watcher
	dbFile := filepath.Join(cursorDir, "state.vscdb")
	if err := os.WriteFile(dbFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test database file: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
		Storage: config.StorageConfig{
			DatabasePath: dbPath,
		},
		Session: config.SessionConfig{
			InactivityTimeoutMinutes: 30,
		},
	}

	service, err := NewCaptureService(cfg, testDB)
	if err != nil {
		t.Fatalf("NewCaptureService() error = %v", err)
	}

	// Start service
	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Try to start again (should fail)
	if err := service.Start(); err == nil {
		t.Error("Start() twice expected error, got nil")
	}

	// Stop service
	if err := service.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestCaptureService_StopWithoutStart(t *testing.T) {
	// Create test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	testDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer testDB.Close()

	// Run migrations
	if err := db.RunMigrations(testDB); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create cursor directory structure
	cursorDir := filepath.Join(tmpDir, "globalStorage")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatalf("Failed to create cursor directory: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
		Storage: config.StorageConfig{
			DatabasePath: dbPath,
		},
		Session: config.SessionConfig{
			InactivityTimeoutMinutes: 30,
		},
	}

	service, err := NewCaptureService(cfg, testDB)
	if err != nil {
		t.Fatalf("NewCaptureService() error = %v", err)
	}

	// Stop without starting (should not error)
	if err := service.Stop(); err != nil {
		t.Errorf("Stop() without Start() error = %v, want nil", err)
	}
}

func TestCaptureService_StopTwice(t *testing.T) {
	// Create test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	testDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer testDB.Close()

	// Run migrations
	if err := db.RunMigrations(testDB); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create cursor directory structure
	cursorDir := filepath.Join(tmpDir, "globalStorage")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatalf("Failed to create cursor directory: %v", err)
	}

	// Create empty database file for watcher
	dbFile := filepath.Join(cursorDir, "state.vscdb")
	if err := os.WriteFile(dbFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test database file: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
		Storage: config.StorageConfig{
			DatabasePath: dbPath,
		},
		Session: config.SessionConfig{
			InactivityTimeoutMinutes: 30,
		},
	}

	service, err := NewCaptureService(cfg, testDB)
	if err != nil {
		t.Fatalf("NewCaptureService() error = %v", err)
	}

	// Start service
	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Stop service
	if err := service.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Stop again (should not error)
	if err := service.Stop(); err != nil {
		t.Errorf("Stop() twice error = %v, want nil", err)
	}
}

// createTestCursorDatabaseForCapture creates a test Cursor database with sample conversation data
func createTestCursorDatabaseForCapture(t *testing.T, dbPath string, composerIDs []string) {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Open database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	// Create cursorDiskKV table
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS cursorDiskKV (
		key TEXT UNIQUE ON CONFLICT REPLACE,
		value BLOB
	);`
	if _, err := db.Exec(createTableSQL); err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Create composer data for each composer ID
	for _, composerID := range composerIDs {
		composerData := map[string]interface{}{
			"composerId": composerID,
			"name":       "Test Conversation " + composerID,
			"status":     "completed",
			"createdAt":  1704067200000, // Unix milliseconds: 2024-01-01 00:00:00 UTC
			"fullConversationHeadersOnly": []map[string]interface{}{
				{"bubbleId": "bubble-1-" + composerID, "type": 1},
				{"bubbleId": "bubble-2-" + composerID, "type": 2},
			},
		}
		composerJSON, _ := json.Marshal(composerData)
		composerKey := "composerData:" + composerID
		if _, err := db.Exec("INSERT INTO cursorDiskKV (key, value) VALUES (?, ?)", composerKey, composerJSON); err != nil {
			t.Fatalf("Failed to insert composer data: %v", err)
		}

		// Create message bubbles
		bubbles := []struct {
			bubbleID  string
			msgType   int
			text      string
			createdAt string
		}{
			{"bubble-1-" + composerID, 1, "Hello from " + composerID, "2024-01-01T12:00:00.000Z"},
			{"bubble-2-" + composerID, 2, "Response for " + composerID, "2024-01-01T12:00:15.000Z"},
		}

		for _, bubble := range bubbles {
			bubbleData := map[string]interface{}{
				"bubbleId":  bubble.bubbleID,
				"type":      bubble.msgType,
				"text":      bubble.text,
				"createdAt": bubble.createdAt,
			}
			bubbleJSON, _ := json.Marshal(bubbleData)
			bubbleKey := "bubbleId:" + composerID + ":" + bubble.bubbleID
			if _, err := db.Exec("INSERT INTO cursorDiskKV (key, value) VALUES (?, ?)", bubbleKey, bubbleJSON); err != nil {
				t.Fatalf("Failed to insert bubble data: %v", err)
			}
		}
	}
}

func TestCaptureService_InitialScan_ProcessesUnprocessedConversations(t *testing.T) {
	// Create test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	testDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer testDB.Close()

	// Run migrations
	if err := db.RunMigrations(testDB); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create cursor directory structure
	cursorDir := filepath.Join(tmpDir, "globalStorage")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatalf("Failed to create cursor directory: %v", err)
	}

	// Create Cursor database with test conversations
	cursorDBPath := filepath.Join(cursorDir, "state.vscdb")
	composerIDs := []string{"composer-1", "composer-2", "composer-3"}
	createTestCursorDatabaseForCapture(t, cursorDBPath, composerIDs)

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
		Storage: config.StorageConfig{
			DatabasePath: dbPath,
		},
		Session: config.SessionConfig{
			InactivityTimeoutMinutes: 30,
		},
	}

	service, err := NewCaptureService(cfg, testDB)
	if err != nil {
		t.Fatalf("NewCaptureService() error = %v", err)
	}

	// Start service (this will trigger initial scan)
	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give initial scan time to complete
	time.Sleep(500 * time.Millisecond)

	// Verify conversations were processed by checking processed_conversations table
	for _, composerID := range composerIDs {
		var messageCount int
		err := testDB.QueryRow("SELECT message_count FROM processed_conversations WHERE composer_id = ?", composerID).Scan(&messageCount)
		if err != nil {
			t.Errorf("Conversation %s was not processed: %v", composerID, err)
		}
		if messageCount != 2 {
			t.Errorf("Expected message count 2 for %s, got %d", composerID, messageCount)
		}
	}

	// Stop service
	if err := service.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestCaptureService_InitialScan_SkipsAlreadyProcessedConversations(t *testing.T) {
	// Create test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	testDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer testDB.Close()

	// Run migrations
	if err := db.RunMigrations(testDB); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create cursor directory structure
	cursorDir := filepath.Join(tmpDir, "globalStorage")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatalf("Failed to create cursor directory: %v", err)
	}

	// Create Cursor database with test conversations
	cursorDBPath := filepath.Join(cursorDir, "state.vscdb")
	composerIDs := []string{"composer-1", "composer-2"}
	createTestCursorDatabaseForCapture(t, cursorDBPath, composerIDs)

	// Mark one conversation as already processed
	_, err = testDB.Exec("INSERT INTO processed_conversations (composer_id, message_count, last_processed_at) VALUES (?, ?, ?)",
		"composer-1", 2, time.Now())
	if err != nil {
		t.Fatalf("Failed to mark conversation as processed: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
		Storage: config.StorageConfig{
			DatabasePath: dbPath,
		},
		Session: config.SessionConfig{
			InactivityTimeoutMinutes: 30,
		},
	}

	service, err := NewCaptureService(cfg, testDB)
	if err != nil {
		t.Fatalf("NewCaptureService() error = %v", err)
	}

	// Start service (this will trigger initial scan)
	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give initial scan time to complete
	time.Sleep(500 * time.Millisecond)

	// Verify composer-1 still has message_count 2 (wasn't reprocessed)
	var messageCount int
	err = testDB.QueryRow("SELECT message_count FROM processed_conversations WHERE composer_id = ?", "composer-1").Scan(&messageCount)
	if err != nil {
		t.Fatalf("Failed to query processed conversation: %v", err)
	}
	if messageCount != 2 {
		t.Errorf("Expected message count 2 for composer-1, got %d", messageCount)
	}

	// Verify composer-2 was processed
	err = testDB.QueryRow("SELECT message_count FROM processed_conversations WHERE composer_id = ?", "composer-2").Scan(&messageCount)
	if err != nil {
		t.Errorf("Conversation composer-2 was not processed: %v", err)
	}
	if messageCount != 2 {
		t.Errorf("Expected message count 2 for composer-2, got %d", messageCount)
	}

	// Stop service
	if err := service.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestCaptureService_InitialScan_EmptyDatabase(t *testing.T) {
	// Create test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	testDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer testDB.Close()

	// Run migrations
	if err := db.RunMigrations(testDB); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create cursor directory structure
	cursorDir := filepath.Join(tmpDir, "globalStorage")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatalf("Failed to create cursor directory: %v", err)
	}

	// Create empty Cursor database
	cursorDBPath := filepath.Join(cursorDir, "state.vscdb")
	db, err := sql.Open("sqlite", cursorDBPath)
	if err != nil {
		t.Fatalf("Failed to open Cursor database: %v", err)
	}
	defer db.Close()

	// Create cursorDiskKV table
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS cursorDiskKV (
		key TEXT UNIQUE ON CONFLICT REPLACE,
		value BLOB
	);`
	if _, err := db.Exec(createTableSQL); err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
		Storage: config.StorageConfig{
			DatabasePath: dbPath,
		},
		Session: config.SessionConfig{
			InactivityTimeoutMinutes: 30,
		},
	}

	service, err := NewCaptureService(cfg, testDB)
	if err != nil {
		t.Fatalf("NewCaptureService() error = %v", err)
	}

	// Start service (this will trigger initial scan with empty database)
	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give initial scan time to complete
	time.Sleep(200 * time.Millisecond)

	// Stop service
	if err := service.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

