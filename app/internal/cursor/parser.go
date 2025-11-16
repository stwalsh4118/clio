package cursor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/stwalsh4118/clio/internal/config"
	"github.com/stwalsh4118/clio/internal/logging"
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
	logger logging.Logger
}

// NewParser creates a new parser instance
func NewParser(cfg *config.Config) (ParserService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Create logger (use component-specific logger)
	logger, err := logging.NewLogger(cfg)
	if err != nil {
		// If logger creation fails, use no-op logger (don't fail parser creation)
		logger = logging.NewNoopLogger()
	}
	logger = logger.With("component", "parser")

	// Construct database path
	dbPath := filepath.Join(cfg.Cursor.LogPath, "globalStorage", "state.vscdb")

	return &parser{
		config: cfg,
		dbPath: dbPath,
		logger: logger,
	}, nil
}

// openDatabase opens the SQLite database in read-only mode
func (p *parser) openDatabase() error {
	if p.db != nil {
		return nil // Already open
	}

	p.logger.Debug("opening Cursor database", "db_path", p.dbPath)

	// Use shared helper function to open Cursor database
	db, err := OpenCursorDatabase(p.config)
	if err != nil {
		p.logger.Error("failed to open Cursor database", "error", err, "db_path", p.dbPath)
		return fmt.Errorf("failed to open Cursor database: %w", err)
	}

	p.db = db
	p.logger.Info("opened Cursor database", "db_path", p.dbPath)
	return nil
}

// Close closes the database connection
func (p *parser) Close() error {
	if p.db == nil {
		return nil
	}
	p.logger.Debug("closing Cursor database connection")
	err := p.db.Close()
	p.db = nil
	if err != nil {
		p.logger.Error("failed to close database connection", "error", err)
		return err
	}
	p.logger.Info("closed Cursor database connection")
	return nil
}

// retryQueryWithBackoff retries a database query function with exponential backoff on SQLITE_BUSY errors
func (p *parser) retryQueryWithBackoff(maxRetries int, fn func() error) error {
	var lastErr error
	baseDelay := 50 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Only retry on SQLITE_BUSY errors
		if !IsSQLiteBusyError(err) {
			return err
		}

		// Log diagnostics on first retry attempt
		if attempt == 0 {
			LogSQLiteBusyDiagnostics(err, "parser", "query")
		}

		// Calculate exponential backoff delay
		delay := baseDelay * time.Duration(1<<uint(attempt))
		if delay > 2*time.Second {
			delay = 2 * time.Second
		}

		p.logger.Debug("database busy, retrying query", "attempt", attempt+1, "max_retries", maxRetries, "delay_ms", delay.Milliseconds())
		time.Sleep(delay)
	}

	return fmt.Errorf("query failed after %d retries: %w", maxRetries, lastErr)
}

// GetComposerIDs retrieves all composer IDs from the database
func (p *parser) GetComposerIDs() ([]string, error) {
	if err := p.openDatabase(); err != nil {
		return nil, err
	}

	p.logger.Debug("querying composer IDs")

	// Query all composerData keys with retry logic
	query := "SELECT key FROM cursorDiskKV WHERE key LIKE 'composerData:%'"
	var rows *sql.Rows
	err := p.retryQueryWithBackoff(5, func() error {
		var queryErr error
		rows, queryErr = p.db.Query(query)
		return queryErr
	})
	if err != nil {
		p.logger.Error("failed to query composer IDs", "error", err)
		return nil, fmt.Errorf("failed to query composer IDs: %w", err)
	}
	defer rows.Close()

	var composerIDs []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			p.logger.Warn("failed to scan composer ID row", "error", err)
			continue // Skip invalid rows
		}
		// Extract composer ID from key format: "composerData:{composerId}"
		if len(key) > 13 { // "composerData:" is 13 characters
			composerID := key[13:] // Extract everything after "composerData:"
			composerIDs = append(composerIDs, composerID)
		}
	}

	if err := rows.Err(); err != nil {
		p.logger.Error("error iterating composer IDs", "error", err)
		return nil, fmt.Errorf("error iterating composer IDs: %w", err)
	}

	p.logger.Debug("retrieved composer IDs", "count", len(composerIDs))
	return composerIDs, nil
}

