package cursor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/stwalsh4118/clio/internal/config"
	"github.com/stwalsh4118/clio/internal/logging"
)

const (
	// shutdownTimeout is the maximum time to wait for graceful shutdown
	shutdownTimeout = 10 * time.Second
	// progressLogInterval is how often to log progress during initial scan (every N conversations)
	progressLogInterval = 25
)

// CaptureService defines the interface for the Cursor conversation capture service
type CaptureService interface {
	Start() error
	Stop() error
}

// captureService orchestrates all Cursor capture components
type captureService struct {
	config          *config.Config
	db              *sql.DB
	logger          logging.Logger
	watcher         WatcherService
	parser          ParserService
	projectDetector ProjectDetector
	sessionManager  SessionManager
	storage         ConversationStorage
	updater         ConversationUpdater
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	started         bool
	mu              sync.Mutex
}

// NewCaptureService creates a new capture service instance
func NewCaptureService(cfg *config.Config, database *sql.DB) (CaptureService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if database == nil {
		return nil, fmt.Errorf("database cannot be nil")
	}

	// Create logger
	logger, err := logging.NewLogger(cfg)
	if err != nil {
		// If logger creation fails, use no-op logger (don't fail capture service creation)
		logger = logging.NewNoopLogger()
	}
	logger = logger.With("component", "capture_service")

	// Validate Cursor log path - if missing, return error (but allow daemon to continue)
	if cfg.Cursor.LogPath == "" {
		logger.Warn("cursor log path not configured, capture service will not be initialized")
		return nil, fmt.Errorf("cursor log path not configured")
	}

	ctx, cancel := context.WithCancel(context.Background())

	cs := &captureService{
		config:  cfg,
		db:      database,
		logger:  logger,
		ctx:     ctx,
		cancel:  cancel,
		started: false,
	}

	// Initialize all components
	if err := cs.initializeComponents(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize capture service components: %w", err)
	}

	return cs, nil
}

// initializeComponents initializes all capture service components
func (cs *captureService) initializeComponents() error {
	// Create watcher
	watcher, err := NewWatcher(cs.config)
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	cs.watcher = watcher

	// Create parser
	parser, err := NewParser(cs.config)
	if err != nil {
		return fmt.Errorf("failed to create parser: %w", err)
	}
	cs.parser = parser

	// Create project detector
	projectDetector, err := NewProjectDetector(cs.config)
	if err != nil {
		return fmt.Errorf("failed to create project detector: %w", err)
	}
	cs.projectDetector = projectDetector

	// Create storage
	storage, err := NewConversationStorage(cs.db, cs.logger)
	if err != nil {
		return fmt.Errorf("failed to create conversation storage: %w", err)
	}
	cs.storage = storage

	// Create session manager
	sessionManager, err := NewSessionManager(cs.config, cs.db)
	if err != nil {
		return fmt.Errorf("failed to create session manager: %w", err)
	}
	cs.sessionManager = sessionManager

	// Load existing sessions
	if err := cs.sessionManager.LoadSessions(); err != nil {
		cs.logger.Warn("failed to load existing sessions", "error", err)
		// Don't fail initialization - sessions will be created as needed
	}

	// Create updater
	updater, err := NewConversationUpdater(cs.config, cs.db, cs.parser, cs.storage, cs.sessionManager)
	if err != nil {
		return fmt.Errorf("failed to create conversation updater: %w", err)
	}
	cs.updater = updater

	cs.logger.Info("capture service components initialized")
	return nil
}

// Start starts the capture service
func (cs *captureService) Start() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.started {
		return fmt.Errorf("capture service is already started")
	}

	// Start watcher
	if err := cs.watcher.Start(); err != nil {
		return fmt.Errorf("failed to start watcher: %w", err)
	}

	// Start session manager inactivity monitor
	if err := cs.sessionManager.StartInactivityMonitor(cs.ctx); err != nil {
		cs.watcher.Stop()
		return fmt.Errorf("failed to start inactivity monitor: %w", err)
	}

	// Perform initial scan to capture existing conversations
	if err := cs.performInitialScan(); err != nil {
		// Log error but don't fail startup - continue with normal operation
		cs.logger.Error("initial scan failed, continuing with normal operation", "error", err)
	}

	// Get event channel from watcher
	events, err := cs.watcher.Watch()
	if err != nil {
		cs.sessionManager.Stop()
		cs.watcher.Stop()
		return fmt.Errorf("failed to get watcher events: %w", err)
	}

	// Start event processing goroutine
	cs.wg.Add(1)
	go cs.processEvents(events)

	cs.started = true
	cs.logger.Info("capture service started")
	return nil
}

