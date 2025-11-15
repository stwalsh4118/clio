package cursor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/stwalsh4118/clio/internal/config"
	"github.com/stwalsh4118/clio/internal/logging"
)

// ConversationUpdater defines the interface for handling conversation updates
type ConversationUpdater interface {
	ProcessUpdate(composerID string) error
	HasBeenProcessed(composerID string, messageCount int) bool
	MarkAsProcessed(composerID string, messageCount int) error
	DetectUpdatedComposers() ([]string, error)
	GetProcessedMessageCount(composerID string) (int, error)
}

// conversationUpdater implements ConversationUpdater for detecting and processing conversation updates
type conversationUpdater struct {
	config         *config.Config
	db             *sql.DB // Our database for tracking processed conversations
	parser         ParserService
	storage        ConversationStorage
	sessionManager SessionManager
	logger         logging.Logger
}

// NewConversationUpdater creates a new conversation updater instance
func NewConversationUpdater(cfg *config.Config, db *sql.DB, parser ParserService, storage ConversationStorage, sessionManager SessionManager) (ConversationUpdater, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if db == nil {
		return nil, fmt.Errorf("database cannot be nil")
	}
	if parser == nil {
		return nil, fmt.Errorf("parser cannot be nil")
	}
	if storage == nil {
		return nil, fmt.Errorf("storage cannot be nil")
	}
	if sessionManager == nil {
		return nil, fmt.Errorf("session manager cannot be nil")
	}

	// Create logger
	logger, err := logging.NewLogger(cfg)
	if err != nil {
		logger = logging.NewNoopLogger()
	}
	logger = logger.With("component", "conversation_updater")

	return &conversationUpdater{
		config:         cfg,
		db:             db,
		parser:         parser,
		storage:        storage,
		sessionManager: sessionManager,
		logger:         logger,
	}, nil
}

// openCursorDatabase opens the Cursor SQLite database in read-only mode
func (u *conversationUpdater) openCursorDatabase() (*sql.DB, error) {
	return OpenCursorDatabase(u.config)
}

// getComposerMessageCount gets the current message count for a composer ID from Cursor database
func (u *conversationUpdater) getComposerMessageCount(composerID string) (int, error) {
	cursorDB, err := u.openCursorDatabase()
	if err != nil {
		return 0, err
	}
	defer cursorDB.Close()

	key := fmt.Sprintf("composerData:%s", composerID)
	query := "SELECT value FROM cursorDiskKV WHERE key = ?"

	var valueBlob []byte
	err = cursorDB.QueryRow(query, key).Scan(&valueBlob)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("composer data not found for ID: %s", composerID)
		}
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

// GetProcessedMessageCount retrieves the processed message count for a composer ID
func (u *conversationUpdater) GetProcessedMessageCount(composerID string) (int, error) {
	var messageCount int
	err := u.db.QueryRow(
		"SELECT message_count FROM processed_conversations WHERE composer_id = ?",
		composerID,
	).Scan(&messageCount)

	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil // Not processed yet
		}
		return 0, fmt.Errorf("failed to get processed message count: %w", err)
	}

	return messageCount, nil
}

// HasBeenProcessed checks if a composer ID has been processed with the given message count
func (u *conversationUpdater) HasBeenProcessed(composerID string, messageCount int) bool {
	processedCount, err := u.GetProcessedMessageCount(composerID)
	if err != nil {
		return false
	}
	return processedCount >= messageCount
}

// MarkAsProcessed marks a composer ID as processed with the given message count
func (u *conversationUpdater) MarkAsProcessed(composerID string, messageCount int) error {
	now := time.Now()
	_, err := u.db.Exec(`
		INSERT INTO processed_conversations (composer_id, message_count, last_processed_at)
		VALUES (?, ?, ?)
		ON CONFLICT(composer_id) DO UPDATE SET
			message_count = excluded.message_count,
			last_processed_at = excluded.last_processed_at
	`, composerID, messageCount, now)

	if err != nil {
		return fmt.Errorf("failed to mark composer as processed: %w", err)
	}

	return nil
}

