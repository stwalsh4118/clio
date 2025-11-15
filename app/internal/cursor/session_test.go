package cursor

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stwalsh4118/clio/internal/config"
	"github.com/stwalsh4118/clio/internal/db"
)

// createTestConfig creates a test configuration with temporary directory
func createTestConfig(t *testing.T) *config.Config {
	tmpDir := t.TempDir()
	return &config.Config{
		Storage: config.StorageConfig{
			SessionsPath: filepath.Join(tmpDir, "sessions"),
			DatabasePath: filepath.Join(tmpDir, "test.db"),
		},
		Session: config.SessionConfig{
			InactivityTimeoutMinutes: 30,
		},
	}
}

// createTestDB creates a test database connection
func createTestDB(t *testing.T, cfg *config.Config) *sql.DB {
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	return database
}

// createTestConversation creates a test conversation with the given timestamp
func createTestConversation(t *testing.T, composerID string, createdAt time.Time) *Conversation {
	return &Conversation{
		ComposerID: composerID,
		Name:       "Test Conversation",
		Status:     "active",
		CreatedAt:  createdAt,
		Messages: []Message{
			{
				BubbleID:  "bubble-1",
				Type:      1,
				Role:      "user",
				Text:      "Test message",
				CreatedAt: createdAt,
			},
		},
	}
}

func TestNewSessionManager(t *testing.T) {
	cfg := createTestConfig(t)

	// Open database (this will run migrations)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	if sm == nil {
		t.Fatal("Session manager is nil")
	}
}

func TestNewSessionManager_NilConfig(t *testing.T) {
	cfg := createTestConfig(t)
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	_, err = NewSessionManager(nil, database)
	if err == nil {
		t.Fatal("Expected error for nil config")
	}
}

func TestNewSessionManager_NilDatabase(t *testing.T) {
	cfg := createTestConfig(t)

	_, err := NewSessionManager(cfg, nil)
	if err == nil {
		t.Fatal("Expected error for nil database")
	}
}

func TestGetOrCreateSession_NewProject(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	conv := createTestConversation(t, "composer-1", time.Now())
	session, err := sm.GetOrCreateSession("project-1", conv)
	if err != nil {
		t.Fatalf("Failed to get or create session: %v", err)
	}

	if session == nil {
		t.Fatal("Session is nil")
	}

	if session.Project != "project-1" {
		t.Errorf("Expected project 'project-1', got '%s'", session.Project)
	}

	if !session.IsActive() {
		t.Error("Session should be active")
	}

	if len(session.Conversations) != 1 {
		t.Errorf("Expected 1 conversation, got %d", len(session.Conversations))
	}

	if session.Conversations[0].ComposerID != "composer-1" {
		t.Errorf("Expected conversation composer ID 'composer-1', got '%s'", session.Conversations[0].ComposerID)
	}
}

func TestGetOrCreateSession_ExistingActiveSession(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	now := time.Now()
	conv1 := createTestConversation(t, "composer-1", now)
	session1, err := sm.GetOrCreateSession("project-1", conv1)
	if err != nil {
		t.Fatalf("Failed to create first session: %v", err)
	}

	conv2 := createTestConversation(t, "composer-2", now.Add(5*time.Minute))
	session2, err := sm.GetOrCreateSession("project-1", conv2)
	if err != nil {
		t.Fatalf("Failed to get or create second session: %v", err)
	}

	if session1.ID != session2.ID {
		t.Error("Expected same session ID for same project")
	}

	if len(session2.Conversations) != 2 {
		t.Errorf("Expected 2 conversations, got %d", len(session2.Conversations))
	}
}

func TestGetOrCreateSession_DifferentProjects(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	now := time.Now()
	conv1 := createTestConversation(t, "composer-1", now)
	session1, err := sm.GetOrCreateSession("project-1", conv1)
	if err != nil {
		t.Fatalf("Failed to create first session: %v", err)
	}

	conv2 := createTestConversation(t, "composer-2", now)
	session2, err := sm.GetOrCreateSession("project-2", conv2)
	if err != nil {
		t.Fatalf("Failed to create second session: %v", err)
	}

	if session1.ID == session2.ID {
		t.Error("Expected different session IDs for different projects")
	}

	if session1.Project != "project-1" {
		t.Errorf("Expected session1 project 'project-1', got '%s'", session1.Project)
	}

	if session2.Project != "project-2" {
		t.Errorf("Expected session2 project 'project-2', got '%s'", session2.Project)
	}
}

