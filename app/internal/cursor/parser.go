package cursor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/stwalsh4118/clio/internal/config"
	_ "modernc.org/sqlite" // SQLite driver
)

// ParserService defines the interface for parsing Cursor conversation data
type ParserService interface {
	ParseConversation(composerID string) (*Conversation, error)
	ParseAllConversations() ([]*Conversation, error)
	GetComposerIDs() ([]string, error)
	Close() error
}

// parser implements ParserService for extracting conversation data from Cursor's SQLite database
type parser struct {
	config *config.Config
	db     *sql.DB
	dbPath string
}

// NewParser creates a new parser instance
func NewParser(cfg *config.Config) (ParserService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Construct database path
	dbPath := filepath.Join(cfg.Cursor.LogPath, "globalStorage", "state.vscdb")

	return &parser{
		config: cfg,
		dbPath: dbPath,
	}, nil
}

// openDatabase opens the SQLite database in read-only mode
func (p *parser) openDatabase() error {
	if p.db != nil {
		return nil // Already open
	}

	// Use shared helper function to open Cursor database
	db, err := OpenCursorDatabase(p.config)
	if err != nil {
		return err
	}

	p.db = db
	return nil
}

// Close closes the database connection
func (p *parser) Close() error {
	if p.db == nil {
		return nil
	}
	err := p.db.Close()
	p.db = nil
	return err
}

// GetComposerIDs retrieves all composer IDs from the database
func (p *parser) GetComposerIDs() ([]string, error) {
	if err := p.openDatabase(); err != nil {
		return nil, err
	}

	// Query all composerData keys
	query := "SELECT key FROM cursorDiskKV WHERE key LIKE 'composerData:%'"
	rows, err := p.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query composer IDs: %w", err)
	}
	defer rows.Close()

	var composerIDs []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			continue // Skip invalid rows
		}
		// Extract composer ID from key format: "composerData:{composerId}"
		if len(key) > 13 { // "composerData:" is 13 characters
			composerID := key[13:] // Extract everything after "composerData:"
			composerIDs = append(composerIDs, composerID)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating composer IDs: %w", err)
	}

	return composerIDs, nil
}

// ParseConversation parses a single conversation by composer ID
func (p *parser) ParseConversation(composerID string) (*Conversation, error) {
	if err := p.openDatabase(); err != nil {
		return nil, err
	}

	// Get composer data
	composerData, err := p.queryComposerData(composerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query composer data: %w", err)
	}

	// Parse timestamp (Unix milliseconds)
	createdAt := parseUnixMilliseconds(composerData.CreatedAt)

	// Build conversation struct
	conversation := &Conversation{
		ComposerID: composerID,
		Name:       composerData.Name,
		Status:     composerData.Status,
		CreatedAt:  createdAt,
		Messages:   []Message{},
	}

	// Get all message bubbles
	messages, err := p.queryMessageBubbles(composerID, composerData.FullConversationHeadersOnly)
	if err != nil {
		// Log error but return partial conversation
		// This allows us to get conversation metadata even if some messages fail
		return conversation, fmt.Errorf("failed to query message bubbles: %w", err)
	}

	conversation.Messages = messages
	return conversation, nil
}

// ParseAllConversations parses all conversations in the database
func (p *parser) ParseAllConversations() ([]*Conversation, error) {
	composerIDs, err := p.GetComposerIDs()
	if err != nil {
		return nil, err
	}

	var conversations []*Conversation
	for _, composerID := range composerIDs {
		conv, err := p.ParseConversation(composerID)
		if err != nil {
			// Log error but continue with other conversations
			// This allows us to parse as many conversations as possible
			continue
		}
		conversations = append(conversations, conv)
	}

	return conversations, nil
}

