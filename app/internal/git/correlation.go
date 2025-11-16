package git

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/stwalsh4118/clio/internal/cursor"
	"github.com/stwalsh4118/clio/internal/logging"
)

const (
	// correlationWindow is the time window for correlating commits with conversations
	correlationWindow = 5 * time.Minute
	// maxProjectNameLength limits the length of normalized project names
	maxProjectNameLength = 255
	// defaultProjectName is returned when project name cannot be determined
	defaultProjectName = "unknown"
)

// CorrelationService defines the interface for correlating commits with sessions
type CorrelationService interface {
	CorrelateCommit(commit CommitMetadata, repository Repository, sessionManager cursor.SessionManager) (*CommitSessionCorrelation, error)
	CorrelateCommits(commits []CommitMetadata, repository Repository, sessionManager cursor.SessionManager) ([]CommitSessionCorrelation, error)
	GroupCommitsBySession(correlations []CommitSessionCorrelation) (map[string][]CommitSessionCorrelation, error)
}

// correlationService implements CorrelationService
type correlationService struct {
	logger logging.Logger
	db     *sql.DB
}

// NewCorrelationService creates a new correlation service instance
func NewCorrelationService(logger logging.Logger, db *sql.DB) (CorrelationService, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	if db == nil {
		return nil, fmt.Errorf("database cannot be nil")
	}

	return &correlationService{
		logger: logger.With("component", "git_correlation"),
		db:     db,
	}, nil
}

// CorrelateCommit correlates a single commit with sessions
func (cs *correlationService) CorrelateCommit(commit CommitMetadata, repository Repository, sessionManager cursor.SessionManager) (*CommitSessionCorrelation, error) {
	cs.logger.Debug("correlating commit with sessions", "commit", commit.Hash, "repository", repository.Path)

	// Validate commit timestamp
	if commit.Timestamp.IsZero() {
		cs.logger.Warn("commit has zero timestamp, cannot correlate", "commit", commit.Hash)
		return &CommitSessionCorrelation{
			CommitHash:      commit.Hash,
			SessionID:       "",
			Project:         cs.normalizeProjectName(repository.Path),
			CorrelationType: "none",
			TimeDiff:        0,
		}, nil
	}

	if sessionManager == nil {
		cs.logger.Debug("session manager is nil, returning no correlation", "commit", commit.Hash)
		return &CommitSessionCorrelation{
			CommitHash:      commit.Hash,
			SessionID:       "",
			Project:         cs.normalizeProjectName(repository.Path),
			CorrelationType: "none",
			TimeDiff:        0,
		}, nil
	}

	// Normalize repository path to project name
	projectName := cs.normalizeProjectName(repository.Path)
	cs.logger.Debug("normalized project name", "repository_path", repository.Path, "project_name", projectName)

	// Get all sessions (active + ended) from database
	sessions, err := cs.getAllSessions(sessionManager)
	if err != nil {
		cs.logger.Warn("failed to get sessions for correlation, returning no correlation", "error", err, "commit", commit.Hash, "project", projectName)
		return &CommitSessionCorrelation{
			CommitHash:      commit.Hash,
			SessionID:       "",
			Project:         projectName,
			CorrelationType: "none",
			TimeDiff:        0,
		}, nil
	}

	// Filter sessions by project
	matchingSessions := cs.filterSessionsByProject(sessions, projectName)
	if len(matchingSessions) == 0 {
		cs.logger.Debug("no matching sessions found for project", "project", projectName, "commit", commit.Hash, "total_sessions", len(sessions))
		return &CommitSessionCorrelation{
			CommitHash:      commit.Hash,
			SessionID:       "",
			Project:         projectName,
			CorrelationType: "none",
			TimeDiff:        0,
		}, nil
	}

	cs.logger.Debug("found matching sessions", "project", projectName, "matching_count", len(matchingSessions), "total_sessions", len(sessions))

	// Find best matching session
	bestMatch := cs.findBestMatchingSession(commit, matchingSessions)
	if bestMatch == nil {
		cs.logger.Debug("no matching session found for commit", "commit", commit.Hash, "project", projectName, "matching_sessions", len(matchingSessions))
		return &CommitSessionCorrelation{
			CommitHash:      commit.Hash,
			SessionID:       "",
			Project:         projectName,
			CorrelationType: "none",
			TimeDiff:        0,
		}, nil
	}

	cs.logger.Info("commit correlated with session", "commit", commit.Hash, "session_id", bestMatch.SessionID, "correlation_type", bestMatch.CorrelationType, "time_diff_ms", bestMatch.TimeDiff.Milliseconds())
	return bestMatch, nil
}