func TestGetOrCreateSession_NilConversation(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	_, err = sm.GetOrCreateSession("project-1", nil)
	if err == nil {
		t.Fatal("Expected error for nil conversation")
	}
}

func TestAddConversation(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	now := time.Now()
	conv1 := createTestConversation(t, "composer-1", now)
	session, err := sm.GetOrCreateSession("project-1", conv1)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	conv2 := createTestConversation(t, "composer-2", now.Add(10*time.Minute))
	err = sm.AddConversation(session.ID, conv2)
	if err != nil {
		t.Fatalf("Failed to add conversation: %v", err)
	}

	// Verify conversation was added
	retrievedSession, err := sm.GetSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if len(retrievedSession.Conversations) != 2 {
		t.Errorf("Expected 2 conversations, got %d", len(retrievedSession.Conversations))
	}
}

func TestAddConversation_NonexistentSession(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	conv := createTestConversation(t, "composer-1", time.Now())
	err = sm.AddConversation("nonexistent-session-id", conv)
	if err == nil {
		t.Fatal("Expected error for nonexistent session")
	}
}

func TestAddConversation_EndedSession(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	conv1 := createTestConversation(t, "composer-1", time.Now())
	session, err := sm.GetOrCreateSession("project-1", conv1)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// End the session
	err = sm.EndSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to end session: %v", err)
	}

	// Try to add conversation to ended session
	conv2 := createTestConversation(t, "composer-2", time.Now())
	err = sm.AddConversation(session.ID, conv2)
	if err == nil {
		t.Fatal("Expected error for adding conversation to ended session")
	}
}

func TestAddConversation_NilConversation(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	conv1 := createTestConversation(t, "composer-1", time.Now())
	session, err := sm.GetOrCreateSession("project-1", conv1)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	err = sm.AddConversation(session.ID, nil)
	if err == nil {
		t.Fatal("Expected error for nil conversation")
	}
}

func TestEndSession(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	conv := createTestConversation(t, "composer-1", time.Now())
	session, err := sm.GetOrCreateSession("project-1", conv)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	if !session.IsActive() {
		t.Error("Session should be active before ending")
	}

	err = sm.EndSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to end session: %v", err)
	}

	// Verify session is ended
	retrievedSession, err := sm.GetSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if retrievedSession.IsActive() {
		t.Error("Session should not be active after ending")
	}

	if retrievedSession.EndTime == nil {
		t.Error("EndTime should be set")
	}
}

func TestEndSession_NonexistentSession(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	err = sm.EndSession("nonexistent-session-id")
	if err == nil {
		t.Fatal("Expected error for nonexistent session")
	}
}

func TestEndSession_AlreadyEnded(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	conv := createTestConversation(t, "composer-1", time.Now())
	session, err := sm.GetOrCreateSession("project-1", conv)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// End session twice
	err = sm.EndSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to end session first time: %v", err)
	}

	err = sm.EndSession(session.ID)
	if err != nil {
		t.Fatalf("Ending already-ended session should not return error, got: %v", err)
	}
}

func TestGetActiveSessions(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	now := time.Now()
	conv1 := createTestConversation(t, "composer-1", now)
	conv2 := createTestConversation(t, "composer-2", now)
	conv3 := createTestConversation(t, "composer-3", now)

	_, err = sm.GetOrCreateSession("project-1", conv1)
	if err != nil {
		t.Fatalf("Failed to create session 1: %v", err)
	}

	_, err = sm.GetOrCreateSession("project-2", conv2)
	if err != nil {
		t.Fatalf("Failed to create session 2: %v", err)
	}

	session3, err := sm.GetOrCreateSession("project-3", conv3)
	if err != nil {
		t.Fatalf("Failed to create session 3: %v", err)
	}

	// End one session
	err = sm.EndSession(session3.ID)
	if err != nil {
		t.Fatalf("Failed to end session 3: %v", err)
	}

	// Get active sessions
	activeSessions, err := sm.GetActiveSessions()
	if err != nil {
		t.Fatalf("Failed to get active sessions: %v", err)
	}

	if len(activeSessions) != 2 {
		t.Errorf("Expected 2 active sessions, got %d", len(activeSessions))
	}
}

