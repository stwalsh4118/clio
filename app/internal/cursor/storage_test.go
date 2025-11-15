package cursor

import (
	"testing"
	"time"

	"github.com/stwalsh4118/clio/internal/db"
	"github.com/stwalsh4118/clio/internal/logging"
)

// createTestConversation creates a test conversation with messages
func createTestConversationWithMessages(t *testing.T, composerID string, messageCount int, createdAt time.Time) *Conversation {
	conv := &Conversation{
		ComposerID: composerID,
		Name:       "Test Conversation " + composerID,
		Status:     "active",
		CreatedAt:  createdAt,
		Messages:   make([]Message, messageCount),
	}

	for i := 0; i < messageCount; i++ {
		msgType := 1
		role := "user"
		if i%2 == 1 {
			msgType = 2
			role = "agent"
		}
		conv.Messages[i] = Message{
			BubbleID:  "bubble-" + composerID + "-" + string(rune('0'+i)),
			Type:     msgType,
			Role:     role,
			Text:     "Message " + string(rune('0'+i)),
			CreatedAt: createdAt.Add(time.Duration(i) * time.Minute),
			Metadata:  make(map[string]interface{}),
		}
	}

	return conv
}

func TestNewConversationStorage(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	if storage == nil {
		t.Fatal("Storage is nil")
	}
}

func TestNewConversationStorage_NilDatabase(t *testing.T) {
	logger := logging.NewNoopLogger()
	_, err := NewConversationStorage(nil, logger)
	if err == nil {
		t.Fatal("Expected error for nil database")
	}
}

func TestNewConversationStorage_NilLogger(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	_, err = NewConversationStorage(database, nil)
	if err == nil {
		t.Fatal("Expected error for nil logger")
	}
}

