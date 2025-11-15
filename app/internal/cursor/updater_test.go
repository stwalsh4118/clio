package cursor

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stwalsh4118/clio/internal/db"
	"github.com/stwalsh4118/clio/internal/logging"
	_ "modernc.org/sqlite"
)

// createTestCursorDatabase creates a test Cursor database with sample conversation data
func createTestCursorDatabase(t *testing.T, dbPath string, composerID string, messageCount int) {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Open database
	cursorDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test Cursor database: %v", err)
	}
	defer cursorDB.Close()

	// Create cursorDiskKV table
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS cursorDiskKV (
		key TEXT UNIQUE ON CONFLICT REPLACE,
		value BLOB
	);`
	if _, err := cursorDB.Exec(createTableSQL); err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Create composer data with specified message count
	headers := make([]map[string]interface{}, messageCount)
	bubbles := make([]map[string]interface{}, messageCount)
	createdAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < messageCount; i++ {
		bubbleID := "bubble-" + composerID + "-" + strconv.Itoa(i)
		msgType := 1
		if i%2 == 1 {
			msgType = 2
		}
		headers[i] = map[string]interface{}{
			"bubbleId": bubbleID,
			"type":     msgType,
		}
		bubbles[i] = map[string]interface{}{
			"bubbleId":  bubbleID,
			"type":      msgType,
			"text":      "Message " + strconv.Itoa(i),
			"createdAt": createdAt.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
		}
	}

	composerData := map[string]interface{}{
		"composerId":                  composerID,
		"name":                        "Test Conversation",
		"status":                      "active",
		"createdAt":                   createdAt.UnixMilli(),
		"fullConversationHeadersOnly": headers,
	}

	// Insert composer data
	composerDataJSON, err := json.Marshal(composerData)
	if err != nil {
		t.Fatalf("Failed to marshal composer data: %v", err)
	}
	composerKey := "composerData:" + composerID
	if _, err := cursorDB.Exec("INSERT INTO cursorDiskKV (key, value) VALUES (?, ?)", composerKey, composerDataJSON); err != nil {
		t.Fatalf("Failed to insert composer data: %v", err)
	}

	// Insert message bubbles
	for _, bubble := range bubbles {
		bubbleJSON, err := json.Marshal(bubble)
		if err != nil {
			t.Fatalf("Failed to marshal bubble data: %v", err)
		}
		bubbleKey := "bubbleId:" + composerID + ":" + bubble["bubbleId"].(string)
		if _, err := cursorDB.Exec("INSERT INTO cursorDiskKV (key, value) VALUES (?, ?)", bubbleKey, bubbleJSON); err != nil {
			t.Fatalf("Failed to insert bubble data: %v", err)
		}
	}
}

func TestNewConversationUpdater(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	sessionManager, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	updater, err := NewConversationUpdater(cfg, database, parser, storage, sessionManager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}
	if updater == nil {
		t.Fatal("Updater is nil")
	}

	// Test nil config
	_, err = NewConversationUpdater(nil, database, parser, storage, sessionManager)
	if err == nil {
		t.Error("Expected error for nil config")
	}

	// Test nil database
	_, err = NewConversationUpdater(cfg, nil, parser, storage, sessionManager)
	if err == nil {
		t.Error("Expected error for nil database")
	}

	// Test nil parser
	_, err = NewConversationUpdater(cfg, database, nil, storage, sessionManager)
	if err == nil {
		t.Error("Expected error for nil parser")
	}

	// Test nil storage
	_, err = NewConversationUpdater(cfg, database, parser, nil, sessionManager)
	if err == nil {
		t.Error("Expected error for nil storage")
	}

	// Test nil session manager
	_, err = NewConversationUpdater(cfg, database, parser, storage, nil)
	if err == nil {
		t.Error("Expected error for nil session manager")
	}
}

func TestGetProcessedMessageCount(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	sessionManager, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	updater, err := NewConversationUpdater(cfg, database, parser, storage, sessionManager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	composerID := "test-composer-123"

	// Test not processed yet
	count, err := updater.GetProcessedMessageCount(composerID)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0, got %d", count)
	}

	// Mark as processed
	if err := updater.MarkAsProcessed(composerID, 5); err != nil {
		t.Fatalf("Failed to mark as processed: %v", err)
	}

	// Test processed count
	count, err = updater.GetProcessedMessageCount(composerID)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if count != 5 {
		t.Errorf("Expected 5, got %d", count)
	}

	// Update processed count
	if err := updater.MarkAsProcessed(composerID, 10); err != nil {
		t.Fatalf("Failed to update processed count: %v", err)
	}

	count, err = updater.GetProcessedMessageCount(composerID)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if count != 10 {
		t.Errorf("Expected 10, got %d", count)
	}
}

func TestHasBeenProcessed(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	sessionManager, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	updater, err := NewConversationUpdater(cfg, database, parser, storage, sessionManager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	composerID := "test-composer-456"

	// Not processed yet
	if updater.HasBeenProcessed(composerID, 5) {
		t.Error("Expected false for unprocessed conversation")
	}

	// Mark as processed with 5 messages
	if err := updater.MarkAsProcessed(composerID, 5); err != nil {
		t.Fatalf("Failed to mark as processed: %v", err)
	}

	// Check same count
	if !updater.HasBeenProcessed(composerID, 5) {
		t.Error("Expected true for processed conversation with same count")
	}

	// Check lower count
	if !updater.HasBeenProcessed(composerID, 3) {
		t.Error("Expected true for processed conversation with lower count")
	}

	// Check higher count
	if updater.HasBeenProcessed(composerID, 10) {
		t.Error("Expected false for processed conversation with higher count")
	}
}

func TestDetectUpdatedComposers(t *testing.T) {
	cfg := createTestConfig(t)

	// Create temporary Cursor database
	tempDir := t.TempDir()
	cursorDBPath := filepath.Join(tempDir, "globalStorage", "state.vscdb")
	cfg.Cursor.LogPath = tempDir

	// Create test Cursor database with 2 composers
	composerID1 := "composer-1"
	composerID2 := "composer-2"
	createTestCursorDatabase(t, cursorDBPath, composerID1, 5)
	createTestCursorDatabase(t, cursorDBPath, composerID2, 3)

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	sessionManager, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	updater, err := NewConversationUpdater(cfg, database, parser, storage, sessionManager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	// Initially, all composers should be detected as updated (not processed yet)
	updated, err := updater.DetectUpdatedComposers()
	if err != nil {
		t.Fatalf("Failed to detect updated composers: %v", err)
	}
	if len(updated) != 2 {
		t.Errorf("Expected 2 updated composers, got %d", len(updated))
	}

	// Mark composer 1 as processed with 5 messages
	if err := updater.MarkAsProcessed(composerID1, 5); err != nil {
		t.Fatalf("Failed to mark as processed: %v", err)
	}

	// Mark composer 2 as processed with 2 messages (less than current 3)
	if err := updater.MarkAsProcessed(composerID2, 2); err != nil {
		t.Fatalf("Failed to mark as processed: %v", err)
	}

	// Now only composer 2 should be detected as updated
	updated, err = updater.DetectUpdatedComposers()
	if err != nil {
		t.Fatalf("Failed to detect updated composers: %v", err)
	}
	if len(updated) != 1 {
		t.Errorf("Expected 1 updated composer, got %d", len(updated))
	}
	if len(updated) > 0 && updated[0] != composerID2 {
		t.Errorf("Expected composer-2, got %s", updated[0])
	}

	// Mark composer 2 as fully processed
	if err := updater.MarkAsProcessed(composerID2, 3); err != nil {
		t.Fatalf("Failed to mark as processed: %v", err)
	}

	// No composers should be updated now
	updated, err = updater.DetectUpdatedComposers()
	if err != nil {
		t.Fatalf("Failed to detect updated composers: %v", err)
	}
	if len(updated) != 0 {
		t.Errorf("Expected 0 updated composers, got %d", len(updated))
	}
}

func TestProcessUpdate(t *testing.T) {
	cfg := createTestConfig(t)

	// Create temporary Cursor database
	tempDir := t.TempDir()
	cursorDBPath := filepath.Join(tempDir, "globalStorage", "state.vscdb")
	cfg.Cursor.LogPath = tempDir

	composerID := "composer-update-test"
	createTestCursorDatabase(t, cursorDBPath, composerID, 5)

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	sessionManager, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	// Create a session and conversation first
	conv := createTestConversationWithMessages(t, composerID, 3, time.Now())
	project := "test-project"
	session, err := sessionManager.GetOrCreateSession(project, conv)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Store initial conversation (3 messages)
	if err := storage.StoreConversation(conv, session.ID); err != nil {
		t.Fatalf("Failed to store conversation: %v", err)
	}

	// Mark as processed with 3 messages
	updater, err := NewConversationUpdater(cfg, database, parser, storage, sessionManager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	if err := updater.MarkAsProcessed(composerID, 3); err != nil {
		t.Fatalf("Failed to mark as processed: %v", err)
	}

	// Now process update (should add 2 new messages: 4 and 5)
	if err := updater.ProcessUpdate(composerID); err != nil {
		t.Fatalf("Failed to process update: %v", err)
	}

	// Verify processed count was updated
	count, err := updater.GetProcessedMessageCount(composerID)
	if err != nil {
		t.Fatalf("Failed to get processed count: %v", err)
	}
	if count != 5 {
		t.Errorf("Expected processed count 5, got %d", count)
	}

	// Verify conversation was updated with new messages
	updatedConv, err := storage.GetConversationByComposerID(composerID)
	if err != nil {
		t.Fatalf("Failed to get updated conversation: %v", err)
	}
	if len(updatedConv.Messages) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(updatedConv.Messages))
	}

	// Process again (should do nothing)
	if err := updater.ProcessUpdate(composerID); err != nil {
		t.Fatalf("Failed to process update: %v", err)
	}

	// Count should still be 5
	count, err = updater.GetProcessedMessageCount(composerID)
	if err != nil {
		t.Fatalf("Failed to get processed count: %v", err)
	}
	if count != 5 {
		t.Errorf("Expected processed count 5, got %d", count)
	}
}

func TestProcessUpdate_NewConversation(t *testing.T) {
	cfg := createTestConfig(t)

	// Create temporary Cursor database
	tempDir := t.TempDir()
	cursorDBPath := filepath.Join(tempDir, "globalStorage", "state.vscdb")
	cfg.Cursor.LogPath = tempDir

	composerID := "composer-new-test"
	createTestCursorDatabase(t, cursorDBPath, composerID, 3)

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	sessionManager, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	updater, err := NewConversationUpdater(cfg, database, parser, storage, sessionManager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	// Process update for conversation that doesn't exist in our database
	// Should return without error (treats as new conversation)
	if err := updater.ProcessUpdate(composerID); err != nil {
		t.Fatalf("Unexpected error processing new conversation: %v", err)
	}

	// Should not be marked as processed
	count, err := updater.GetProcessedMessageCount(composerID)
	if err != nil {
		t.Fatalf("Failed to get processed count: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 for new conversation, got %d", count)
	}
}

func TestProcessUpdate_NoNewMessages(t *testing.T) {
	cfg := createTestConfig(t)

	// Create temporary Cursor database
	tempDir := t.TempDir()
	cursorDBPath := filepath.Join(tempDir, "globalStorage", "state.vscdb")
	cfg.Cursor.LogPath = tempDir

	composerID := "composer-no-update"
	createTestCursorDatabase(t, cursorDBPath, composerID, 3)

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	sessionManager, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	// Create and store conversation
	conv := createTestConversationWithMessages(t, composerID, 3, time.Now())
	project := "test-project"
	session, err := sessionManager.GetOrCreateSession(project, conv)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	if err := storage.StoreConversation(conv, session.ID); err != nil {
		t.Fatalf("Failed to store conversation: %v", err)
	}

	updater, err := NewConversationUpdater(cfg, database, parser, storage, sessionManager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	// Mark as fully processed
	if err := updater.MarkAsProcessed(composerID, 3); err != nil {
		t.Fatalf("Failed to mark as processed: %v", err)
	}

	// Process update (should do nothing)
	if err := updater.ProcessUpdate(composerID); err != nil {
		t.Fatalf("Failed to process update: %v", err)
	}

	// Count should still be 3
	count, err := updater.GetProcessedMessageCount(composerID)
	if err != nil {
		t.Fatalf("Failed to get processed count: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected processed count 3, got %d", count)
	}
}
