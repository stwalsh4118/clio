package git

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stwalsh4118/clio/internal/config"
	"github.com/stwalsh4118/clio/internal/cursor"
	"github.com/stwalsh4118/clio/internal/db"
	"github.com/stwalsh4118/clio/internal/logging"
	_ "modernc.org/sqlite"
)

func setupTestCorrelationDB(t *testing.T) (*sql.DB, func()) {
	// Create in-memory database
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Initialize database with migrations
	if err := db.RunMigrations(database); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	// Verify conversations table exists
	var tableName string
	err = database.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='conversations'").Scan(&tableName)
	if err != nil {
		if err == sql.ErrNoRows {
			t.Fatalf("conversations table was not created by migrations")
		}
		t.Fatalf("failed to verify conversations table: %v", err)
	}

	cleanup := func() {
		database.Close()
	}

	return database, cleanup
}

func createTestSession(t *testing.T, database *sql.DB, sessionID, project string, startTime, endTime time.Time) *cursor.Session {
	// Insert session into database
	var endTimeNull interface{}
	if !endTime.IsZero() {
		endTimeNull = endTime
	}

	_, err := database.Exec(`
		INSERT INTO sessions (id, project, start_time, end_time, last_activity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, project, startTime, endTimeNull, startTime, startTime, startTime)
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	session := &cursor.Session{
		ID:           sessionID,
		Project:      project,
		StartTime:    startTime,
		LastActivity: startTime,
		CreatedAt:    startTime,
		UpdatedAt:    startTime,
	}

	if !endTime.IsZero() {
		session.EndTime = &endTime
	}

	return session
}

func createTestConversation(t *testing.T, database *sql.DB, composerID, sessionID string, messages []cursor.Message) *cursor.Conversation {
	if len(messages) == 0 {
		t.Fatalf("conversation must have at least one message")
	}

	firstMsgTime := messages[0].CreatedAt
	lastMsgTime := messages[len(messages)-1].CreatedAt

	// Insert conversation
	_, err := database.Exec(`
		INSERT INTO conversations (id, session_id, composer_id, name, status, message_count, first_message_time, last_message_time, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, composerID, sessionID, composerID, "Test Conversation", "completed", len(messages), firstMsgTime, lastMsgTime, firstMsgTime, firstMsgTime)
	if err != nil {
		t.Fatalf("failed to create test conversation: %v", err)
	}

	// Insert messages
	for _, msg := range messages {
		_, err := database.Exec(`
			INSERT INTO messages (id, conversation_id, bubble_id, type, role, content, created_at, has_code, has_thinking, has_tool_calls, content_source)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, msg.BubbleID, composerID, msg.BubbleID, msg.Type, msg.Role, msg.Text, msg.CreatedAt, 0, 0, 0, "text")
		if err != nil {
			t.Fatalf("failed to create test message: %v", err)
		}
	}

	return &cursor.Conversation{
		ComposerID: composerID,
		Name:       "Test Conversation",
		Status:     "completed",
		CreatedAt:  firstMsgTime,
		Messages:   messages,
	}
}


func TestCorrelateCommit_ProximateSession(t *testing.T) {
	database, cleanup := setupTestCorrelationDB(t)
	defer cleanup()

	logger := logging.NewNoopLogger()
	service, err := NewCorrelationService(logger, database)
	if err != nil {
		t.Fatalf("failed to create correlation service: %v", err)
	}

	sessionManager := createMockSessionManager(t, database)

	// Create ended session
	now := time.Now()
	sessionStart := now.Add(-2 * time.Hour)
	sessionEnd := now.Add(-1 * time.Hour) // Session ended 1 hour ago

	session := createTestSession(t, database, "session-1", "my-project", sessionStart, sessionEnd)

	// Create conversation with message 3 minutes before commit (within window, but session ended)
	commitTime := now
	messageTime := commitTime.Add(-3 * time.Minute) // Within 5-minute window

	messages := []cursor.Message{
		{
			BubbleID:  "msg-1",
			Type:      1,
			Role:      "user",
			Text:      "Test message",
			CreatedAt: messageTime,
		},
	}

	conv := createTestConversation(t, database, "conv-1", session.ID, messages)
	session.Conversations = []*cursor.Conversation{conv}

	// Create commit
	commit := CommitMetadata{
		Hash:      "abc123",
		Message:   "Test commit",
		Timestamp: commitTime,
		Author: AuthorInfo{
			Name:  "Test User",
			Email: "test@example.com",
		},
		Branch: "main",
	}

	repository := Repository{
		Path: "/home/user/my-project",
		Name: "my-project",
	}

	// Correlate commit
	correlation, err := service.CorrelateCommit(commit, repository, sessionManager)
	if err != nil {
		t.Fatalf("failed to correlate commit: %v", err)
	}

	if correlation == nil {
		t.Fatal("correlation should not be nil")
	}

	if correlation.CorrelationType != "proximate" {
		t.Errorf("expected correlation type 'proximate', got '%s'", correlation.CorrelationType)
	}

	if correlation.SessionID != session.ID {
		t.Errorf("expected session ID '%s', got '%s'", session.ID, correlation.SessionID)
	}
}

func TestCorrelateCommit_NoCorrelation(t *testing.T) {
	database, cleanup := setupTestCorrelationDB(t)
	defer cleanup()

	logger := logging.NewNoopLogger()
	service, err := NewCorrelationService(logger, database)
	if err != nil {
		t.Fatalf("failed to create correlation service: %v", err)
	}

	sessionManager := createMockSessionManager(t, database)

	// Create commit outside correlation window
	now := time.Now()
	commitTime := now

	commit := CommitMetadata{
		Hash:      "abc123",
		Message:   "Test commit",
		Timestamp: commitTime,
		Author: AuthorInfo{
			Name:  "Test User",
			Email: "test@example.com",
		},
		Branch: "main",
	}

	repository := Repository{
		Path: "/home/user/my-project",
		Name: "my-project",
	}

	// Correlate commit (no sessions exist)
	correlation, err := service.CorrelateCommit(commit, repository, sessionManager)
	if err != nil {
		t.Fatalf("failed to correlate commit: %v", err)
	}

	if correlation == nil {
		t.Fatal("correlation should not be nil")
	}

	if correlation.CorrelationType != "none" {
		t.Errorf("expected correlation type 'none', got '%s'", correlation.CorrelationType)
	}

	if correlation.SessionID != "" {
		t.Errorf("expected empty session ID, got '%s'", correlation.SessionID)
	}
}

func TestCorrelateCommit_ProjectMatching(t *testing.T) {
	database, cleanup := setupTestCorrelationDB(t)
	defer cleanup()

	logger := logging.NewNoopLogger()
	service, err := NewCorrelationService(logger, database)
	if err != nil {
		t.Fatalf("failed to create correlation service: %v", err)
	}

	sessionManager := createMockSessionManager(t, database)

	// Create session for different project
	now := time.Now()
	sessionStart := now.Add(-1 * time.Hour)
	sessionEnd := now.Add(30 * time.Minute)

	session := createTestSession(t, database, "session-1", "other-project", sessionStart, sessionEnd)

	messageTime := now.Add(-4 * time.Minute)
	messages := []cursor.Message{
		{
			BubbleID:  "msg-1",
			Type:      1,
			Role:      "user",
			Text:      "Test message",
			CreatedAt: messageTime,
		},
	}

	conv := createTestConversation(t, database, "conv-1", session.ID, messages)
	session.Conversations = []*cursor.Conversation{conv}

	// Create commit for different project
	commit := CommitMetadata{
		Hash:      "abc123",
		Message:   "Test commit",
		Timestamp: now,
		Author: AuthorInfo{
			Name:  "Test User",
			Email: "test@example.com",
		},
		Branch: "main",
	}

	repository := Repository{
		Path: "/home/user/my-project",
		Name: "my-project",
	}

	// Correlate commit (should not match different project)
	correlation, err := service.CorrelateCommit(commit, repository, sessionManager)
	if err != nil {
		t.Fatalf("failed to correlate commit: %v", err)
	}

	if correlation == nil {
		t.Fatal("correlation should not be nil")
	}

	if correlation.CorrelationType != "none" {
		t.Errorf("expected correlation type 'none' for different project, got '%s'", correlation.CorrelationType)
	}
}

func TestCorrelateCommits_MultipleCommits(t *testing.T) {
	database, cleanup := setupTestCorrelationDB(t)
	defer cleanup()

	logger := logging.NewNoopLogger()
	service, err := NewCorrelationService(logger, database)
	if err != nil {
		t.Fatalf("failed to create correlation service: %v", err)
	}

	sessionManager := createMockSessionManager(t, database)

	// Create session
	now := time.Now()
	sessionStart := now.Add(-1 * time.Hour)
	sessionEnd := now.Add(30 * time.Minute)

	session := createTestSession(t, database, "session-1", "my-project", sessionStart, sessionEnd)

	messageTime := now.Add(-4 * time.Minute)
	messages := []cursor.Message{
		{
			BubbleID:  "msg-1",
			Type:      1,
			Role:      "user",
			Text:      "Test message",
			CreatedAt: messageTime,
		},
	}

	conv := createTestConversation(t, database, "conv-1", session.ID, messages)
	session.Conversations = []*cursor.Conversation{conv}

	// Create multiple commits
	commits := []CommitMetadata{
		{
			Hash:      "abc123",
			Message:   "Commit 1",
			Timestamp: now,
			Author:    AuthorInfo{Name: "Test User", Email: "test@example.com"},
			Branch:    "main",
		},
		{
			Hash:      "def456",
			Message:   "Commit 2",
			Timestamp: now.Add(1 * time.Minute),
			Author:    AuthorInfo{Name: "Test User", Email: "test@example.com"},
			Branch:    "main",
		},
	}

	repository := Repository{
		Path: "/home/user/my-project",
		Name: "my-project",
	}

	// Correlate commits
	correlations, err := service.CorrelateCommits(commits, repository, sessionManager)
	if err != nil {
		t.Fatalf("failed to correlate commits: %v", err)
	}

	if len(correlations) != 2 {
		t.Errorf("expected 2 correlations, got %d", len(correlations))
	}

	for _, corr := range correlations {
		if corr.CorrelationType != "active" {
			t.Errorf("expected correlation type 'active', got '%s'", corr.CorrelationType)
		}
	}
}

func TestGroupCommitsBySession(t *testing.T) {
	logger := logging.NewNoopLogger()
	database, _ := setupTestCorrelationDB(t)
	service, err := NewCorrelationService(logger, database)
	if err != nil {
		t.Fatalf("failed to create correlation service: %v", err)
	}

	correlations := []CommitSessionCorrelation{
		{
			CommitHash:      "abc123",
			SessionID:       "session-1",
			Project:         "my-project",
			CorrelationType: "active",
			TimeDiff:        2 * time.Minute,
		},
		{
			CommitHash:      "def456",
			SessionID:       "session-1",
			Project:         "my-project",
			CorrelationType: "active",
			TimeDiff:        3 * time.Minute,
		},
		{
			CommitHash:      "ghi789",
			SessionID:       "session-2",
			Project:         "my-project",
			CorrelationType: "proximate",
			TimeDiff:        4 * time.Minute,
		},
		{
			CommitHash:      "jkl012",
			SessionID:       "",
			Project:         "my-project",
			CorrelationType: "none",
			TimeDiff:        0,
		},
	}

	grouped, err := service.GroupCommitsBySession(correlations)
	if err != nil {
		t.Fatalf("failed to group commits: %v", err)
	}

	if len(grouped) != 3 {
		t.Errorf("expected 3 groups, got %d", len(grouped))
	}

	if len(grouped["session-1"]) != 2 {
		t.Errorf("expected 2 commits in session-1, got %d", len(grouped["session-1"]))
	}

	if len(grouped["session-2"]) != 1 {
		t.Errorf("expected 1 commit in session-2, got %d", len(grouped["session-2"]))
	}

	if len(grouped[""]) != 1 {
		t.Errorf("expected 1 commit with no session, got %d", len(grouped[""]))
	}
}

func TestNormalizeProjectName(t *testing.T) {
	logger := logging.NewNoopLogger()
	database, _ := setupTestCorrelationDB(t)
	service, err := NewCorrelationService(logger, database)
	if err != nil {
		t.Fatalf("failed to create correlation service: %v", err)
	}

	cs := service.(*correlationService)

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "absolute path",
			input:    "/home/user/my-project",
			expected: "my-project",
		},
		{
			name:     "relative path",
			input:    "./my-project",
			expected: "my-project",
		},
		{
			name:     "path with spaces",
			input:    "/home/user/my project",
			expected: "my-project",
		},
		{
			name:     "path with special chars",
			input:    "/home/user/my@project#123",
			expected: "my-project-123",
		},
		{
			name:     "uppercase",
			input:    "/home/user/MyProject",
			expected: "myproject",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "unknown",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := cs.normalizeProjectName(tc.input)
			if result != tc.expected {
				t.Errorf("input %q: expected %q, got %q", tc.input, tc.expected, result)
			}
		})
	}
}

// createMockSessionManager creates a minimal mock session manager for testing
func createMockSessionManager(t *testing.T, database *sql.DB) cursor.SessionManager {
	cfg := &config.Config{
		Session: config.SessionConfig{
			InactivityTimeoutMinutes: 30,
		},
	}

	// Create a real session manager (it's lightweight enough for tests)
	sm, err := cursor.NewSessionManager(cfg, database)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	return sm
}