// processEvents processes file system events from the watcher
func (cs *captureService) processEvents(events <-chan FileEvent) {
	defer cs.wg.Done()

	cs.logger.Info("event processing started")

	for {
		select {
		case <-cs.ctx.Done():
			cs.logger.Info("event processing stopped (shutdown requested)")
			return
		case event, ok := <-events:
			if !ok {
				cs.logger.Info("event channel closed, stopping event processing")
				return
			}

			// Process event asynchronously to avoid blocking
			cs.wg.Add(1)
			go cs.handleFileEvent(event)
		}
	}
}

// handleFileEvent handles a single file system event
func (cs *captureService) handleFileEvent(event FileEvent) {
	defer cs.wg.Done()

	cs.logger.Debug("processing file event", "event_type", event.EventType, "path", event.Path)

	// Detect updated composers
	updatedComposers, err := cs.updater.DetectUpdatedComposers()
	if err != nil {
		cs.logger.Error("failed to detect updated composers", "error", err)
		return
	}

	if len(updatedComposers) == 0 {
		cs.logger.Debug("no updated composers detected")
		return
	}

	cs.logger.Info("detected updated composers", "count", len(updatedComposers))

	// Process each updated composer
	for _, composerID := range updatedComposers {
		if err := cs.processComposer(composerID); err != nil {
			cs.logger.Error("failed to process composer", "composer_id", composerID, "error", err)
			// Continue processing other composers despite errors
		}
	}
}

// processComposer processes a single composer ID (new conversation or update)
func (cs *captureService) processComposer(composerID string) error {
	// Check if this is a new conversation or an update
	processedCount, err := cs.updater.GetProcessedMessageCount(composerID)
	if err != nil {
		cs.logger.Debug("failed to get processed count, treating as new conversation", "composer_id", composerID, "error", err)
		processedCount = 0
	}

	// Get current message count from Cursor database
	currentCount, err := cs.getCurrentMessageCount(composerID)
	if err != nil {
		return fmt.Errorf("failed to get current message count: %w", err)
	}

	// If already processed with same count, skip
	if processedCount >= currentCount {
		cs.logger.Debug("conversation already processed", "composer_id", composerID, "message_count", currentCount)
		return nil
	}

	// If not processed yet (processedCount == 0), treat as new conversation
	if processedCount == 0 {
		return cs.processNewConversation(composerID)
	}

	// Otherwise, treat as update
	return cs.updater.ProcessUpdate(composerID)
}

// processNewConversation processes a new conversation
func (cs *captureService) processNewConversation(composerID string) error {
	// Parse conversation
	conversation, err := cs.parser.ParseConversation(composerID)
	if err != nil {
		return fmt.Errorf("failed to parse conversation: %w", err)
	}

	if len(conversation.Messages) == 0 {
		cs.logger.Debug("conversation has no messages, skipping", "composer_id", composerID)
		return nil
	}

	// Detect project
	project, err := cs.projectDetector.DetectProject(conversation)
	if err != nil {
		cs.logger.Warn("failed to detect project, using default", "composer_id", composerID, "error", err)
		project = "unknown"
	}

	// Get or create session
	session, err := cs.sessionManager.GetOrCreateSession(project, conversation)
	if err != nil {
		return fmt.Errorf("failed to get or create session: %w", err)
	}

	// Mark as processed
	messageCount := len(conversation.Messages)
	if err := cs.updater.MarkAsProcessed(composerID, messageCount); err != nil {
		cs.logger.Warn("failed to mark conversation as processed", "composer_id", composerID, "error", err)
		// Don't fail - conversation was stored successfully
	}

	cs.logger.Info("processed new conversation", "composer_id", composerID, "project", project, "session_id", session.ID, "message_count", messageCount)
	return nil
}