// ParseConversation parses a single conversation by composer ID
func (p *parser) ParseConversation(composerID string) (*Conversation, error) {
	if err := p.openDatabase(); err != nil {
		return nil, err
	}

	p.logger.Debug("parsing conversation", "composer_id", composerID)

	// Get composer data
	composerData, err := p.queryComposerData(composerID)
	if err != nil {
		p.logger.Error("failed to query composer data", "composer_id", composerID, "error", err)
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
		p.logger.Warn("failed to query message bubbles, returning partial conversation", "composer_id", composerID, "error", err)
		return conversation, fmt.Errorf("failed to query message bubbles: %w", err)
	}

	conversation.Messages = messages
	p.logger.Info("parsed conversation", "composer_id", composerID, "name", conversation.Name, "message_count", len(messages), "status", conversation.Status)
	return conversation, nil
}

// ParseAllConversations parses all conversations in the database
func (p *parser) ParseAllConversations() ([]*Conversation, error) {
	p.logger.Debug("parsing all conversations")

	composerIDs, err := p.GetComposerIDs()
	if err != nil {
		return nil, err
	}

	var conversations []*Conversation
	var errorCount int
	for _, composerID := range composerIDs {
		conv, err := p.ParseConversation(composerID)
		if err != nil {
			// Log error but continue with other conversations
			// This allows us to parse as many conversations as possible
			p.logger.Warn("failed to parse conversation, skipping", "composer_id", composerID, "error", err)
			errorCount++
			continue
		}
		conversations = append(conversations, conv)
	}

	p.logger.Info("parsed all conversations", "total_composers", len(composerIDs), "successful", len(conversations), "failed", errorCount)
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

	p.logger.Debug("querying composer data", "composer_id", composerID)

	var valueBlob []byte
	err := p.retryQueryWithBackoff(5, func() error {
		return p.db.QueryRow(query, key).Scan(&valueBlob)
	})
	if err != nil {
		if err == sql.ErrNoRows {
			p.logger.Warn("composer data not found", "composer_id", composerID)
			return nil, fmt.Errorf("composer data not found for ID: %s", composerID)
		}
		p.logger.Error("failed to query composer data", "composer_id", composerID, "error", err)
		return nil, fmt.Errorf("failed to query composer data: %w", err)
	}

	// Parse JSON
	var composerData composerDataJSON
	if err := json.Unmarshal(valueBlob, &composerData); err != nil {
		p.logger.Error("failed to parse composer data JSON", "composer_id", composerID, "error", err)
		return nil, fmt.Errorf("failed to parse composer data JSON: %w", err)
	}

	// Ensure ComposerID is set (may not be in JSON)
	if composerData.ComposerID == "" {
		composerData.ComposerID = composerID
	}

	p.logger.Debug("queried composer data", "composer_id", composerID, "name", composerData.Name, "message_count", len(composerData.FullConversationHeadersOnly))
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
	var missingCount, corruptedCount, invalidTimestampCount int

	p.logger.Debug("querying message bubbles", "composer_id", composerID, "header_count", len(headers))

	for _, header := range headers {
		// Query bubble data
		key := fmt.Sprintf("bubbleId:%s:%s", composerID, header.BubbleID)
		query := "SELECT value FROM cursorDiskKV WHERE key = ?"

		var valueBlob []byte
		err := p.retryQueryWithBackoff(5, func() error {
			return p.db.QueryRow(query, key).Scan(&valueBlob)
		})
		if err != nil {
			if err == sql.ErrNoRows {
				// Missing bubble - log warning but continue
				p.logger.Warn("missing message bubble", "composer_id", composerID, "bubble_id", header.BubbleID)
				missingCount++
				continue
			}
			p.logger.Error("failed to query bubble data", "composer_id", composerID, "bubble_id", header.BubbleID, "error", err)
			return nil, fmt.Errorf("failed to query bubble data: %w", err)
		}

		// Parse JSON into a map first to capture all fields
		var rawBubbleData map[string]interface{}
		if err := json.Unmarshal(valueBlob, &rawBubbleData); err != nil {
			// Corrupted JSON - skip this message but continue
			p.logger.Warn("corrupted JSON in message bubble, skipping", "composer_id", composerID, "bubble_id", header.BubbleID, "error", err)
			corruptedCount++
			continue
		}

		// Extract known fields
		bubbleID, _ := rawBubbleData["bubbleId"].(string)
		if bubbleID == "" {
			bubbleID = header.BubbleID
		}

		msgType := 0
		if typeVal, ok := rawBubbleData["type"].(float64); ok {
			msgType = int(typeVal)
		} else if header.Type != 0 {
			msgType = header.Type
		}

		text, _ := rawBubbleData["text"].(string)
		createdAtStr, _ := rawBubbleData["createdAt"].(string)

		// Parse timestamp (ISO 8601 format)
		createdAt, err := parseISO8601Timestamp(createdAtStr)
		if err != nil {
			// Invalid timestamp - use zero time but continue
			p.logger.Warn("invalid timestamp in message bubble, using zero time", "composer_id", composerID, "bubble_id", bubbleID, "timestamp", createdAtStr, "error", err)
			createdAt = time.Time{}
			invalidTimestampCount++
		}

		// Identify role from type
		role := identifyRole(msgType)

		// Extract thinking text (for agent messages)
		thinkingText := ""
		if thinkingVal, ok := rawBubbleData["thinking"].(map[string]interface{}); ok {
			if thinkingTextVal, ok := thinkingVal["text"].(string); ok {
				thinkingText = thinkingTextVal
			}
		}

		// Extract code blocks (from codeBlocks or suggestedCodeBlocks)
		codeBlocks := extractCodeBlocks(rawBubbleData)

		// Extract tool calls (from toolFormerData)
		toolCalls := extractToolCalls(rawBubbleData)

		// Determine content source
		contentSource := determineContentSource(text, thinkingText, codeBlocks, toolCalls)

		// Build metadata map with all fields except the ones we're storing directly
		metadata := make(map[string]interface{})
		for key, value := range rawBubbleData {
			// Skip fields we're storing directly in the Message struct
			if key != "bubbleId" && key != "type" && key != "text" && key != "createdAt" &&
				key != "thinking" && key != "codeBlocks" && key != "suggestedCodeBlocks" &&
				key != "toolFormerData" && key != "toolResults" {
				metadata[key] = value
			}
		}

		// Build message
		message := Message{
			BubbleID:      bubbleID,
			Type:          msgType,
			Role:          role,
			Text:          text,
			ThinkingText:  thinkingText,
			CodeBlocks:    codeBlocks,
			ToolCalls:     toolCalls,
			ContentSource: contentSource,
			HasCode:       len(codeBlocks) > 0,
			HasThinking:   thinkingText != "",
			HasToolCalls:  len(toolCalls) > 0,
			CreatedAt:     createdAt,
			Metadata:      metadata,
		}

		messages = append(messages, message)
		p.logger.Debug("parsed message bubble", "composer_id", composerID, "bubble_id", header.BubbleID, "role", role)
	}

	if missingCount > 0 || corruptedCount > 0 || invalidTimestampCount > 0 {
		p.logger.Warn("message bubble parsing completed with issues", "composer_id", composerID, "total_headers", len(headers), "successful", len(messages), "missing", missingCount, "corrupted", corruptedCount, "invalid_timestamps", invalidTimestampCount)
	} else {
		p.logger.Debug("message bubble parsing completed", "composer_id", composerID, "message_count", len(messages))
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

// extractCodeBlocks extracts code blocks from raw bubble data
// Checks both codeBlocks and suggestedCodeBlocks fields
func extractCodeBlocks(data map[string]interface{}) []CodeBlock {
	var codeBlocks []CodeBlock

	// Try codeBlocks first
	if codeBlocksVal, ok := data["codeBlocks"].([]interface{}); ok {
		for _, cb := range codeBlocksVal {
			if cbMap, ok := cb.(map[string]interface{}); ok {
				codeBlock := CodeBlock{}
				if content, ok := cbMap["content"].(string); ok {
					codeBlock.Content = content
				}
				if langID, ok := cbMap["languageId"].(string); ok {
					codeBlock.LanguageID = langID
				}
				if idx, ok := cbMap["codeBlockIdx"].(float64); ok {
					codeBlock.CodeBlockIdx = int(idx)
				}
				if codeBlock.Content != "" {
					codeBlocks = append(codeBlocks, codeBlock)
				}
			}
		}
	}

	// Also check suggestedCodeBlocks
	if suggestedVal, ok := data["suggestedCodeBlocks"].([]interface{}); ok {
		for _, cb := range suggestedVal {
			if cbMap, ok := cb.(map[string]interface{}); ok {
				codeBlock := CodeBlock{}
				if content, ok := cbMap["content"].(string); ok {
					codeBlock.Content = content
				}
				if langID, ok := cbMap["languageId"].(string); ok {
					codeBlock.LanguageID = langID
				}
				if idx, ok := cbMap["codeBlockIdx"].(float64); ok {
					codeBlock.CodeBlockIdx = int(idx)
				}
				if codeBlock.Content != "" {
					codeBlocks = append(codeBlocks, codeBlock)
				}
			}
		}
	}

	return codeBlocks
}

// extractToolCalls extracts tool calls from raw bubble data
// Checks toolFormerData field
func extractToolCalls(data map[string]interface{}) []ToolCall {
	var toolCalls []ToolCall

	if toolDataVal, ok := data["toolFormerData"].(map[string]interface{}); ok {
		toolCall := ToolCall{}
		if name, ok := toolDataVal["name"].(string); ok {
			toolCall.Name = name
		}
		if status, ok := toolDataVal["status"].(string); ok {
			toolCall.Status = status
		}
		if idx, ok := toolDataVal["toolIndex"].(float64); ok {
			toolCall.ToolIndex = int(idx)
		}
		if toolCall.Name != "" {
			toolCalls = append(toolCalls, toolCall)
		}
	}

	// Also check toolResults array (multiple tool calls)
	if toolResultsVal, ok := data["toolResults"].([]interface{}); ok {
		for _, tr := range toolResultsVal {
			if trMap, ok := tr.(map[string]interface{}); ok {
				toolCall := ToolCall{}
				if name, ok := trMap["name"].(string); ok {
					toolCall.Name = name
				} else if name, ok := trMap["toolName"].(string); ok {
					toolCall.Name = name
				}
				if status, ok := trMap["status"].(string); ok {
					toolCall.Status = status
				}
				if idx, ok := trMap["toolIndex"].(float64); ok {
					toolCall.ToolIndex = int(idx)
				}
				if toolCall.Name != "" {
					toolCalls = append(toolCalls, toolCall)
				}
			}
		}
	}

	return toolCalls
}

// determineContentSource determines where the message content came from
// Returns: "text" | "thinking" | "code" | "tool" | "mixed"
func determineContentSource(text, thinkingText string, codeBlocks []CodeBlock, toolCalls []ToolCall) string {
	hasText := text != ""
	hasThinking := thinkingText != ""
	hasCode := len(codeBlocks) > 0
	hasTools := len(toolCalls) > 0

	count := 0
	if hasText {
		count++
	}
	if hasThinking {
		count++
	}
	if hasCode {
		count++
	}
	if hasTools {
		count++
	}

	if count > 1 {
		return "mixed"
	}
	if hasText {
		return "text"
	}
	if hasThinking {
		return "thinking"
	}
	if hasCode {
		return "code"
	}
	if hasTools {
		return "tool"
	}
	return "text" // Default fallback
}
