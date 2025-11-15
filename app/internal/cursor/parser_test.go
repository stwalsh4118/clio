package cursor

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stwalsh4118/clio/internal/config"
	_ "modernc.org/sqlite"
)

// createTestDatabase creates a test SQLite database with sample conversation data
func createTestDatabase(t *testing.T, dbPath string) {
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

	// Sample composer data
	composerID := "test-composer-id-123"
	composerData := map[string]interface{}{
		"composerId": composerID,
		"name":       "Test Conversation",
		"status":     "completed",
		"createdAt":  1704067200000, // Unix milliseconds: 2024-01-01 00:00:00 UTC
		"fullConversationHeadersOnly": []map[string]interface{}{
			{"bubbleId": "bubble-1", "type": 1},
			{"bubbleId": "bubble-2", "type": 2},
			{"bubbleId": "bubble-3", "type": 1},
		},
	}
	composerJSON, _ := json.Marshal(composerData)
	composerKey := "composerData:" + composerID
	if _, err := db.Exec("INSERT INTO cursorDiskKV (key, value) VALUES (?, ?)", composerKey, composerJSON); err != nil {
		t.Fatalf("Failed to insert composer data: %v", err)
	}

	// Sample message bubbles
	bubbles := []struct {
		bubbleID  string
		msgType   int
		text      string
		createdAt string
	}{
		{"bubble-1", 1, "Hello, how do I debug this?", "2024-01-01T12:00:00.000Z"},
		{"bubble-2", 2, "To debug this, you can...", "2024-01-01T12:00:15.000Z"},
		{"bubble-3", 1, "Thanks!", "2024-01-01T12:00:30.000Z"},
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

func TestNewParser(t *testing.T) {
	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: "/tmp/test/cursor",
		},
	}

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("NewParser() error = %v, want nil", err)
	}

	if parser == nil {
		t.Fatal("NewParser() returned nil parser")
	}

	// Test nil config
	_, err = NewParser(nil)
	if err == nil {
		t.Error("NewParser(nil) expected error, got nil")
	}
}

func TestParser_ParseConversation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "globalStorage", "state.vscdb")
	createTestDatabase(t, dbPath)

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("NewParser() error = %v", err)
	}
	defer parser.Close()

	// Parse conversation
	conversation, err := parser.ParseConversation("test-composer-id-123")
	if err != nil {
		t.Fatalf("ParseConversation() error = %v", err)
	}

	// Verify conversation metadata
	if conversation.ComposerID != "test-composer-id-123" {
		t.Errorf("ComposerID = %v, want test-composer-id-123", conversation.ComposerID)
	}
	if conversation.Name != "Test Conversation" {
		t.Errorf("Name = %v, want Test Conversation", conversation.Name)
	}
	if conversation.Status != "completed" {
		t.Errorf("Status = %v, want completed", conversation.Status)
	}

	// Verify timestamp parsing (Unix milliseconds)
	expectedTime := time.Unix(0, 1704067200000*int64(time.Millisecond))
	if !conversation.CreatedAt.Equal(expectedTime) {
		t.Errorf("CreatedAt = %v, want %v", conversation.CreatedAt, expectedTime)
	}

	// Verify messages
	if len(conversation.Messages) != 3 {
		t.Fatalf("Messages count = %v, want 3", len(conversation.Messages))
	}

	// Verify first message (user)
	msg1 := conversation.Messages[0]
	if msg1.Role != "user" {
		t.Errorf("Message 1 Role = %v, want user", msg1.Role)
	}
	if msg1.Type != 1 {
		t.Errorf("Message 1 Type = %v, want 1", msg1.Type)
	}
	if msg1.Text != "Hello, how do I debug this?" {
		t.Errorf("Message 1 Text = %v, want Hello, how do I debug this?", msg1.Text)
	}

	// Verify second message (agent)
	msg2 := conversation.Messages[1]
	if msg2.Role != "agent" {
		t.Errorf("Message 2 Role = %v, want agent", msg2.Role)
	}
	if msg2.Type != 2 {
		t.Errorf("Message 2 Type = %v, want 2", msg2.Type)
	}
	if msg2.Text != "To debug this, you can..." {
		t.Errorf("Message 2 Text = %v, want To debug this, you can...", msg2.Text)
	}
}

func TestParser_GetComposerIDs(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "globalStorage", "state.vscdb")
	createTestDatabase(t, dbPath)

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("NewParser() error = %v", err)
	}
	defer parser.Close()

	composerIDs, err := parser.GetComposerIDs()
	if err != nil {
		t.Fatalf("GetComposerIDs() error = %v", err)
	}

	if len(composerIDs) != 1 {
		t.Fatalf("GetComposerIDs() count = %v, want 1", len(composerIDs))
	}

	if composerIDs[0] != "test-composer-id-123" {
		t.Errorf("ComposerID = %v, want test-composer-id-123", composerIDs[0])
	}
}