// CorrelateCommits correlates multiple commits with sessions
func (cs *correlationService) CorrelateCommits(commits []CommitMetadata, repository Repository, sessionManager cursor.SessionManager) ([]CommitSessionCorrelation, error) {
	cs.logger.Debug("correlating multiple commits", "commit_count", len(commits), "repository", repository.Path)
	correlations := make([]CommitSessionCorrelation, 0, len(commits))
	var failedCount int

	for _, commit := range commits {
		correlation, err := cs.CorrelateCommit(commit, repository, sessionManager)
		if err != nil {
			cs.logger.Warn("failed to correlate commit, skipping", "error", err, "commit", commit.Hash, "repository", repository.Path)
			failedCount++
			continue
		}
		if correlation != nil {
			correlations = append(correlations, *correlation)
		}
	}

	if failedCount > 0 {
		cs.logger.Warn("some commits failed correlation", "total", len(commits), "successful", len(correlations), "failed", failedCount)
	} else {
		cs.logger.Debug("correlated all commits", "total", len(commits), "correlated", len(correlations))
	}

	return correlations, nil
}

// GroupCommitsBySession groups correlated commits by session ID
func (cs *correlationService) GroupCommitsBySession(correlations []CommitSessionCorrelation) (map[string][]CommitSessionCorrelation, error) {
	grouped := make(map[string][]CommitSessionCorrelation)

	for _, correlation := range correlations {
		if correlation.SessionID == "" {
			// Commits with no correlation are grouped under empty string
			grouped[""] = append(grouped[""], correlation)
		} else {
			grouped[correlation.SessionID] = append(grouped[correlation.SessionID], correlation)
		}
	}

	return grouped, nil
}

// getAllSessions retrieves all sessions (active + ended) from the database
func (cs *correlationService) getAllSessions(sessionManager cursor.SessionManager) ([]*cursor.Session, error) {
	// Query database for all sessions (including ended ones)
	query := `
		SELECT id, project, start_time, end_time, last_activity, created_at, updated_at
		FROM sessions
		ORDER BY start_time DESC
	`

	rows, err := cs.db.Query(query)
	if err != nil {
		cs.logger.Error("failed to query sessions from database", "error", err)
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]*cursor.Session, 0)

	// Load all sessions from database
	for rows.Next() {
		var session cursor.Session
		var endTime sql.NullTime

		err := rows.Scan(
			&session.ID,
			&session.Project,
			&session.StartTime,
			&endTime,
			&session.LastActivity,
			&session.CreatedAt,
			&session.UpdatedAt,
		)
		if err != nil {
			cs.logger.Warn("failed to scan session row, skipping", "error", err)
			continue
		}

		if endTime.Valid {
			session.EndTime = &endTime.Time
		}

		// Load conversations for this session
		conversations, err := cs.getConversationsForSession(session.ID)
		if err != nil {
			cs.logger.Warn("failed to load conversations for session, using empty slice", "session_id", session.ID, "error", err)
			session.Conversations = []*cursor.Conversation{}
		} else {
			session.Conversations = conversations
			cs.logger.Debug("loaded conversations for session", "session_id", session.ID, "conversation_count", len(conversations))
		}

		sessions = append(sessions, &session)
	}

	if err := rows.Err(); err != nil {
		cs.logger.Error("error iterating sessions", "error", err)
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	cs.logger.Debug("loaded all sessions from database", "session_count", len(sessions))
	return sessions, nil
}

