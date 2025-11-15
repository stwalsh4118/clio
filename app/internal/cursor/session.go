package cursor

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/stwalsh4118/clio/internal/config"
	"github.com/stwalsh4118/clio/internal/logging"
)

// Session represents a continuous development session containing multiple conversations
type Session struct {
	ID            string          `json:"id"`            // Unique session identifier
	Project       string          `json:"project"`       // Project name
	StartTime     time.Time       `json:"start_time"`    // When session started
	EndTime       *time.Time      `json:"end_time"`      // When session ended (nil if active)
	Conversations []*Conversation `json:"conversations"` // Conversations in this session
	LastActivity  time.Time       `json:"last_activity"` // Last conversation/message timestamp
	CreatedAt     time.Time       `json:"created_at"`    // When session record was created
	UpdatedAt     time.Time       `json:"updated_at"`    // When session was last updated
}

// IsActive returns true if the session is currently active (not ended)
func (s *Session) IsActive() bool {
	return s.EndTime == nil
}

// Duration returns the duration of the session
func (s *Session) Duration() time.Duration {
	endTime := time.Now()
	if s.EndTime != nil {
		endTime = *s.EndTime
	}
	return endTime.Sub(s.StartTime)
}

// SessionManager defines the interface for managing sessions
type SessionManager interface {
	GetOrCreateSession(project string, conversation *Conversation) (*Session, error)
	AddConversation(sessionID string, conversation *Conversation) error
	EndSession(sessionID string) error
	GetActiveSessions() ([]*Session, error)
	GetSession(sessionID string) (*Session, error)
	LoadSessions() error
	SaveSessions() error
	StartInactivityMonitor(ctx context.Context) error
	Stop() error
}

// sessionManager implements SessionManager for tracking development sessions
type sessionManager struct {
	config                  *config.Config
	db                      *sql.DB             // SQLite database connection
	storage                 ConversationStorage // Storage service for conversations
	logger                  logging.Logger      // Logger for structured logging
	sessions                map[string]*Session // All sessions keyed by session ID
	activeSessionsByProject map[string]string   // Active sessions keyed by project name
	mu                      sync.RWMutex        // Mutex for thread-safe access
	inactivityMonitorCtx    context.Context     // Context for inactivity monitor
	inactivityMonitorCancel context.CancelFunc  // Cancel function for inactivity monitor
	monitorRunning          bool                // Whether inactivity monitor is running
	monitorMu               sync.Mutex          // Mutex for monitor state
}

const (
	// inactivityCheckInterval is how often we check for inactive sessions
	inactivityCheckInterval = 1 * time.Minute
	// sessionIDLength is the length of random bytes for session ID suffix
	sessionIDLength = 8
)

// NewSessionManager creates a new session manager instance
// The database connection should already be initialized and migrated
func NewSessionManager(cfg *config.Config, database *sql.DB) (SessionManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if database == nil {
		return nil, fmt.Errorf("database cannot be nil")
	}

	// Create storage service
	storage, err := NewConversationStorage(database)
	if err != nil {
		return nil, fmt.Errorf("failed to create conversation storage: %w", err)
	}

	// Create logger (use component-specific logger)
	logger, err := logging.NewLogger(cfg)
	if err != nil {
		// If logger creation fails, use no-op logger (don't fail session manager creation)
		// This allows the system to work even if logging is misconfigured
		logger = logging.NewNoopLogger()
	}
	logger = logger.With("component", "session_manager")

	sm := &sessionManager{
		config:                  cfg,
		db:                      database,
		storage:                 storage,
		logger:                  logger,
		sessions:                make(map[string]*Session),
		activeSessionsByProject: make(map[string]string),
	}

	return sm, nil
}

// generateSessionID generates a unique session ID
func generateSessionID() (string, error) {
	// Use timestamp + random bytes for uniqueness
	timestamp := time.Now().Unix()
	randomBytes := make([]byte, sessionIDLength)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return fmt.Sprintf("%d-%s", timestamp, hex.EncodeToString(randomBytes)), nil
}

