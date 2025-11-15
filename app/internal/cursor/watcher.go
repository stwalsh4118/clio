package cursor

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stwalsh4118/clio/internal/config"
)

// FileEvent represents a file system event detected by the watcher
type FileEvent struct {
	Path      string    // Full path to the file
	EventType string    // Type of event (e.g., "WRITE", "CREATE")
	Timestamp time.Time // When the event occurred
}

// WatcherService defines the interface for file system watching
type WatcherService interface {
	Start() error
	Stop() error
	Watch() (<-chan FileEvent, error)
}

// watcher implements WatcherService for monitoring Cursor database file
type watcher struct {
	config    *config.Config
	fsWatcher *fsnotify.Watcher
	events    chan FileEvent
	done      chan struct{}
	mu        sync.Mutex
	started   bool
	dbPath    string // Full path to the database file being watched
}

// NewWatcher creates a new file system watcher instance
func NewWatcher(cfg *config.Config) (WatcherService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Construct database file path
	dbPath := filepath.Join(cfg.Cursor.LogPath, "globalStorage", "state.vscdb")

	return &watcher{
		config:  cfg,
		events:  make(chan FileEvent, 100), // Buffered channel for event bursts
		done:    make(chan struct{}),
		dbPath:  dbPath,
		started: false,
	}, nil
}

// Start begins watching the database file for modifications
func (w *watcher) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return fmt.Errorf("watcher is already started")
	}

	// Create fsnotify watcher
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}
	w.fsWatcher = fsWatcher

	// Determine what to watch
	watchPath, err := w.determineWatchPath()
	if err != nil {
		w.fsWatcher.Close()
		return fmt.Errorf("failed to determine watch path: %w", err)
	}

	// Add watch
	if err := w.fsWatcher.Add(watchPath); err != nil {
		w.fsWatcher.Close()
		return fmt.Errorf("failed to add watch for %s: %w", watchPath, err)
	}

	// Start event processing goroutine
	go w.processEvents()

	w.started = true
	return nil
}

// determineWatchPath determines what path to watch based on file existence
func (w *watcher) determineWatchPath() (string, error) {
	// Check if database file exists
	_, err := os.Stat(w.dbPath)
	if err == nil {
		// File exists - watch the file directly
		return w.dbPath, nil
	}

	if !os.IsNotExist(err) {
		// Some other error checking the file
		return "", fmt.Errorf("failed to check database file: %w", err)
	}

	// File doesn't exist - watch parent directory
	parentDir := filepath.Dir(w.dbPath)
	_, err = os.Stat(parentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("parent directory does not exist: %s", parentDir)
		}
		return "", fmt.Errorf("failed to check parent directory: %w", err)
	}

	return parentDir, nil
}

// processEvents processes fsnotify events in a separate goroutine
func (w *watcher) processEvents() {
	defer close(w.events)

	for {
		select {
		case <-w.done:
			return
		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			// Log error but continue monitoring
			// In a real implementation, we'd use a logger here
			_ = err
			// Attempt to re-establish watch if needed
			w.recoverWatch()
		}
	}
}

// handleEvent processes a single fsnotify event
func (w *watcher) handleEvent(event fsnotify.Event) {
	// Normalize paths for comparison
	eventPath, err := filepath.Abs(event.Name)
	if err != nil {
		return
	}
	dbPath, err := filepath.Abs(w.dbPath)
	if err != nil {
		return
	}

	// Check if this event is for our database file
	if eventPath != dbPath {
		// If we're watching the parent directory, check if this is our file
		parentDir := filepath.Dir(dbPath)
		if filepath.Dir(eventPath) != parentDir {
			return
		}
		// Check if the filename matches
		if filepath.Base(eventPath) != "state.vscdb" {
			return
		}
		// This is our file - update dbPath to the actual event path
		// (in case file was just created)
		dbPath = eventPath
	}

	// Filter events - only process WRITE and CREATE
	if event.Op&fsnotify.Write == 0 && event.Op&fsnotify.Create == 0 {
		return
	}

	// Convert to FileEvent
	fileEvent := FileEvent{
		Path:      w.dbPath, // Always use the configured path
		EventType: w.mapEventType(event.Op),
		Timestamp: time.Now(),
	}

	// If file was created and we were watching parent, switch to watching the file
	if event.Op&fsnotify.Create != 0 && filepath.Dir(eventPath) == filepath.Dir(dbPath) {
		w.switchToFileWatch()
	}

	// Send event (non-blocking due to buffered channel)
	select {
	case w.events <- fileEvent:
	default:
		// Channel full - log warning but don't block
		// In a real implementation, we'd use a logger here
	}
}

// mapEventType converts fsnotify event operation to string
func (w *watcher) mapEventType(op fsnotify.Op) string {
	if op&fsnotify.Create != 0 {
		return "CREATE"
	}
	if op&fsnotify.Write != 0 {
		return "WRITE"
	}
	return "UNKNOWN"
}

// switchToFileWatch switches from watching parent directory to watching the file directly
func (w *watcher) switchToFileWatch() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.started || w.fsWatcher == nil {
		return
	}

	// Check if file exists now
	_, err := os.Stat(w.dbPath)
	if err != nil {
		return
	}

	// Remove parent directory watch
	parentDir := filepath.Dir(w.dbPath)
	_ = w.fsWatcher.Remove(parentDir)

	// Add file watch
	if err := w.fsWatcher.Add(w.dbPath); err != nil {
		// Failed - try to restore parent watch
		_ = w.fsWatcher.Add(parentDir)
	}
}

// recoverWatch attempts to re-establish the watch if it was lost
func (w *watcher) recoverWatch() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.started {
		return
	}

	// Try to re-add the watch path
	watchPath, err := w.determineWatchPath()
	if err != nil {
		// Can't recover - log error
		return
	}

	// Remove old watch and add new one
	_ = w.fsWatcher.Remove(watchPath) // Ignore error if not watching
	if err := w.fsWatcher.Add(watchPath); err != nil {
		// Failed to re-establish - log error
		return
	}
}

// Stop stops watching and cleans up resources
func (w *watcher) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.started {
		return nil
	}

	// Signal shutdown
	close(w.done)

	// Close fsnotify watcher
	if w.fsWatcher != nil {
		if err := w.fsWatcher.Close(); err != nil {
			return fmt.Errorf("failed to close file watcher: %w", err)
		}
	}

	w.started = false
	return nil
}

// Watch returns the channel for receiving file events
func (w *watcher) Watch() (<-chan FileEvent, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.started {
		return nil, fmt.Errorf("watcher is not started")
	}

	return w.events, nil
}