// getConversationsForSession retrieves conversations for a session from the database
func (cs *correlationService) getConversationsForSession(sessionID string) ([]*cursor.Conversation, error) {
	// Query conversations table
	// Note: id = composer_id in conversations table
	query := `
		SELECT id, composer_id, name, status, first_message_time, last_message_time, created_at
		FROM conversations
		WHERE session_id = ?
		ORDER BY first_message_time ASC
	`

	rows, err := cs.db.Query(query, sessionID)
	if err != nil {
		// If table doesn't exist, return empty slice (migrations may not have run yet)
		if strings.Contains(err.Error(), "no such table") {
			cs.logger.Debug("conversations table does not exist yet, returning empty slice", "session_id", sessionID)
			return []*cursor.Conversation{}, nil
		}
		cs.logger.Error("failed to query conversations from database", "session_id", sessionID, "error", err)
		return nil, fmt.Errorf("failed to query conversations: %w", err)
	}
	defer rows.Close()

	var conversations []*cursor.Conversation

	for rows.Next() {
		var conv cursor.Conversation
		var conversationID string // Store the conversation id (first column)
		var firstMsgTime, lastMsgTime sql.NullTime

		err := rows.Scan(
			&conversationID,  // id (conversation id, used for foreign key in messages table)
			&conv.ComposerID, // composer_id
			&conv.Name,
			&conv.Status,
			&firstMsgTime,
			&lastMsgTime,
			&conv.CreatedAt,
		)
		if err != nil {
			cs.logger.Warn("failed to scan conversation row, skipping", "session_id", sessionID, "error", err)
			continue
		}

		if firstMsgTime.Valid {
			conv.CreatedAt = firstMsgTime.Time
		}

		// Load messages for this conversation (conversation_id references conversations.id)
		messages, err := cs.getMessagesForConversation(conversationID)
		if err != nil {
			cs.logger.Warn("failed to load messages for conversation, using empty slice", "composer_id", conv.ComposerID, "conversation_id", conversationID, "error", err)
			conv.Messages = []cursor.Message{}
		} else {
			conv.Messages = messages
			cs.logger.Debug("loaded messages for conversation", "composer_id", conv.ComposerID, "message_count", len(messages))
		}

		conversations = append(conversations, &conv)
	}

	if err := rows.Err(); err != nil {
		cs.logger.Error("error iterating conversations", "session_id", sessionID, "error", err)
		return nil, fmt.Errorf("error iterating conversations: %w", err)
	}

	cs.logger.Debug("loaded conversations for session", "session_id", sessionID, "conversation_count", len(conversations))
	return conversations, nil
}