// DetectUpdatedComposers detects which composer IDs have been updated since last processing
func (u *conversationUpdater) DetectUpdatedComposers() ([]string, error) {
	// Get all composer IDs from Cursor database
	composerIDs, err := u.parser.GetComposerIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get composer IDs: %w", err)
	}

	var updatedComposers []string

	for _, composerID := range composerIDs {
		// Get current message count from Cursor database
		currentCount, err := u.getComposerMessageCount(composerID)
		if err != nil {
			u.logger.Warn("failed to get message count for composer", "composer_id", composerID, "error", err)
			continue // Skip this composer, continue with others
		}

		// Get processed message count from our database
		processedCount, err := u.GetProcessedMessageCount(composerID)
		if err != nil {
			// If not found, treat as new conversation (needs processing)
			if currentCount > 0 {
				updatedComposers = append(updatedComposers, composerID)
			}
			continue
		}

		// If current count is greater than processed count, conversation has been updated
		if currentCount > processedCount {
			updatedComposers = append(updatedComposers, composerID)
		}
	}

	return updatedComposers, nil
}

// ProcessUpdate processes an update for a specific composer ID
func (u *conversationUpdater) ProcessUpdate(composerID string) error {
	// Get processed message count
	processedCount, err := u.GetProcessedMessageCount(composerID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get processed message count: %w", err)
	}

	// Parse the full conversation
	conversation, err := u.parser.ParseConversation(composerID)
	if err != nil {
		return fmt.Errorf("failed to parse conversation: %w", err)
	}

	// If no messages, nothing to process
	if len(conversation.Messages) == 0 {
		u.logger.Debug("conversation has no messages", "composer_id", composerID)
		return nil
	}

	// Extract only new messages beyond the processed count
	var newMessages []*Message
	if processedCount >= len(conversation.Messages) {
		// Already processed all messages, nothing new
		u.logger.Debug("conversation already fully processed", "composer_id", composerID, "message_count", len(conversation.Messages))
		return nil
	}

	// Get messages beyond processed count
	newMessages = make([]*Message, 0, len(conversation.Messages)-processedCount)
	for i := processedCount; i < len(conversation.Messages); i++ {
		newMessages = append(newMessages, &conversation.Messages[i])
	}

	// Check if conversation exists in our database
	existingConv, err := u.storage.GetConversationByComposerID(composerID)
	if err != nil {
		// Conversation doesn't exist - this is a new conversation, not an update
		// Return without error - new conversations should be handled by the initial capture flow
		u.logger.Debug("conversation not found in database, treating as new", "composer_id", composerID)
		return nil
	}

	// Update conversation with new messages
	if err := u.storage.UpdateConversation(existingConv.ComposerID, newMessages); err != nil {
		return fmt.Errorf("failed to update conversation: %w", err)
	}

	// Mark as processed with new message count
	newMessageCount := len(conversation.Messages)
	if err := u.MarkAsProcessed(composerID, newMessageCount); err != nil {
		u.logger.Error("failed to mark conversation as processed", "composer_id", composerID, "error", err)
		// Don't return error - conversation was updated successfully
	}

	// Update session metadata
	// Get session ID from conversation
	sessions, err := u.db.Query(`
		SELECT session_id FROM conversations WHERE composer_id = ?
	`, composerID)
	if err != nil {
		u.logger.Warn("failed to get session for conversation", "composer_id", composerID, "error", err)
		return nil // Don't fail if session update fails
	}
	defer sessions.Close()

	if sessions.Next() {
		var sessionID string
		if err := sessions.Scan(&sessionID); err == nil {
			// Update session last activity if we have new messages
			if len(newMessages) > 0 {
				// Get the last message timestamp
				lastMessageTime := newMessages[len(newMessages)-1].CreatedAt
				_, err := u.db.Exec(`
					UPDATE sessions
					SET last_activity = ?,
						updated_at = ?
					WHERE id = ? AND (last_activity IS NULL OR ? > last_activity)
				`, lastMessageTime, time.Now(), sessionID, lastMessageTime)
				if err != nil {
					u.logger.Warn("failed to update session metadata", "session_id", sessionID, "error", err)
				}
			}
		}
	}

	u.logger.Info("processed conversation update", "composer_id", composerID, "new_messages", len(newMessages), "total_messages", newMessageCount)

	return nil
}