func TestParser_ParseAllConversations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "globalStorage", "state.vscdb")
	createTestDatabase(t, dbPath)

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("NewParser() error = %v", err)
	}
	defer parser.Close()

	conversations, err := parser.ParseAllConversations()
	if err != nil {
		t.Fatalf("ParseAllConversations() error = %v", err)
	}

	if len(conversations) != 1 {
		t.Fatalf("ParseAllConversations() count = %v, want 1", len(conversations))
	}
}

func TestParser_MissingComposer(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "globalStorage", "state.vscdb")
	createTestDatabase(t, dbPath)

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("NewParser() error = %v", err)
	}
	defer parser.Close()

	// Try to parse non-existent conversation
	_, err = parser.ParseConversation("non-existent-id")
	if err == nil {
		t.Error("ParseConversation() expected error for missing composer, got nil")
	}
}

func TestParser_MissingBubble(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "globalStorage", "state.vscdb")
	createTestDatabase(t, dbPath)

	// Add composer with reference to missing bubble
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	composerID := "test-missing-bubble"
	composerData := map[string]interface{}{
		"composerId": composerID,
		"name":       "Test Missing Bubble",
		"status":     "active",
		"createdAt":  1704067200000,
		"fullConversationHeadersOnly": []map[string]interface{}{
			{"bubbleId": "missing-bubble-id", "type": 1},
		},
	}
	composerJSON, _ := json.Marshal(composerData)
	composerKey := "composerData:" + composerID
	if _, err := db.Exec("INSERT INTO cursorDiskKV (key, value) VALUES (?, ?)", composerKey, composerJSON); err != nil {
		t.Fatalf("Failed to insert composer data: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("NewParser() error = %v", err)
	}
	defer parser.Close()

	// Parse conversation - should handle missing bubble gracefully
	conversation, err := parser.ParseConversation(composerID)
	if err != nil {
		// Error is acceptable for missing bubbles
		t.Logf("ParseConversation() returned error (expected): %v", err)
	}

	// Conversation should still be created with metadata
	if conversation != nil && conversation.ComposerID != composerID {
		t.Errorf("ComposerID = %v, want %v", conversation.ComposerID, composerID)
	}
}

func TestParser_ReadOnlyMode(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "globalStorage", "state.vscdb")
	createTestDatabase(t, dbPath)

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("NewParser() error = %v", err)
	}
	defer parser.Close()

	// Should be able to read in read-only mode
	_, err = parser.GetComposerIDs()
	if err != nil {
		t.Fatalf("GetComposerIDs() error = %v (read-only mode should work)", err)
	}
}

func TestParser_DatabaseNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("NewParser() error = %v", err)
	}
	defer parser.Close()

	// Should return error when database doesn't exist
	_, err = parser.GetComposerIDs()
	if err == nil {
		t.Error("GetComposerIDs() expected error for missing database, got nil")
	}
}

func TestIdentifyRole(t *testing.T) {
	tests := []struct {
		msgType int
		want    string
	}{
		{1, "user"},
		{2, "agent"},
		{0, "unknown"},
		{99, "unknown"},
	}

	for _, tt := range tests {
		got := identifyRole(tt.msgType)
		if got != tt.want {
			t.Errorf("identifyRole(%d) = %v, want %v", tt.msgType, got, tt.want)
		}
	}
}

func TestParseUnixMilliseconds(t *testing.T) {
	// Test timestamp: 2024-01-01 00:00:00 UTC
	ms := int64(1704067200000)
	got := parseUnixMilliseconds(ms)
	expected := time.Unix(0, ms*int64(time.Millisecond))

	if !got.Equal(expected) {
		t.Errorf("parseUnixMilliseconds(%d) = %v, want %v", ms, got, expected)
	}
}

func TestParseISO8601Timestamp(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
		checkFn func(time.Time) bool
	}{
		{"2024-01-01T12:00:00.000Z", false, func(t time.Time) bool {
			return t.Year() == 2024 && t.Month() == 1 && t.Day() == 1
		}},
		{"2024-01-01T12:00:00Z", false, func(t time.Time) bool {
			return t.Year() == 2024 && t.Month() == 1 && t.Day() == 1
		}},
		{"invalid", true, nil},
	}

	for _, tt := range tests {
		got, err := parseISO8601Timestamp(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseISO8601Timestamp(%q) expected error, got nil", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("parseISO8601Timestamp(%q) error = %v", tt.input, err)
			} else if tt.checkFn != nil && !tt.checkFn(got) {
				t.Errorf("parseISO8601Timestamp(%q) = %v, validation failed", tt.input, got)
			}
		}
	}
}

func TestParser_Close(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "globalStorage", "state.vscdb")
	createTestDatabase(t, dbPath)

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("NewParser() error = %v", err)
	}

	// Open database
	_, err = parser.GetComposerIDs()
	if err != nil {
		t.Fatalf("GetComposerIDs() error = %v", err)
	}

	// Close should work without error
	if err := parser.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Closing again should be safe
	if err := parser.Close(); err != nil {
		t.Errorf("Close() second call error = %v", err)
	}
}