// getCurrentMessageCount gets the current message count for a composer ID from Cursor database
func (cs *captureService) getCurrentMessageCount(composerID string) (int, error) {
	// Use updater's method to get message count
	cursorDB, err := OpenCursorDatabase(cs.config)
	if err != nil {
		return 0, err
	}
	defer cursorDB.Close()

	key := fmt.Sprintf("composerData:%s", composerID)
	query := "SELECT value FROM cursorDiskKV WHERE key = ?"

	var valueBlob []byte
	err = cursorDB.QueryRow(query, key).Scan(&valueBlob)
	if err != nil {
		return 0, fmt.Errorf("failed to query composer data: %w", err)
	}

	// Parse JSON to get message count
	var composerData struct {
		FullConversationHeadersOnly []struct {
			BubbleID string `json:"bubbleId"`
			Type     int    `json:"type"`
		} `json:"fullConversationHeadersOnly"`
	}

	if err := json.Unmarshal(valueBlob, &composerData); err != nil {
		return 0, fmt.Errorf("failed to parse composer data JSON: %w", err)
	}

	return len(composerData.FullConversationHeadersOnly), nil
}

// performInitialScan performs an initial scan of all existing conversations
// to capture any that haven't been processed yet
func (cs *captureService) performInitialScan() error {
	startTime := time.Now()
	cs.logger.Info("starting initial scan for existing conversations")

	// Get all composer IDs from Cursor database
	composerIDs, err := cs.parser.GetComposerIDs()
	if err != nil {
		return fmt.Errorf("failed to get composer IDs: %w", err)
	}

	totalFound := len(composerIDs)
	if totalFound == 0 {
		cs.logger.Info("initial scan completed - no conversations found")
		return nil
	}

	cs.logger.Info("initial scan found conversations", "total", totalFound)

	// Statistics
	var newProcessedCount int
	var skippedCount int
	var failedCount int

	// Process each composer ID
	for i, composerID := range composerIDs {
		// Check for shutdown request
		select {
		case <-cs.ctx.Done():
			cs.logger.Info("initial scan interrupted by shutdown request", "processed", newProcessedCount, "skipped", skippedCount, "failed", failedCount, "remaining", totalFound-i)
			return nil
		default:
		}

		// Check if already processed
		existingProcessedCount, err := cs.updater.GetProcessedMessageCount(composerID)
		if err != nil {
			// If error getting processed count, treat as unprocessed
			cs.logger.Debug("failed to get processed count, treating as unprocessed", "composer_id", composerID, "error", err)
			existingProcessedCount = 0
		}

		// Get current message count
		currentCount, err := cs.getCurrentMessageCount(composerID)
		if err != nil {
			cs.logger.Warn("failed to get message count for composer, skipping", "composer_id", composerID, "error", err)
			failedCount++
			continue
		}

		// If already processed with same or more messages, skip
		if existingProcessedCount >= currentCount {
			cs.logger.Debug("conversation already processed, skipping", "composer_id", composerID, "message_count", currentCount)
			skippedCount++
			continue
		}

		// Process the composer (handles both new and updated conversations)
		if err := cs.processComposer(composerID); err != nil {
			cs.logger.Warn("failed to process composer during initial scan", "composer_id", composerID, "error", err)
			failedCount++
			// Continue with next composer
			continue
		}

		newProcessedCount++

		// Log progress periodically
		if (i+1)%progressLogInterval == 0 || i == totalFound-1 {
			cs.logger.Info("initial scan progress", "processed", newProcessedCount, "skipped", skippedCount, "failed", failedCount, "total", totalFound, "progress", i+1)
		}
	}

	duration := time.Since(startTime)
	cs.logger.Info("initial scan completed", "total_found", totalFound, "processed", newProcessedCount, "skipped", skippedCount, "failed", failedCount, "duration", duration)

	return nil
}

// Stop stops the capture service gracefully
func (cs *captureService) Stop() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.started {
		return nil // Already stopped
	}

	cs.logger.Info("capture service shutdown initiated")

	// Cancel context to signal shutdown
	cs.cancel()

	// Stop watcher
	if cs.watcher != nil {
		if err := cs.watcher.Stop(); err != nil {
			cs.logger.Error("failed to stop watcher", "error", err)
		}
	}

	// Stop session manager (saves sessions)
	if cs.sessionManager != nil {
		if err := cs.sessionManager.Stop(); err != nil {
			cs.logger.Error("failed to stop session manager", "error", err)
		}
	}

	// Close parser database connection
	if cs.parser != nil {
		if err := cs.parser.Close(); err != nil {
			cs.logger.Error("failed to close parser", "error", err)
		}
	}

	// Wait for in-flight operations to complete (with timeout)
	done := make(chan struct{})
	go func() {
		cs.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		cs.logger.Info("capture service shutdown completed")
	case <-time.After(shutdownTimeout):
		cs.logger.Warn("capture service shutdown timeout, some operations may not have completed")
	}

	cs.started = false
	return nil
}