// GetOrCreateSession gets an active session for the project or creates a new one
func (sm *sessionManager) GetOrCreateSession(project string, conversation *Conversation) (*Session, error) {
	if conversation == nil {
		return nil, fmt.Errorf("conversation cannot be nil")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if there's an active session for this project
	if sessionID, exists := sm.activeSessionsByProject[project]; exists {
		session, found := sm.sessions[sessionID]
		if found && session.IsActive() {
			// Check if session is still within inactivity timeout
			timeout := time.Duration(sm.config.Session.InactivityTimeoutMinutes) * time.Minute
			if time.Since(session.LastActivity) < timeout {
				// Session is still active, update last activity and add conversation
				// Update LastActivity only if conversation.CreatedAt is later, or if LastActivity is zero
				if session.LastActivity.IsZero() || conversation.CreatedAt.After(session.LastActivity) {
					session.LastActivity = conversation.CreatedAt
				}
				session.Conversations = append(session.Conversations, conversation)
				session.UpdatedAt = time.Now()

				// Save session to database first (so conversation storage can verify it exists)
				if err := sm.saveSessionToDB(session); err != nil {
					// Log error but don't fail - session is still valid in memory
					sm.logger.Error("failed to save session to database", "error", err, "session_id", sessionID)
				}

				// Store conversation in database
				if err := sm.storage.StoreConversation(conversation, sessionID); err != nil {
					// Log error but don't fail - session is still valid in memory
					sm.logger.Error("failed to store conversation", "error", err, "session_id", sessionID, "composer_id", conversation.ComposerID)
				}

				return session, nil
			}
			// Session expired, end it
			now := time.Now()
			session.EndTime = &now
			delete(sm.activeSessionsByProject, project)
		}
	}

	// Create new session
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	now := time.Now()
	session := &Session{
		ID:            sessionID,
		Project:       project,
		StartTime:     now,
		EndTime:       nil,
		Conversations: []*Conversation{conversation},
		LastActivity:  conversation.CreatedAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	sm.sessions[sessionID] = session
	sm.activeSessionsByProject[project] = sessionID

	// Save session to database first (so conversation storage can verify it exists)
	if err := sm.saveSessionToDB(session); err != nil {
		// Log error but don't fail - session is still valid in memory
		sm.logger.Error("failed to save session to database", "error", err, "session_id", sessionID)
	}

	// Store conversation in database
	if err := sm.storage.StoreConversation(conversation, sessionID); err != nil {
		// Log error but don't fail - session is still valid in memory
		sm.logger.Error("failed to store conversation", "error", err, "session_id", sessionID, "composer_id", conversation.ComposerID)
	}

	sm.logger.Info("created new session", "session_id", sessionID, "project", project)

	return session, nil
}

// AddConversation adds a conversation to an existing session
func (sm *sessionManager) AddConversation(sessionID string, conversation *Conversation) error {
	if conversation == nil {
		return fmt.Errorf("conversation cannot be nil")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if !session.IsActive() {
		return fmt.Errorf("cannot add conversation to ended session: %s", sessionID)
	}

	// Add conversation
	session.Conversations = append(session.Conversations, conversation)

	// Update last activity if conversation is newer
	if conversation.CreatedAt.After(session.LastActivity) {
		session.LastActivity = conversation.CreatedAt
	}

	session.UpdatedAt = time.Now()

	// Save session to database first (so conversation storage can verify it exists)
	if err := sm.saveSessionToDB(session); err != nil {
		// Log error but don't fail - session is still valid in memory
		sm.logger.Error("failed to save session to database", "error", err, "session_id", sessionID)
	}

	// Store conversation in database
	if err := sm.storage.StoreConversation(conversation, sessionID); err != nil {
		// Log error and return it
		sm.logger.Error("failed to store conversation", "error", err, "session_id", sessionID, "composer_id", conversation.ComposerID)
		return fmt.Errorf("failed to store conversation: %w", err)
	}

	sm.logger.Info("added conversation to session", "session_id", sessionID, "composer_id", conversation.ComposerID)

	return nil
}

// EndSession ends an active session
func (sm *sessionManager) EndSession(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if !session.IsActive() {
		return nil // Already ended, no error
	}

	now := time.Now()
	session.EndTime = &now
	session.UpdatedAt = now

	// Remove from active sessions map
	delete(sm.activeSessionsByProject, session.Project)

	return nil
}

// GetActiveSessions returns all currently active sessions
func (sm *sessionManager) GetActiveSessions() ([]*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var activeSessions []*Session
	for _, session := range sm.sessions {
		if session.IsActive() {
			activeSessions = append(activeSessions, session)
		}
	}

	return activeSessions, nil
}

// GetSession retrieves a session by ID
func (sm *sessionManager) GetSession(sessionID string) (*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return session, nil
}

// LoadSessions loads sessions from the SQLite database
func (sm *sessionManager) LoadSessions() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	query := `
		SELECT id, project, start_time, end_time, last_activity, conversations_json, created_at, updated_at
		FROM sessions
	`

	rows, err := sm.db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	sm.sessions = make(map[string]*Session)
	sm.activeSessionsByProject = make(map[string]string)

	for rows.Next() {
		var session Session
		var endTime sql.NullTime
		var conversationsJSON sql.NullString

		err := rows.Scan(
			&session.ID,
			&session.Project,
			&session.StartTime,
			&endTime,
			&session.LastActivity,
			&conversationsJSON,
			&session.CreatedAt,
			&session.UpdatedAt,
		)
		if err != nil {
			continue // Skip invalid rows
		}

		if endTime.Valid {
			session.EndTime = &endTime.Time
		}

		// Load conversations from normalized storage
		conversations, err := sm.storage.GetConversationsBySession(session.ID)
		if err != nil {
			conversations = []*Conversation{} // Initialize empty slice on error
		}

		// If no conversations found in normalized storage but JSON exists, migrate from JSON
		if len(conversations) == 0 && conversationsJSON.Valid && conversationsJSON.String != "" {
			var jsonConversations []*Conversation
			if err := json.Unmarshal([]byte(conversationsJSON.String), &jsonConversations); err == nil && len(jsonConversations) > 0 {
				// Migrate JSON conversations to normalized storage
				// First ensure session exists in DB (it should, but be safe)
				if err := sm.saveSessionToDB(&session); err == nil {
					for _, conv := range jsonConversations {
						if err := sm.storage.StoreConversation(conv, session.ID); err == nil {
							conversations = append(conversations, conv)
						}
					}
				}
			}
		}

		session.Conversations = conversations

		sm.sessions[session.ID] = &session
		if session.IsActive() {
			sm.activeSessionsByProject[session.Project] = session.ID
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating sessions: %w", err)
	}

	return nil
}

// saveSessionToDB saves a single session to the database (without locking)
func (sm *sessionManager) saveSessionToDB(session *Session) error {
	var endTime interface{}
	if session.EndTime != nil {
		endTime = session.EndTime
	}

	_, err := sm.db.Exec(`
		INSERT INTO sessions (id, project, start_time, end_time, last_activity, conversations_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project = excluded.project,
			start_time = excluded.start_time,
			end_time = excluded.end_time,
			last_activity = excluded.last_activity,
			conversations_json = NULL,
			updated_at = excluded.updated_at
	`,
		session.ID,
		session.Project,
		session.StartTime,
		endTime,
		session.LastActivity,
		nil, // conversations_json is NULL - conversations stored in normalized tables
		session.CreatedAt,
		session.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save session %s: %w", session.ID, err)
	}

	return nil
}

// SaveSessions saves sessions to the SQLite database
func (sm *sessionManager) SaveSessions() error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Begin transaction
	tx, err := sm.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Upsert each session (conversations are stored separately in normalized tables)
	stmt, err := tx.Prepare(`
		INSERT INTO sessions (id, project, start_time, end_time, last_activity, conversations_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project = excluded.project,
			start_time = excluded.start_time,
			end_time = excluded.end_time,
			last_activity = excluded.last_activity,
			conversations_json = NULL,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, session := range sm.sessions {
		var endTime interface{}
		if session.EndTime != nil {
			endTime = session.EndTime
		}

		// conversations_json is set to NULL since conversations are stored in normalized tables
		_, err = stmt.Exec(
			session.ID,
			session.Project,
			session.StartTime,
			endTime,
			session.LastActivity,
			nil, // conversations_json is NULL - conversations stored in normalized tables
			session.CreatedAt,
			session.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to save session %s: %w", session.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// StartInactivityMonitor starts a background goroutine that checks for inactive sessions
func (sm *sessionManager) StartInactivityMonitor(ctx context.Context) error {
	sm.monitorMu.Lock()
	defer sm.monitorMu.Unlock()

	if sm.monitorRunning {
		return fmt.Errorf("inactivity monitor is already running")
	}

	monitorCtx, cancel := context.WithCancel(ctx)
	sm.inactivityMonitorCtx = monitorCtx
	sm.inactivityMonitorCancel = cancel
	sm.monitorRunning = true

	go sm.checkInactivity(monitorCtx)

	return nil
}

// checkInactivity periodically checks for inactive sessions and ends them
func (sm *sessionManager) checkInactivity(ctx context.Context) {
	ticker := time.NewTicker(inactivityCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sm.endInactiveSessions()
		}
	}
}

// endInactiveSessions ends sessions that have exceeded the inactivity timeout
func (sm *sessionManager) endInactiveSessions() {
	sm.mu.Lock()

	timeout := time.Duration(sm.config.Session.InactivityTimeoutMinutes) * time.Minute
	now := time.Now()

	var sessionsToEnd []string

	// Find inactive sessions
	for project, sessionID := range sm.activeSessionsByProject {
		session, exists := sm.sessions[sessionID]
		if !exists || !session.IsActive() {
			// Clean up invalid entries
			delete(sm.activeSessionsByProject, project)
			continue
		}

		if now.Sub(session.LastActivity) >= timeout {
			sessionsToEnd = append(sessionsToEnd, sessionID)
		}
	}

	// End inactive sessions
	for _, sessionID := range sessionsToEnd {
		session := sm.sessions[sessionID]
		if session != nil && session.IsActive() {
			session.EndTime = &now
			session.UpdatedAt = now
			delete(sm.activeSessionsByProject, session.Project)
		}
	}

	shouldSave := len(sessionsToEnd) > 0
	sm.mu.Unlock()

	// Save sessions if any were ended (outside of lock to avoid deadlock)
	if shouldSave {
		_ = sm.SaveSessions()
	}
}

// Stop stops the inactivity monitor and saves sessions
func (sm *sessionManager) Stop() error {
	sm.monitorMu.Lock()
	if sm.monitorRunning && sm.inactivityMonitorCancel != nil {
		sm.inactivityMonitorCancel()
		sm.monitorRunning = false
	}
	sm.monitorMu.Unlock()

	// Save sessions before stopping
	if err := sm.SaveSessions(); err != nil {
		return err
	}

	// Note: We do not close sm.db here because the database connection
	// is owned by the caller (passed in via constructor). The caller
	// is responsible for managing its lifecycle.

	return nil
}