// getMessagesForConversation retrieves messages for a conversation from the database
// conversationID is the composer_id (which is also the conversation id in the conversations table)
func (cs *correlationService) getMessagesForConversation(conversationID string) ([]cursor.Message, error) {
	query := `
		SELECT bubble_id, type, role, content, thinking_text, code_blocks, tool_calls,
			has_code, has_thinking, has_tool_calls, content_source, created_at
		FROM messages
		WHERE conversation_id = ?
		ORDER BY created_at ASC
	`

	rows, err := cs.db.Query(query, conversationID)
	if err != nil {
		cs.logger.Error("failed to query messages from database", "conversation_id", conversationID, "error", err)
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []cursor.Message

	for rows.Next() {
		var msg cursor.Message
		var thinkingText, codeBlocks, toolCalls sql.NullString
		var hasCode, hasThinking, hasToolCalls int

		err := rows.Scan(
			&msg.BubbleID,
			&msg.Type,
			&msg.Role,
			&msg.Text,
			&thinkingText,
			&codeBlocks,
			&toolCalls,
			&hasCode,
			&hasThinking,
			&hasToolCalls,
			&msg.ContentSource,
			&msg.CreatedAt,
		)
		if err != nil {
			cs.logger.Warn("failed to scan message row, skipping", "conversation_id", conversationID, "error", err)
			continue
		}

		if thinkingText.Valid {
			msg.ThinkingText = thinkingText.String
		}
		// Set boolean flags from integer values
		msg.HasCode = hasCode == 1
		msg.HasToolCalls = hasToolCalls == 1
		msg.HasThinking = hasThinking == 1

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		cs.logger.Error("error iterating messages", "conversation_id", conversationID, "error", err)
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	cs.logger.Debug("loaded messages for conversation", "conversation_id", conversationID, "message_count", len(messages))
	return messages, nil
}

// filterSessionsByProject filters sessions by matching project name
func (cs *correlationService) filterSessionsByProject(sessions []*cursor.Session, projectName string) []*cursor.Session {
	matching := make([]*cursor.Session, 0)

	for _, session := range sessions {
		// Normalize session project name for comparison
		normalizedSessionProject := cs.normalizeProjectName(session.Project)
		if normalizedSessionProject == projectName {
			matching = append(matching, session)
		}
	}

	return matching
}

// findBestMatchingSession finds the best matching session for a commit
func (cs *correlationService) findBestMatchingSession(commit CommitMetadata, sessions []*cursor.Session) *CommitSessionCorrelation {
	cs.logger.Debug("finding best matching session", "commit", commit.Hash, "commit_time", commit.Timestamp, "session_count", len(sessions))
	var bestMatch *CommitSessionCorrelation
	var bestTimeDiff time.Duration = time.Duration(1<<63 - 1) // Max duration
	bestType := "none"

	commitTime := commit.Timestamp

	for _, session := range sessions {
		// Skip sessions with no conversations
		if len(session.Conversations) == 0 {
			continue
		}

		// Determine session time window
		sessionEnd := session.LastActivity
		if session.EndTime != nil {
			sessionEnd = *session.EndTime
		}

		// Find minimum time difference to any message in this session
		minTimeDiff := time.Duration(1<<63 - 1)
		foundWithinWindow := false

		for _, conv := range session.Conversations {
			for _, msg := range conv.Messages {
				diff := commitTime.Sub(msg.CreatedAt)
				if diff < 0 {
					diff = -diff
				}

				if diff < minTimeDiff {
					minTimeDiff = diff
				}

				// Check if within correlation window
				if diff <= correlationWindow {
					foundWithinWindow = true
				}
			}
		}

		// Determine correlation type
		correlationType := "none"
		isWithinSessionWindow := commitTime.After(session.StartTime) && commitTime.Before(sessionEnd.Add(time.Second))

		if isWithinSessionWindow && foundWithinWindow {
			correlationType = "active"
		} else if foundWithinWindow {
			correlationType = "proximate"
		}

		// Select best match: prefer "active" over "proximate" over "none"
		// For same type, prefer closer timestamp
		isBetter := false
		if correlationType == "active" && (bestType != "active" || minTimeDiff < bestTimeDiff) {
			isBetter = true
		} else if correlationType == "proximate" && bestType == "none" {
			isBetter = true
		} else if correlationType == "proximate" && bestType == "proximate" && minTimeDiff < bestTimeDiff {
			isBetter = true
		}

		if isBetter {
			bestMatch = &CommitSessionCorrelation{
				CommitHash:      commit.Hash,
				SessionID:       session.ID,
				Project:         session.Project,
				CorrelationType: correlationType,
				TimeDiff:        minTimeDiff,
			}
			bestTimeDiff = minTimeDiff
			bestType = correlationType
		}
	}

	return bestMatch
}

// normalizeProjectName normalizes a project path or name to a filesystem-safe project name
// This matches the logic from cursor.ProjectDetector.NormalizeProjectName
func (cs *correlationService) normalizeProjectName(name string) string {
	if name == "" {
		return defaultProjectName
	}

	// Handle file:// URIs
	if strings.HasPrefix(name, "file://") {
		parsedURL, err := url.Parse(name)
		if err == nil {
			name = parsedURL.Path
		} else {
			// If parsing fails, try to extract path manually
			if idx := strings.Index(name, "://"); idx != -1 {
				if pathIdx := strings.Index(name[idx+3:], "/"); pathIdx != -1 {
					name = name[idx+3+pathIdx:]
				}
			}
		}
	}

	// Extract directory name from full path
	name = filepath.Base(name)

	// Remove special characters that aren't filesystem-safe
	// Keep alphanumeric, dash, underscore, and dot
	reg := regexp.MustCompile(`[^a-zA-Z0-9._-]`)
	name = reg.ReplaceAllString(name, "-")

	// Convert to lowercase for consistency
	name = strings.ToLower(name)

	// Remove consecutive dashes
	reg = regexp.MustCompile(`-+`)
	name = reg.ReplaceAllString(name, "-")

	// Remove leading/trailing dashes
	name = strings.Trim(name, "-")

	// Limit length
	if len(name) > maxProjectNameLength {
		name = name[:maxProjectNameLength]
	}

	// If result is empty after normalization, return default
	if name == "" {
		return defaultProjectName
	}

	return name
}