func TestStoreConversation(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Create a session first
	sessionID := "test-session-1"
	_, err = database.Exec(`
		INSERT INTO sessions (id, project, start_time, end_time, last_activity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, "test-project", time.Now(), nil, time.Now(), time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	conv := createTestConversationWithMessages(t, "composer-1", 3, time.Now())
	err = storage.StoreConversation(conv, sessionID)
	if err != nil {
		t.Fatalf("Failed to store conversation: %v", err)
	}

	// Verify conversation was stored
	retrieved, err := storage.GetConversationByComposerID("composer-1")
	if err != nil {
		t.Fatalf("Failed to retrieve conversation: %v", err)
	}

	if retrieved.ComposerID != conv.ComposerID {
		t.Errorf("Expected ComposerID %s, got %s", conv.ComposerID, retrieved.ComposerID)
	}
	if retrieved.Name != conv.Name {
		t.Errorf("Expected Name %s, got %s", conv.Name, retrieved.Name)
	}
	if len(retrieved.Messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(retrieved.Messages))
	}
}

func TestStoreConversation_InvalidSession(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	conv := createTestConversationWithMessages(t, "composer-1", 1, time.Now())
	err = storage.StoreConversation(conv, "nonexistent-session")
	if err == nil {
		t.Fatal("Expected error for nonexistent session")
	}
}

func TestStoreConversation_TransactionRollback(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Create a session first
	sessionID := "test-session-2"
	_, err = database.Exec(`
		INSERT INTO sessions (id, project, start_time, end_time, last_activity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, "test-project", time.Now(), nil, time.Now(), time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Create conversation with invalid message (duplicate bubble ID will cause conflict)
	conv := createTestConversationWithMessages(t, "composer-2", 2, time.Now())
	conv.Messages[1].BubbleID = conv.Messages[0].BubbleID // Duplicate bubble ID

	// This should succeed (ON CONFLICT handles duplicates)
	err = storage.StoreConversation(conv, sessionID)
	if err != nil {
		t.Fatalf("StoreConversation should handle duplicates: %v", err)
	}
}

func TestStoreMessage(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Create a session and conversation first
	sessionID := "test-session-3"
	_, err = database.Exec(`
		INSERT INTO sessions (id, project, start_time, end_time, last_activity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, "test-project", time.Now(), nil, time.Now(), time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	conv := createTestConversationWithMessages(t, "composer-3", 1, time.Now())
	err = storage.StoreConversation(conv, sessionID)
	if err != nil {
		t.Fatalf("Failed to store conversation: %v", err)
	}

	// Add a new message
	newMsg := Message{
		BubbleID:  "bubble-new",
		Type:      2,
		Role:      "agent",
		Text:      "New message",
		CreatedAt: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	err = storage.StoreMessage(&newMsg, "composer-3")
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	// Verify message was added
	retrieved, err := storage.GetConversationByComposerID("composer-3")
	if err != nil {
		t.Fatalf("Failed to retrieve conversation: %v", err)
	}

	if len(retrieved.Messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(retrieved.Messages))
	}
}

func TestStoreMessage_InvalidConversation(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	msg := Message{
		BubbleID:  "bubble-1",
		Type:      1,
		Role:      "user",
		Text:      "Test",
		CreatedAt: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	err = storage.StoreMessage(&msg, "nonexistent-conversation")
	if err == nil {
		t.Fatal("Expected error for nonexistent conversation")
	}
}

func TestUpdateConversation(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Create a session first
	sessionID := "test-session-4"
	_, err = database.Exec(`
		INSERT INTO sessions (id, project, start_time, end_time, last_activity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, "test-project", time.Now(), nil, time.Now(), time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	conv := createTestConversationWithMessages(t, "composer-4", 2, time.Now())
	err = storage.StoreConversation(conv, sessionID)
	if err != nil {
		t.Fatalf("Failed to store conversation: %v", err)
	}

	// Add new messages
	newMessages := []*Message{
		{
			BubbleID:  "bubble-new-1",
			Type:      1,
			Role:      "user",
			Text:      "New message 1",
			CreatedAt: time.Now().Add(10 * time.Minute),
			Metadata:  make(map[string]interface{}),
		},
		{
			BubbleID:  "bubble-new-2",
			Type:      2,
			Role:      "agent",
			Text:      "New message 2",
			CreatedAt: time.Now().Add(11 * time.Minute),
			Metadata:  make(map[string]interface{}),
		},
	}

	err = storage.UpdateConversation("composer-4", newMessages)
	if err != nil {
		t.Fatalf("Failed to update conversation: %v", err)
	}

	// Verify messages were added
	retrieved, err := storage.GetConversationByComposerID("composer-4")
	if err != nil {
		t.Fatalf("Failed to retrieve conversation: %v", err)
	}

	if len(retrieved.Messages) != 4 {
		t.Errorf("Expected 4 messages, got %d", len(retrieved.Messages))
	}
}

func TestGetConversationByComposerID(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Create a session first
	sessionID := "test-session-5"
	_, err = database.Exec(`
		INSERT INTO sessions (id, project, start_time, end_time, last_activity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, "test-project", time.Now(), nil, time.Now(), time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	conv := createTestConversationWithMessages(t, "composer-5", 5, time.Now())
	err = storage.StoreConversation(conv, sessionID)
	if err != nil {
		t.Fatalf("Failed to store conversation: %v", err)
	}

	// Retrieve conversation
	retrieved, err := storage.GetConversationByComposerID("composer-5")
	if err != nil {
		t.Fatalf("Failed to retrieve conversation: %v", err)
	}

	if retrieved.ComposerID != "composer-5" {
		t.Errorf("Expected ComposerID composer-5, got %s", retrieved.ComposerID)
	}
	if len(retrieved.Messages) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(retrieved.Messages))
	}

	// Verify messages are ordered by created_at
	for i := 1; i < len(retrieved.Messages); i++ {
		if retrieved.Messages[i].CreatedAt.Before(retrieved.Messages[i-1].CreatedAt) {
			t.Errorf("Messages not ordered correctly: message %d is before message %d", i, i-1)
		}
	}
}

func TestGetConversationByComposerID_NotFound(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	_, err = storage.GetConversationByComposerID("nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent conversation")
	}
}

func TestGetConversationsBySession(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Create a session
	sessionID := "test-session-6"
	_, err = database.Exec(`
		INSERT INTO sessions (id, project, start_time, end_time, last_activity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, "test-project", time.Now(), nil, time.Now(), time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Store multiple conversations
	conv1 := createTestConversationWithMessages(t, "composer-6-1", 2, time.Now())
	conv2 := createTestConversationWithMessages(t, "composer-6-2", 3, time.Now().Add(5*time.Minute))

	err = storage.StoreConversation(conv1, sessionID)
	if err != nil {
		t.Fatalf("Failed to store conversation 1: %v", err)
	}

	err = storage.StoreConversation(conv2, sessionID)
	if err != nil {
		t.Fatalf("Failed to store conversation 2: %v", err)
	}

	// Retrieve conversations by session
	conversations, err := storage.GetConversationsBySession(sessionID)
	if err != nil {
		t.Fatalf("Failed to retrieve conversations: %v", err)
	}

	if len(conversations) != 2 {
		t.Errorf("Expected 2 conversations, got %d", len(conversations))
	}

	// Verify conversations are ordered by created_at
	if conversations[0].CreatedAt.After(conversations[1].CreatedAt) {
		t.Error("Conversations not ordered correctly")
	}
}

func TestGetConversationsBySession_Empty(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Create a session with no conversations
	sessionID := "test-session-7"
	_, err = database.Exec(`
		INSERT INTO sessions (id, project, start_time, end_time, last_activity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, "test-project", time.Now(), nil, time.Now(), time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	conversations, err := storage.GetConversationsBySession(sessionID)
	if err != nil {
		t.Fatalf("Failed to retrieve conversations: %v", err)
	}

	if len(conversations) != 0 {
		t.Errorf("Expected 0 conversations, got %d", len(conversations))
	}
}

func TestStoreConversation_MessageOrdering(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Create a session first
	sessionID := "test-session-8"
	_, err = database.Exec(`
		INSERT INTO sessions (id, project, start_time, end_time, last_activity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, "test-project", time.Now(), nil, time.Now(), time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Create conversation with messages in reverse chronological order
	baseTime := time.Now()
	conv := &Conversation{
		ComposerID: "composer-8",
		Name:       "Test Conversation",
		Status:     "active",
		CreatedAt:  baseTime,
		Messages: []Message{
			{
				BubbleID:  "bubble-3",
				Type:      1,
				Role:      "user",
				Text:      "Message 3",
				CreatedAt: baseTime.Add(3 * time.Minute),
				Metadata:  make(map[string]interface{}),
			},
			{
				BubbleID:  "bubble-1",
				Type:      1,
				Role:      "user",
				Text:      "Message 1",
				CreatedAt: baseTime,
				Metadata:  make(map[string]interface{}),
			},
			{
				BubbleID:  "bubble-2",
				Type:      2,
				Role:      "agent",
				Text:      "Message 2",
				CreatedAt: baseTime.Add(2 * time.Minute),
				Metadata:  make(map[string]interface{}),
			},
		},
	}

	err = storage.StoreConversation(conv, sessionID)
	if err != nil {
		t.Fatalf("Failed to store conversation: %v", err)
	}

	// Retrieve and verify messages are ordered correctly
	retrieved, err := storage.GetConversationByComposerID("composer-8")
	if err != nil {
		t.Fatalf("Failed to retrieve conversation: %v", err)
	}

	if len(retrieved.Messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(retrieved.Messages))
	}

	// Verify ordering
	if retrieved.Messages[0].BubbleID != "bubble-1" {
		t.Errorf("Expected first message bubble-1, got %s", retrieved.Messages[0].BubbleID)
	}
	if retrieved.Messages[1].BubbleID != "bubble-2" {
		t.Errorf("Expected second message bubble-2, got %s", retrieved.Messages[1].BubbleID)
	}
	if retrieved.Messages[2].BubbleID != "bubble-3" {
		t.Errorf("Expected third message bubble-3, got %s", retrieved.Messages[2].BubbleID)
	}
}

func TestStoreConversation_Metadata(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Create a session first
	sessionID := "test-session-9"
	_, err = database.Exec(`
		INSERT INTO sessions (id, project, start_time, end_time, last_activity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, "test-project", time.Now(), nil, time.Now(), time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	logger := logging.NewNoopLogger()
	storage, err := NewConversationStorage(database, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Create conversation with metadata
	conv := &Conversation{
		ComposerID: "composer-9",
		Name:       "Test Conversation",
		Status:     "active",
		CreatedAt:  time.Now(),
		Messages: []Message{
			{
				BubbleID:  "bubble-1",
				Type:      1,
				Role:      "user",
				Text:      "Test message",
				CreatedAt: time.Now(),
				Metadata: map[string]interface{}{
					"key1": "value1",
					"key2": 42,
				},
			},
		},
	}

	err = storage.StoreConversation(conv, sessionID)
	if err != nil {
		t.Fatalf("Failed to store conversation: %v", err)
	}

	// Retrieve and verify metadata
	retrieved, err := storage.GetConversationByComposerID("composer-9")
	if err != nil {
		t.Fatalf("Failed to retrieve conversation: %v", err)
	}

	if len(retrieved.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(retrieved.Messages))
	}

	metadata := retrieved.Messages[0].Metadata
	if metadata == nil {
		t.Fatal("Metadata is nil")
	}
	if metadata["key1"] != "value1" {
		t.Errorf("Expected metadata key1=value1, got %v", metadata["key1"])
	}
	if metadata["key2"] != float64(42) { // JSON numbers are float64
		t.Errorf("Expected metadata key2=42, got %v", metadata["key2"])
	}
}

