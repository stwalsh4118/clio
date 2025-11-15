package cursor

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stwalsh4118/clio/internal/config"
	"github.com/stwalsh4118/clio/internal/logging"
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
	logger    logging.Logger
}

// NewWatcher creates a new file system watcher instance
func NewWatcher(cfg *config.Config) (WatcherService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Create logger (use component-specific logger)
	logger, err := logging.NewLogger(cfg)
	if err != nil {
		// If logger creation fails, use no-op logger (don't fail watcher creation)
		logger = logging.NewNoopLogger()
	}
	logger = logger.With("component", "watcher")

	// Construct database file path
	dbPath := filepath.Join(cfg.Cursor.LogPath, "globalStorage", "state.vscdb")

	return &watcher{
		config:  cfg,
		events:  make(chan FileEvent, 100), // Buffered channel for event bursts
		done:    make(chan struct{}),
		dbPath:  dbPath,
		started: false,
		logger:  logger,
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
		w.logger.Error("failed to create file watcher", "error", err)
		return fmt.Errorf("failed to create file watcher: %w", err)
	}
	w.fsWatcher = fsWatcher

	// Determine what to watch
	watchPath, err := w.determineWatchPath()
	if err != nil {
		w.fsWatcher.Close()
		w.logger.Error("failed to determine watch path", "error", err, "db_path", w.dbPath)
		return fmt.Errorf("failed to determine watch path: %w", err)
	}

	w.logger.Info("determined watch path", "watch_path", watchPath, "db_path", w.dbPath)

	// Add watch
	if err := w.fsWatcher.Add(watchPath); err != nil {
		w.fsWatcher.Close()
		w.logger.Error("failed to add watch", "error", err, "watch_path", watchPath)
		return fmt.Errorf("failed to add watch for %s: %w", watchPath, err)
	}

	// Start event processing goroutine
	go w.processEvents()

	w.started = true
	w.logger.Info("watcher started", "watch_path", watchPath, "db_path", w.dbPath)
	return nil
}

// determineWatchPath determines what path to watch based on file existence
func (w *watcher) determineWatchPath() (string, error) {
	// Check if database file exists
	_, err := os.Stat(w.dbPath)
	if err == nil {
		// File exists - watch the file directly
		w.logger.Debug("database file exists, watching file directly", "db_path", w.dbPath)
		return w.dbPath, nil
	}

	if !os.IsNotExist(err) {
		// Some other error checking the file
		w.logger.Error("failed to check database file", "error", err, "db_path", w.dbPath)
		return "", fmt.Errorf("failed to check database file: %w", err)
	}

	// File doesn't exist - watch parent directory
	w.logger.Debug("database file does not exist, watching parent directory", "db_path", w.dbPath)
	parentDir := filepath.Dir(w.dbPath)
	_, err = os.Stat(parentDir)
	if err != nil {
		if os.IsNotExist(err) {
			w.logger.Error("parent directory does not exist", "parent_dir", parentDir, "db_path", w.dbPath)
			return "", fmt.Errorf("parent directory does not exist: %s", parentDir)
		}
		w.logger.Error("failed to check parent directory", "error", err, "parent_dir", parentDir)
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
			w.logger.Warn("file watcher error", "error", err)
			// Attempt to re-establish watch if needed
			w.recoverWatch()
		}
	}
}

// handleEvent processes a single fsnotify event
func (w *watcher) handleEvent(event fsnotify.Event) {
	w.logger.Debug("received file system event", "event_name", event.Name, "event_op", event.Op.String())

	// Normalize paths for comparison
	eventPath, err := filepath.Abs(event.Name)
	if err != nil {
		w.logger.Debug("failed to normalize event path", "error", err, "event_name", event.Name)
		return
	}
	dbPath, err := filepath.Abs(w.dbPath)
	if err != nil {
		w.logger.Debug("failed to normalize database path", "error", err, "db_path", w.dbPath)
		return
	}

	// Check if this event is for our database file
	if eventPath != dbPath {
		// If we're watching the parent directory, check if this is our file
		parentDir := filepath.Dir(dbPath)
		if filepath.Dir(eventPath) != parentDir {
			w.logger.Debug("event filtered - different directory", "event_path", eventPath, "db_path", dbPath)
			return
		}
		// Check if the filename matches
		if filepath.Base(eventPath) != "state.vscdb" {
			w.logger.Debug("event filtered - different filename", "event_path", eventPath)
			return
		}
		// This is our file - update dbPath to the actual event path
		// (in case file was just created)
		dbPath = eventPath
	}

	// Filter events - only process WRITE and CREATE
	if event.Op&fsnotify.Write == 0 && event.Op&fsnotify.Create == 0 {
		w.logger.Debug("event filtered - not WRITE or CREATE", "event_op", event.Op.String())
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
		w.logger.Debug("file event sent", "event_type", fileEvent.EventType, "file_path", fileEvent.Path)
	default:
		// Channel full - log warning but don't block
		w.logger.Warn("event channel full, dropping event", "event_type", fileEvent.EventType, "file_path", fileEvent.Path)
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
		w.logger.Debug("file does not exist yet, cannot switch to file watch", "db_path", w.dbPath, "error", err)
		return
	}

	// Remove parent directory watch
	parentDir := filepath.Dir(w.dbPath)
	if err := w.fsWatcher.Remove(parentDir); err != nil {
		w.logger.Debug("failed to remove parent directory watch", "error", err, "parent_dir", parentDir)
	}

	// Add file watch
	if err := w.fsWatcher.Add(w.dbPath); err != nil {
		w.logger.Warn("failed to switch to file watch, restoring parent watch", "error", err, "db_path", w.dbPath)
		// Failed - try to restore parent watch
		if restoreErr := w.fsWatcher.Add(parentDir); restoreErr != nil {
			w.logger.Error("failed to restore parent watch after file watch failure", "error", restoreErr, "parent_dir", parentDir)
		}
		return
	}

	w.logger.Info("switched from parent directory watch to file watch", "db_path", w.dbPath)
}

// recoverWatch attempts to re-establish the watch if it was lost
func (w *watcher) recoverWatch() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.started {
		return
	}

	w.logger.Debug("attempting to recover watch")

	// Try to re-add the watch path
	watchPath, err := w.determineWatchPath()
	if err != nil {
		// Can't recover - log error
		w.logger.Error("failed to determine watch path during recovery", "error", err, "db_path", w.dbPath)
		return
	}

	// Remove old watch and add new one
	if err := w.fsWatcher.Remove(watchPath); err != nil {
		// Ignore error if not watching - this is expected if watch was already removed
		w.logger.Debug("failed to remove old watch during recovery (may be expected)", "error", err, "watch_path", watchPath)
	}
	if err := w.fsWatcher.Add(watchPath); err != nil {
		// Failed to re-establish - log error
		w.logger.Error("failed to re-establish watch during recovery", "error", err, "watch_path", watchPath)
		return
	}

	w.logger.Info("successfully recovered watch", "watch_path", watchPath)
}

// Stop stops watching and cleans up resources
func (w *watcher) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.started {
		return nil
	}

	w.logger.Info("stopping watcher", "db_path", w.dbPath)

	// Signal shutdown
	close(w.done)

	// Close fsnotify watcher
	if w.fsWatcher != nil {
		if err := w.fsWatcher.Close(); err != nil {
			w.logger.Error("failed to close file watcher", "error", err)
			return fmt.Errorf("failed to close file watcher: %w", err)
		}
	}

	w.started = false
	w.logger.Info("watcher stopped")
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
