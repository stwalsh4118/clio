package cursor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/stwalsh4118/clio/internal/logging"
)

// ConversationStorage defines the interface for storing and retrieving conversations and messages
type ConversationStorage interface {
	StoreConversation(conversation *Conversation, sessionID string) error
	StoreMessage(message *Message, conversationID string) error
	UpdateConversation(conversationID string, newMessages []*Message) error
	GetConversation(conversationID string) (*Conversation, error)
	GetConversationByComposerID(composerID string) (*Conversation, error)
	GetConversationsBySession(sessionID string) ([]*Conversation, error)
}

// conversationStorage implements ConversationStorage for database persistence
type conversationStorage struct {
	db     *sql.DB
	logger logging.Logger
}

// NewConversationStorage creates a new conversation storage instance
func NewConversationStorage(db *sql.DB, logger logging.Logger) (ConversationStorage, error) {
	if db == nil {
		return nil, fmt.Errorf("database cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	// Use component-specific logger
	logger = logger.With("component", "conversation_storage")

	return &conversationStorage{
		db:     db,
		logger: logger,
	}, nil
}

// StoreConversation stores a conversation and all its messages in a single transaction
func (cs *conversationStorage) StoreConversation(conversation *Conversation, sessionID string) error {
	if conversation == nil {
		return fmt.Errorf("conversation cannot be nil")
	}
	if sessionID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}

	cs.logger.Debug("storing conversation", "composer_id", conversation.ComposerID, "session_id", sessionID, "message_count", len(conversation.Messages))

	// Verify session exists
	var exists bool
	err := cs.db.QueryRow("SELECT EXISTS(SELECT 1 FROM sessions WHERE id = ?)", sessionID).Scan(&exists)
	if err != nil {
		cs.logger.Error("failed to verify session exists", "session_id", sessionID, "error", err)
		return fmt.Errorf("failed to verify session exists: %w", err)
	}
	if !exists {
		cs.logger.Error("session not found", "session_id", sessionID, "composer_id", conversation.ComposerID)
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Begin transaction
	cs.logger.Debug("starting transaction for conversation storage", "composer_id", conversation.ComposerID)
	tx, err := cs.db.Begin()
	if err != nil {
		cs.logger.Error("failed to begin transaction", "composer_id", conversation.ComposerID, "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Calculate message count and timestamps
	messageCount := len(conversation.Messages)
	var firstMessageTime, lastMessageTime *time.Time
	if messageCount > 0 {
		firstMsgTime := conversation.Messages[0].CreatedAt
		lastMsgTime := conversation.Messages[0].CreatedAt
		for _, msg := range conversation.Messages {
			if msg.CreatedAt.Before(firstMsgTime) {
				firstMsgTime = msg.CreatedAt
			}
			if msg.CreatedAt.After(lastMsgTime) {
				lastMsgTime = msg.CreatedAt
			}
		}
		firstMessageTime = &firstMsgTime
		lastMessageTime = &lastMsgTime
	}

	now := time.Now()

	// Store conversation (use composer_id as the conversation ID)
	_, err = tx.Exec(`
		INSERT INTO conversations (id, session_id, composer_id, name, status, message_count, first_message_time, last_message_time, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			session_id = excluded.session_id,
			name = excluded.name,
			status = excluded.status,
			message_count = excluded.message_count,
			first_message_time = excluded.first_message_time,
			last_message_time = excluded.last_message_time,
			updated_at = excluded.updated_at
	`,
		conversation.ComposerID, // id = composer_id
		sessionID,
		conversation.ComposerID,
		conversation.Name,
		conversation.Status,
		messageCount,
		firstMessageTime,
		lastMessageTime,
		conversation.CreatedAt,
		now,
	)
	if err != nil {
		cs.logger.Error("failed to store conversation", "composer_id", conversation.ComposerID, "session_id", sessionID, "error", err)
		return fmt.Errorf("failed to store conversation: %w", err)
	}

	// Store all messages
	for i := range conversation.Messages {
		if err := cs.storeMessageInTx(tx, &conversation.Messages[i], conversation.ComposerID); err != nil {
			cs.logger.Error("failed to store message", "composer_id", conversation.ComposerID, "bubble_id", conversation.Messages[i].BubbleID, "error", err)
			return fmt.Errorf("failed to store message %s: %w", conversation.Messages[i].BubbleID, err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		cs.logger.Error("failed to commit transaction", "composer_id", conversation.ComposerID, "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	cs.logger.Info("stored conversation", "composer_id", conversation.ComposerID, "session_id", sessionID, "message_count", messageCount)
	return nil
}

// storeMessageInTx stores a message within an existing transaction
func (cs *conversationStorage) storeMessageInTx(tx *sql.Tx, message *Message, conversationID string) error {
	// Marshal metadata to JSON
	var metadataJSON sql.NullString
	if len(message.Metadata) > 0 {
		metadataBytes, err := json.Marshal(message.Metadata)
		if err != nil {
			cs.logger.Warn("failed to marshal message metadata", "conversation_id", conversationID, "bubble_id", message.BubbleID, "error", err)
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		metadataJSON = sql.NullString{String: string(metadataBytes), Valid: true}
	}

	_, err := tx.Exec(`
		INSERT INTO messages (id, conversation_id, bubble_id, type, role, content, created_at, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			conversation_id = excluded.conversation_id,
			bubble_id = excluded.bubble_id,
			type = excluded.type,
			role = excluded.role,
			content = excluded.content,
			created_at = excluded.created_at,
			metadata = excluded.metadata
	`,
		message.BubbleID, // id = bubble_id
		conversationID,
		message.BubbleID,
		message.Type,
		message.Role,
		message.Text,
		message.CreatedAt,
		metadataJSON,
	)
	if err != nil {
		cs.logger.Error("failed to insert message", "conversation_id", conversationID, "bubble_id", message.BubbleID, "error", err)
		return fmt.Errorf("failed to insert message: %w", err)
	}

	cs.logger.Debug("stored message", "conversation_id", conversationID, "bubble_id", message.BubbleID, "role", message.Role)
	return nil
}

// StoreMessage stores a single message for an existing conversation
func (cs *conversationStorage) StoreMessage(message *Message, conversationID string) error {
	if message == nil {
		return fmt.Errorf("message cannot be nil")
	}
	if conversationID == "" {
		return fmt.Errorf("conversation ID cannot be empty")
	}

	cs.logger.Debug("storing single message", "conversation_id", conversationID, "bubble_id", message.BubbleID)

	// Verify conversation exists
	var exists bool
	err := cs.db.QueryRow("SELECT EXISTS(SELECT 1 FROM conversations WHERE id = ?)", conversationID).Scan(&exists)
	if err != nil {
		cs.logger.Error("failed to verify conversation exists", "conversation_id", conversationID, "error", err)
		return fmt.Errorf("failed to verify conversation exists: %w", err)
	}
	if !exists {
		cs.logger.Error("conversation not found", "conversation_id", conversationID, "bubble_id", message.BubbleID)
		return fmt.Errorf("conversation not found: %s", conversationID)
	}

	// Begin transaction
	tx, err := cs.db.Begin()
	if err != nil {
		cs.logger.Error("failed to begin transaction", "conversation_id", conversationID, "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Store message
	if err := cs.storeMessageInTx(tx, message, conversationID); err != nil {
		return err
	}

	// Update conversation message count and timestamps
	// Use CASE statements to update first_message_time and last_message_time
	_, err = tx.Exec(`
		UPDATE conversations
		SET message_count = message_count + 1,
			first_message_time = CASE
				WHEN first_message_time IS NULL THEN ?
				WHEN ? < first_message_time THEN ?
				ELSE first_message_time
			END,
			last_message_time = CASE
				WHEN last_message_time IS NULL THEN ?
				WHEN ? > last_message_time THEN ?
				ELSE last_message_time
			END,
			updated_at = ?
		WHERE id = ?
	`, message.CreatedAt, message.CreatedAt, message.CreatedAt, message.CreatedAt, message.CreatedAt, message.CreatedAt, time.Now(), conversationID)
	if err != nil {
		return fmt.Errorf("failed to update conversation: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// UpdateConversation adds new messages to an existing conversation
func (cs *conversationStorage) UpdateConversation(conversationID string, newMessages []*Message) error {
	if conversationID == "" {
		return fmt.Errorf("conversation ID cannot be empty")
	}
	if len(newMessages) == 0 {
		cs.logger.Debug("no new messages to update", "conversation_id", conversationID)
		return nil // Nothing to update
	}

	cs.logger.Debug("updating conversation with new messages", "conversation_id", conversationID, "new_message_count", len(newMessages))

	// Verify conversation exists
	var exists bool
	err := cs.db.QueryRow("SELECT EXISTS(SELECT 1 FROM conversations WHERE id = ?)", conversationID).Scan(&exists)
	if err != nil {
		cs.logger.Error("failed to verify conversation exists", "conversation_id", conversationID, "error", err)
		return fmt.Errorf("failed to verify conversation exists: %w", err)
	}
	if !exists {
		cs.logger.Error("conversation not found", "conversation_id", conversationID)
		return fmt.Errorf("conversation not found: %s", conversationID)
	}

	// Begin transaction
	tx, err := cs.db.Begin()
	if err != nil {
		cs.logger.Error("failed to begin transaction", "conversation_id", conversationID, "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Store all new messages
	for _, message := range newMessages {
		if err := cs.storeMessageInTx(tx, message, conversationID); err != nil {
			cs.logger.Error("failed to store message in update", "conversation_id", conversationID, "bubble_id", message.BubbleID, "error", err)
			return fmt.Errorf("failed to store message %s: %w", message.BubbleID, err)
		}
	}

	// Update conversation message count and timestamps
	// Calculate new first and last message times
	var firstMsgTime, lastMsgTime *time.Time
	for _, msg := range newMessages {
		if firstMsgTime == nil || msg.CreatedAt.Before(*firstMsgTime) {
			t := msg.CreatedAt
			firstMsgTime = &t
		}
		if lastMsgTime == nil || msg.CreatedAt.After(*lastMsgTime) {
			t := msg.CreatedAt
			lastMsgTime = &t
		}
	}

	// Update conversation
	updateQuery := `
		UPDATE conversations
		SET message_count = message_count + ?,
			updated_at = ?
	`
	args := []interface{}{len(newMessages), time.Now()}

	if firstMsgTime != nil {
		updateQuery += `,
			first_message_time = CASE
				WHEN first_message_time IS NULL THEN ?
				WHEN ? < first_message_time THEN ?
				ELSE first_message_time
			END`
		args = append(args, *firstMsgTime, *firstMsgTime, *firstMsgTime)
	}

	if lastMsgTime != nil {
		updateQuery += `,
			last_message_time = CASE
				WHEN last_message_time IS NULL THEN ?
				WHEN ? > last_message_time THEN ?
				ELSE last_message_time
			END`
		args = append(args, *lastMsgTime, *lastMsgTime, *lastMsgTime)
	}

	updateQuery += ` WHERE id = ?`
	args = append(args, conversationID)

	_, err = tx.Exec(updateQuery, args...)
	if err != nil {
		cs.logger.Error("failed to update conversation metadata", "conversation_id", conversationID, "error", err)
		return fmt.Errorf("failed to update conversation: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		cs.logger.Error("failed to commit transaction", "conversation_id", conversationID, "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	cs.logger.Info("updated conversation", "conversation_id", conversationID, "new_message_count", len(newMessages))
	return nil
}

// GetConversation retrieves a conversation by its ID (composer_id)
func (cs *conversationStorage) GetConversation(conversationID string) (*Conversation, error) {
	return cs.GetConversationByComposerID(conversationID)
}

// GetConversationByComposerID retrieves a conversation by composer ID
func (cs *conversationStorage) GetConversationByComposerID(composerID string) (*Conversation, error) {
	if composerID == "" {
		return nil, fmt.Errorf("composer ID cannot be empty")
	}

	cs.logger.Debug("retrieving conversation by composer ID", "composer_id", composerID)

	// Query conversation
	var conv Conversation
	var firstMsgTime, lastMsgTime sql.NullTime
	var messageCount int // We'll use actual message count from messages table
	err := cs.db.QueryRow(`
		SELECT id, composer_id, name, status, message_count, first_message_time, last_message_time, created_at
		FROM conversations
		WHERE composer_id = ?
	`, composerID).Scan(
		&conv.ComposerID,
		&conv.ComposerID,
		&conv.Name,
		&conv.Status,
		&messageCount,
		&firstMsgTime,
		&lastMsgTime,
		&conv.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			cs.logger.Debug("conversation not found", "composer_id", composerID)
			return nil, fmt.Errorf("conversation not found: %s", composerID)
		}
		cs.logger.Error("failed to query conversation", "composer_id", composerID, "error", err)
		return nil, fmt.Errorf("failed to query conversation: %w", err)
	}

	// Query messages
	messages, err := cs.getMessagesByConversationID(conv.ComposerID)
	if err != nil {
		cs.logger.Error("failed to get messages", "composer_id", composerID, "error", err)
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	conv.Messages = messages
	cs.logger.Info("retrieved conversation", "composer_id", composerID, "message_count", len(messages))
	return &conv, nil
}

// GetConversationsBySession retrieves all conversations for a session
func (cs *conversationStorage) GetConversationsBySession(sessionID string) ([]*Conversation, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}

	cs.logger.Debug("retrieving conversations by session", "session_id", sessionID)

	// Query conversations
	rows, err := cs.db.Query(`
		SELECT id, composer_id, name, status, message_count, first_message_time, last_message_time, created_at
		FROM conversations
		WHERE session_id = ?
		ORDER BY created_at ASC
	`, sessionID)
	if err != nil {
		cs.logger.Error("failed to query conversations", "session_id", sessionID, "error", err)
		return nil, fmt.Errorf("failed to query conversations: %w", err)
	}
	defer rows.Close()

	var conversations []*Conversation
	var skippedCount int
	for rows.Next() {
		var conv Conversation
		var firstMsgTime, lastMsgTime sql.NullTime
		var messageCount int // We'll use actual message count from messages table
		err := rows.Scan(
			&conv.ComposerID,
			&conv.ComposerID,
			&conv.Name,
			&conv.Status,
			&messageCount,
			&firstMsgTime,
			&lastMsgTime,
			&conv.CreatedAt,
		)
		if err != nil {
			cs.logger.Warn("failed to scan conversation row, skipping", "session_id", sessionID, "error", err)
			skippedCount++
			continue // Skip invalid rows
		}

		// Query messages for this conversation
		messages, err := cs.getMessagesByConversationID(conv.ComposerID)
		if err != nil {
			cs.logger.Warn("failed to get messages for conversation, skipping", "session_id", sessionID, "composer_id", conv.ComposerID, "error", err)
			skippedCount++
			continue // Skip conversations with message errors
		}

		conv.Messages = messages
		conversations = append(conversations, &conv)
	}

	if err := rows.Err(); err != nil {
		cs.logger.Error("error iterating conversations", "session_id", sessionID, "error", err)
		return nil, fmt.Errorf("error iterating conversations: %w", err)
	}

	if skippedCount > 0 {
		cs.logger.Warn("retrieved conversations with skipped entries", "session_id", sessionID, "successful", len(conversations), "skipped", skippedCount)
	} else {
		cs.logger.Info("retrieved conversations", "session_id", sessionID, "count", len(conversations))
	}
	return conversations, nil
}

// getMessagesByConversationID retrieves all messages for a conversation, ordered by created_at
func (cs *conversationStorage) getMessagesByConversationID(conversationID string) ([]Message, error) {
	rows, err := cs.db.Query(`
		SELECT id, bubble_id, type, role, content, created_at, metadata
		FROM messages
		WHERE conversation_id = ?
		ORDER BY created_at ASC
	`, conversationID)
	if err != nil {
		cs.logger.Error("failed to query messages", "conversation_id", conversationID, "error", err)
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	var skippedCount int
	for rows.Next() {
		var msg Message
		var metadataJSON sql.NullString

		err := rows.Scan(
			&msg.BubbleID,
			&msg.BubbleID,
			&msg.Type,
			&msg.Role,
			&msg.Text,
			&msg.CreatedAt,
			&metadataJSON,
		)
		if err != nil {
			cs.logger.Warn("failed to scan message row, skipping", "conversation_id", conversationID, "error", err)
			skippedCount++
			continue // Skip invalid rows
		}

		// Parse metadata JSON
		if metadataJSON.Valid && metadataJSON.String != "" {
			msg.Metadata = make(map[string]interface{})
			if err := json.Unmarshal([]byte(metadataJSON.String), &msg.Metadata); err != nil {
				// If metadata is invalid, use empty map
				cs.logger.Warn("failed to parse message metadata JSON, using empty map", "conversation_id", conversationID, "bubble_id", msg.BubbleID, "error", err)
				msg.Metadata = make(map[string]interface{})
			}
		} else {
			msg.Metadata = make(map[string]interface{})
		}

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		cs.logger.Error("error iterating messages", "conversation_id", conversationID, "error", err)
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	if skippedCount > 0 {
		cs.logger.Warn("retrieved messages with skipped entries", "conversation_id", conversationID, "successful", len(messages), "skipped", skippedCount)
	}

	return messages, nil
}