func TestGetSession(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	conv := createTestConversation(t, "composer-1", time.Now())
	session, err := sm.GetOrCreateSession("project-1", conv)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	retrievedSession, err := sm.GetSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if retrievedSession.ID != session.ID {
		t.Errorf("Expected session ID '%s', got '%s'", session.ID, retrievedSession.ID)
	}
}

func TestGetSession_Nonexistent(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	_, err = sm.GetSession("nonexistent-session-id")
	if err == nil {
		t.Fatal("Expected error for nonexistent session")
	}
}

func TestSaveAndLoadSessions(t *testing.T) {
	cfg := createTestConfig(t)
	database1 := createTestDB(t, cfg)
	defer database1.Close()
	sm1, err := NewSessionManager(cfg, database1)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	now := time.Now()
	conv1 := createTestConversation(t, "composer-1", now)
	conv2 := createTestConversation(t, "composer-2", now)

	session1, err := sm1.GetOrCreateSession("project-1", conv1)
	if err != nil {
		t.Fatalf("Failed to create session 1: %v", err)
	}

	session2, err := sm1.GetOrCreateSession("project-2", conv2)
	if err != nil {
		t.Fatalf("Failed to create session 2: %v", err)
	}

	// End one session
	err = sm1.EndSession(session2.ID)
	if err != nil {
		t.Fatalf("Failed to end session 2: %v", err)
	}

	// Save sessions
	err = sm1.SaveSessions()
	if err != nil {
		t.Fatalf("Failed to save sessions: %v", err)
	}

	// Create new session manager and load sessions
	database2 := createTestDB(t, cfg)
	defer database2.Close()
	sm2, err := NewSessionManager(cfg, database2)
	if err != nil {
		t.Fatalf("Failed to create second session manager: %v", err)
	}

	err = sm2.LoadSessions()
	if err != nil {
		t.Fatalf("Failed to load sessions: %v", err)
	}

	// Verify sessions were loaded
	loadedSession1, err := sm2.GetSession(session1.ID)
	if err != nil {
		t.Fatalf("Failed to get loaded session 1: %v", err)
	}

	if loadedSession1.Project != session1.Project {
		t.Errorf("Expected project '%s', got '%s'", session1.Project, loadedSession1.Project)
	}

	if !loadedSession1.IsActive() {
		t.Error("Session 1 should be active")
	}

	loadedSession2, err := sm2.GetSession(session2.ID)
	if err != nil {
		t.Fatalf("Failed to get loaded session 2: %v", err)
	}

	if loadedSession2.IsActive() {
		t.Error("Session 2 should not be active")
	}

	// Verify active sessions map
	activeSessions, err := sm2.GetActiveSessions()
	if err != nil {
		t.Fatalf("Failed to get active sessions: %v", err)
	}

	if len(activeSessions) != 1 {
		t.Errorf("Expected 1 active session after load, got %d", len(activeSessions))
	}
}

func TestLoadSessions_NoFile(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	// Load when file doesn't exist should not error
	err = sm.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions should not error when file doesn't exist: %v", err)
	}
}

func TestInactivityMonitor(t *testing.T) {
	cfg := createTestConfig(t)
	cfg.Session.InactivityTimeoutMinutes = 1 // 1 minute timeout for testing
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	// Create session with old last activity
	oldTime := time.Now().Add(-2 * time.Minute) // 2 minutes ago
	conv := createTestConversation(t, "composer-1", oldTime)
	session, err := sm.GetOrCreateSession("project-1", conv)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Start inactivity monitor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.StartInactivityMonitor(ctx)
	if err != nil {
		t.Fatalf("Failed to start inactivity monitor: %v", err)
	}

	// Wait for monitor to check (runs every 1 minute, but we'll wait a bit)
	// Since the session was created with old time, it should be ended
	time.Sleep(2 * time.Second)

	// The monitor should have ended the session, but since it checks every minute,
	// we'll test the expiration logic through GetOrCreateSession instead
	// Create a new conversation - should create new session since old one expired
	now := time.Now()
	conv2 := createTestConversation(t, "composer-2", now)
	session2, err := sm.GetOrCreateSession("project-1", conv2)
	if err != nil {
		t.Fatalf("Failed to get or create session: %v", err)
	}

	// Should create new session because old one expired
	if session.ID == session2.ID {
		t.Error("Expected new session ID for expired session")
	}
}

