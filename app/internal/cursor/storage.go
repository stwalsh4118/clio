package cursor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
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
	db *sql.DB
}

// NewConversationStorage creates a new conversation storage instance
func NewConversationStorage(db *sql.DB) (ConversationStorage, error) {
	if db == nil {
		return nil, fmt.Errorf("database cannot be nil")
	}

	return &conversationStorage{
		db: db,
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

	// Verify session exists
	var exists bool
	err := cs.db.QueryRow("SELECT EXISTS(SELECT 1 FROM sessions WHERE id = ?)", sessionID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to verify session exists: %w", err)
	}
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Begin transaction
	tx, err := cs.db.Begin()
	if err != nil {
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
		return fmt.Errorf("failed to store conversation: %w", err)
	}

	// Store all messages
	for i := range conversation.Messages {
		if err := cs.storeMessageInTx(tx, &conversation.Messages[i], conversation.ComposerID); err != nil {
			return fmt.Errorf("failed to store message %s: %w", conversation.Messages[i].BubbleID, err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// storeMessageInTx stores a message within an existing transaction
func (cs *conversationStorage) storeMessageInTx(tx *sql.Tx, message *Message, conversationID string) error {
	// Marshal metadata to JSON
	var metadataJSON sql.NullString
	if len(message.Metadata) > 0 {
		metadataBytes, err := json.Marshal(message.Metadata)
		if err != nil {
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
		return fmt.Errorf("failed to insert message: %w", err)
	}

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

	// Verify conversation exists
	var exists bool
	err := cs.db.QueryRow("SELECT EXISTS(SELECT 1 FROM conversations WHERE id = ?)", conversationID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to verify conversation exists: %w", err)
	}
	if !exists {
		return fmt.Errorf("conversation not found: %s", conversationID)
	}

	// Begin transaction
	tx, err := cs.db.Begin()
	if err != nil {
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
		return nil // Nothing to update
	}

	// Verify conversation exists
	var exists bool
	err := cs.db.QueryRow("SELECT EXISTS(SELECT 1 FROM conversations WHERE id = ?)", conversationID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to verify conversation exists: %w", err)
	}
	if !exists {
		return fmt.Errorf("conversation not found: %s", conversationID)
	}

	// Begin transaction
	tx, err := cs.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Store all new messages
	for _, message := range newMessages {
		if err := cs.storeMessageInTx(tx, message, conversationID); err != nil {
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
		return fmt.Errorf("failed to update conversation: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

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
			return nil, fmt.Errorf("conversation not found: %s", composerID)
		}
		return nil, fmt.Errorf("failed to query conversation: %w", err)
	}

	// Query messages
	messages, err := cs.getMessagesByConversationID(conv.ComposerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	conv.Messages = messages
	return &conv, nil
}

// GetConversationsBySession retrieves all conversations for a session
func (cs *conversationStorage) GetConversationsBySession(sessionID string) ([]*Conversation, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}

	// Query conversations
	rows, err := cs.db.Query(`
		SELECT id, composer_id, name, status, message_count, first_message_time, last_message_time, created_at
		FROM conversations
		WHERE session_id = ?
		ORDER BY created_at ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query conversations: %w", err)
	}
	defer rows.Close()

	var conversations []*Conversation
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
			continue // Skip invalid rows
		}

		// Query messages for this conversation
		messages, err := cs.getMessagesByConversationID(conv.ComposerID)
		if err != nil {
			continue // Skip conversations with message errors
		}

		conv.Messages = messages
		conversations = append(conversations, &conv)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating conversations: %w", err)
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
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
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
			continue // Skip invalid rows
		}

		// Parse metadata JSON
		if metadataJSON.Valid && metadataJSON.String != "" {
			msg.Metadata = make(map[string]interface{})
			if err := json.Unmarshal([]byte(metadataJSON.String), &msg.Metadata); err != nil {
				// If metadata is invalid, use empty map
				msg.Metadata = make(map[string]interface{})
			}
		} else {
			msg.Metadata = make(map[string]interface{})
		}

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	return messages, nil
}

