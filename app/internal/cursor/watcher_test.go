package cursor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stwalsh4118/clio/internal/config"
)

func TestNewWatcher(t *testing.T) {
	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: "/tmp/test/cursor",
		},
	}

	watcher, err := NewWatcher(cfg)
	if err != nil {
		t.Fatalf("NewWatcher() error = %v, want nil", err)
	}

	if watcher == nil {
		t.Fatal("NewWatcher() returned nil watcher")
	}

	// Test nil config
	_, err = NewWatcher(nil)
	if err == nil {
		t.Error("NewWatcher(nil) expected error, got nil")
	}
}

func TestWatcher_PathConstruction(t *testing.T) {
	// Test that path construction works correctly by verifying Start() behavior
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, "cursor")
	globalStorageDir := filepath.Join(cursorDir, "globalStorage")
	dbFile := filepath.Join(globalStorageDir, "state.vscdb")

	if err := os.MkdirAll(globalStorageDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	// Create empty database file
	if err := os.WriteFile(dbFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test database file: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: cursorDir,
		},
	}

	watcher, err := NewWatcher(cfg)
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	// If path construction is correct, Start() should succeed
	if err := watcher.Start(); err != nil {
		t.Fatalf("Start() error = %v (path construction may be incorrect)", err)
	}
	watcher.Stop()
}

func TestWatcher_StartStop(t *testing.T) {
	// Create temporary directory structure
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, "cursor")
	globalStorageDir := filepath.Join(cursorDir, "globalStorage")
	dbFile := filepath.Join(globalStorageDir, "state.vscdb")

	if err := os.MkdirAll(globalStorageDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	// Create empty database file
	if err := os.WriteFile(dbFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test database file: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: cursorDir,
		},
	}

	watcher, err := NewWatcher(cfg)
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	// Test Watch before start (should fail)
	_, err = watcher.Watch()
	if err == nil {
		t.Error("Watch() before Start() expected error, got nil")
	}

	// Test Start
	if err := watcher.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Start again (should fail)
	if err := watcher.Start(); err == nil {
		t.Error("Start() twice expected error, got nil")
	}

	// Test Stop
	if err := watcher.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Stop again (should be safe)
	if err := watcher.Stop(); err != nil {
		t.Errorf("Stop() twice error = %v, want nil", err)
	}
}

func TestWatcher_StartWithMissingFile(t *testing.T) {
	// Create temporary directory structure without database file
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, "cursor")
	globalStorageDir := filepath.Join(cursorDir, "globalStorage")

	if err := os.MkdirAll(globalStorageDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: cursorDir,
		},
	}

	watcher, err := NewWatcher(cfg)
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	// Should start successfully by watching parent directory
	if err := watcher.Start(); err != nil {
		t.Fatalf("Start() with missing file error = %v", err)
	}

	// Clean up
	_ = watcher.Stop()
}

func TestWatcher_StartWithMissingParent(t *testing.T) {
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, "nonexistent", "cursor")

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: cursorDir,
		},
	}

	watcher, err := NewWatcher(cfg)
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	// Should fail because parent directory doesn't exist
	if err := watcher.Start(); err == nil {
		t.Error("Start() with missing parent directory expected error, got nil")
	}
}

func TestWatcher_EventDetection(t *testing.T) {
	// Create temporary directory structure
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, "cursor")
	globalStorageDir := filepath.Join(cursorDir, "globalStorage")
	dbFile := filepath.Join(globalStorageDir, "state.vscdb")

	if err := os.MkdirAll(globalStorageDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	// Create empty database file
	if err := os.WriteFile(dbFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test database file: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: cursorDir,
		},
	}

	watcher, err := NewWatcher(cfg)
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	if err := watcher.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer watcher.Stop()

	events, err := watcher.Watch()
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Modify the database file
	time.Sleep(100 * time.Millisecond) // Give watcher time to set up
	if err := os.WriteFile(dbFile, []byte("modified"), 0644); err != nil {
		t.Fatalf("Failed to modify database file: %v", err)
	}

	// Wait for event with timeout
	select {
	case event := <-events:
		if event.Path != dbFile {
			t.Errorf("Event.Path = %v, want %v", event.Path, dbFile)
		}
		if event.EventType != "WRITE" {
			t.Errorf("Event.EventType = %v, want WRITE", event.EventType)
		}
		if event.Timestamp.IsZero() {
			t.Error("Event.Timestamp is zero")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for file modification event")
	}
}

func TestWatcher_EventFiltering(t *testing.T) {
	// Create temporary directory structure
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, "cursor")
	globalStorageDir := filepath.Join(cursorDir, "globalStorage")
	dbFile := filepath.Join(globalStorageDir, "state.vscdb")
	otherFile := filepath.Join(globalStorageDir, "other.vscdb")

	if err := os.MkdirAll(globalStorageDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	// Create database file
	if err := os.WriteFile(dbFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test database file: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: cursorDir,
		},
	}

	watcher, err := NewWatcher(cfg)
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	if err := watcher.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer watcher.Stop()

	events, err := watcher.Watch()
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Modify other file (should be filtered out)
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(otherFile, []byte("other"), 0644); err != nil {
		t.Fatalf("Failed to create other file: %v", err)
	}

	// Modify database file (should trigger event)
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(dbFile, []byte("modified"), 0644); err != nil {
		t.Fatalf("Failed to modify database file: %v", err)
	}

	// Wait for event - should only get event for dbFile
	select {
	case event := <-events:
		if event.Path != dbFile {
			t.Errorf("Event.Path = %v, want %v (should filter out other file)", event.Path, dbFile)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for file modification event")
	}
}

func TestWatcher_FileCreation(t *testing.T) {
	// Create temporary directory structure without database file
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, "cursor")
	globalStorageDir := filepath.Join(cursorDir, "globalStorage")
	dbFile := filepath.Join(globalStorageDir, "state.vscdb")

	if err := os.MkdirAll(globalStorageDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: cursorDir,
		},
	}

	watcher, err := NewWatcher(cfg)
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	if err := watcher.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer watcher.Stop()

	events, err := watcher.Watch()
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Create the database file
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(dbFile, []byte("created"), 0644); err != nil {
		t.Fatalf("Failed to create database file: %v", err)
	}

	// Wait for CREATE event
	select {
	case event := <-events:
		if event.Path != dbFile {
			t.Errorf("Event.Path = %v, want %v", event.Path, dbFile)
		}
		if event.EventType != "CREATE" {
			t.Errorf("Event.EventType = %v, want CREATE", event.EventType)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for file creation event")
	}
}

func TestWatcher_ConcurrentEvents(t *testing.T) {
	// Create temporary directory structure
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, "cursor")
	globalStorageDir := filepath.Join(cursorDir, "globalStorage")
	dbFile := filepath.Join(globalStorageDir, "state.vscdb")

	if err := os.MkdirAll(globalStorageDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	if err := os.WriteFile(dbFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test database file: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: cursorDir,
		},
	}

	watcher, err := NewWatcher(cfg)
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	if err := watcher.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer watcher.Stop()

	events, err := watcher.Watch()
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Trigger multiple rapid modifications
	time.Sleep(100 * time.Millisecond)
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(dbFile, []byte("modified"), 0644); err != nil {
			t.Fatalf("Failed to modify database file: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Should receive at least one event (may receive multiple due to rapid writes)
	eventCount := 0
	timeout := time.After(2 * time.Second)
	for eventCount < 1 {
		select {
		case <-events:
			eventCount++
		case <-timeout:
			if eventCount == 0 {
				t.Error("No events received from concurrent modifications")
			}
			return
		}
	}
}