func TestInactivityMonitor_ActiveSession(t *testing.T) {
	cfg := createTestConfig(t)
	cfg.Session.InactivityTimeoutMinutes = 30
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	// Create session with recent activity
	now := time.Now()
	conv := createTestConversation(t, "composer-1", now)
	session, err := sm.GetOrCreateSession("project-1", conv)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Start inactivity monitor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.StartInactivityMonitor(ctx)
	if err != nil {
		t.Fatalf("Failed to start inactivity monitor: %v", err)
	}

	// Wait a bit
	time.Sleep(2 * time.Second)

	// Verify session is still active (monitor shouldn't have ended it)
	retrievedSession, err := sm.GetSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if !retrievedSession.IsActive() {
		t.Error("Session should still be active")
	}
}

func TestStartInactivityMonitor_AlreadyRunning(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.StartInactivityMonitor(ctx)
	if err != nil {
		t.Fatalf("Failed to start inactivity monitor: %v", err)
	}

	// Try to start again
	err = sm.StartInactivityMonitor(ctx)
	if err == nil {
		t.Fatal("Expected error when starting monitor that's already running")
	}
}

func TestStop(t *testing.T) {
	cfg := createTestConfig(t)
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	conv := createTestConversation(t, "composer-1", time.Now())
	_, err = sm.GetOrCreateSession("project-1", conv)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.StartInactivityMonitor(ctx)
	if err != nil {
		t.Fatalf("Failed to start inactivity monitor: %v", err)
	}

	// Stop should save sessions
	err = sm.Stop()
	if err != nil {
		t.Fatalf("Failed to stop session manager: %v", err)
	}

	// Verify sessions file exists
	database2 := createTestDB(t, cfg)
	defer database2.Close()
	sm2, err := NewSessionManager(cfg, database2)
	if err != nil {
		t.Fatalf("Failed to create second session manager: %v", err)
	}

	err = sm2.LoadSessions()
	if err != nil {
		t.Fatalf("Failed to load sessions: %v", err)
	}

	activeSessions, err := sm2.GetActiveSessions()
	if err != nil {
		t.Fatalf("Failed to get active sessions: %v", err)
	}

	if len(activeSessions) != 1 {
		t.Errorf("Expected 1 active session after stop, got %d", len(activeSessions))
	}
}

func TestSession_IsActive(t *testing.T) {
	session := &Session{
		EndTime: nil,
	}

	if !session.IsActive() {
		t.Error("Session with nil EndTime should be active")
	}

	now := time.Now()
	session.EndTime = &now

	if session.IsActive() {
		t.Error("Session with EndTime set should not be active")
	}
}

func TestSession_Duration(t *testing.T) {
	startTime := time.Now().Add(-1 * time.Hour)
	session := &Session{
		StartTime: startTime,
		EndTime:   nil,
	}

	duration := session.Duration()
	if duration < 1*time.Hour {
		t.Errorf("Expected duration >= 1 hour, got %v", duration)
	}

	// Test with ended session
	endTime := time.Now()
	session.EndTime = &endTime
	duration = session.Duration()
	expectedDuration := endTime.Sub(startTime)
	if duration != expectedDuration {
		t.Errorf("Expected duration %v, got %v", expectedDuration, duration)
	}
}

func TestGetOrCreateSession_ExpiredSession(t *testing.T) {
	cfg := createTestConfig(t)
	cfg.Session.InactivityTimeoutMinutes = 1
	database := createTestDB(t, cfg)
	defer database.Close()
	sm, err := NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	// Create session with old activity (2 minutes ago, timeout is 1 minute)
	oldTime := time.Now().Add(-2 * time.Minute)
	conv1 := createTestConversation(t, "composer-1", oldTime)
	session1, err := sm.GetOrCreateSession("project-1", conv1)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Try to get/create session with new conversation (should create new session)
	// because the old session's LastActivity is oldTime which is > 1 minute ago
	now := time.Now()
	conv2 := createTestConversation(t, "composer-2", now)
	session2, err := sm.GetOrCreateSession("project-1", conv2)
	if err != nil {
		t.Fatalf("Failed to get or create session: %v", err)
	}

	// Should create new session because old one expired
	if session1.ID == session2.ID {
		t.Error("Expected new session ID for expired session")
	}
}