// composerDataJSON represents the JSON structure of composerData entries
type composerDataJSON struct {
	ComposerID                  string `json:"composerId"`
	Name                        string `json:"name"`
	Status                      string `json:"status"`
	CreatedAt                   int64  `json:"createdAt"` // Unix timestamp in milliseconds
	FullConversationHeadersOnly []struct {
		BubbleID string `json:"bubbleId"`
		Type     int    `json:"type"`
	} `json:"fullConversationHeadersOnly"`
}

// queryComposerData queries and parses composer data from the database
func (p *parser) queryComposerData(composerID string) (*composerDataJSON, error) {
	key := fmt.Sprintf("composerData:%s", composerID)
	query := "SELECT value FROM cursorDiskKV WHERE key = ?"

	var valueBlob []byte
	err := p.db.QueryRow(query, key).Scan(&valueBlob)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("composer data not found for ID: %s", composerID)
		}
		return nil, fmt.Errorf("failed to query composer data: %w", err)
	}

	// Parse JSON
	var composerData composerDataJSON
	if err := json.Unmarshal(valueBlob, &composerData); err != nil {
		return nil, fmt.Errorf("failed to parse composer data JSON: %w", err)
	}

	// Ensure ComposerID is set (may not be in JSON)
	if composerData.ComposerID == "" {
		composerData.ComposerID = composerID
	}

	return &composerData, nil
}

// bubbleDataJSON represents the JSON structure of bubbleId entries
type bubbleDataJSON struct {
	BubbleID  string `json:"bubbleId"`
	Type      int    `json:"type"`      // 1 = user, 2 = agent
	Text      string `json:"text"`      // Message content
	CreatedAt string `json:"createdAt"` // ISO 8601 timestamp
}

// queryMessageBubbles queries and parses message bubbles from the database
func (p *parser) queryMessageBubbles(composerID string, headers []struct {
	BubbleID string `json:"bubbleId"`
	Type     int    `json:"type"`
}) ([]Message, error) {
	var messages []Message

	for _, header := range headers {
		// Query bubble data
		key := fmt.Sprintf("bubbleId:%s:%s", composerID, header.BubbleID)
		query := "SELECT value FROM cursorDiskKV WHERE key = ?"

		var valueBlob []byte
		err := p.db.QueryRow(query, key).Scan(&valueBlob)
		if err != nil {
			if err == sql.ErrNoRows {
				// Missing bubble - log warning but continue
				continue
			}
			return nil, fmt.Errorf("failed to query bubble data: %w", err)
		}

		// Parse JSON
		var bubbleData bubbleDataJSON
		if err := json.Unmarshal(valueBlob, &bubbleData); err != nil {
			// Corrupted JSON - skip this message but continue
			continue
		}

		// Parse timestamp (ISO 8601 format)
		createdAt, err := parseISO8601Timestamp(bubbleData.CreatedAt)
		if err != nil {
			// Invalid timestamp - use zero time but continue
			createdAt = time.Time{}
		}

		// Identify role from type
		role := identifyRole(bubbleData.Type)

		// Build message
		message := Message{
			BubbleID:  bubbleData.BubbleID,
			Type:      bubbleData.Type,
			Role:      role,
			Text:      bubbleData.Text,
			CreatedAt: createdAt,
			Metadata:  make(map[string]interface{}),
		}

		messages = append(messages, message)
	}

	return messages, nil
}

// parseUnixMilliseconds parses a Unix timestamp in milliseconds to time.Time
func parseUnixMilliseconds(ms int64) time.Time {
	return time.Unix(0, ms*int64(time.Millisecond))
}

// parseISO8601Timestamp parses an ISO 8601 timestamp string to time.Time
func parseISO8601Timestamp(ts string) (time.Time, error) {
	// Try common ISO 8601 formats
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, ts); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", ts)
}

// identifyRole converts message type to human-readable role
func identifyRole(msgType int) string {
	switch msgType {
	case 1:
		return "user"
	case 2:
		return "agent"
	default:
		return "unknown"
	}
}
