package git

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/stwalsh4118/clio/internal/logging"
)

// CommitStorage defines the interface for storing and retrieving commits and file changes
type CommitStorage interface {
	StoreCommit(commit *Commit, diff *CommitDiff, correlation *CommitSessionCorrelation, repository *Repository, sessionID string) error
	GetCommit(commitHash string) (*StoredCommit, error)
	GetCommitsBySession(sessionID string) ([]*StoredCommit, error)
	GetCommitsByRepository(repoPath string) ([]*StoredCommit, error)
}

// StoredCommit represents a commit retrieved from the database
type StoredCommit struct {
	ID              string
	SessionID       *string
	RepositoryPath  string
	RepositoryName  string
	Hash            string
	Message         string
	AuthorName      string
	AuthorEmail     string
	Timestamp       time.Time
	Branch          string
	IsMerge         bool
	ParentHashes    []string
	FullDiff        string
	DiffTruncated   bool
	DiffTruncatedAt *int
	CorrelationType *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Files           []StoredFileDiff
}

// StoredFileDiff represents a file diff retrieved from the database
type StoredFileDiff struct {
	ID           string
	CommitID     string
	FilePath     string
	LinesAdded   int
	LinesRemoved int
	Diff         string
	CreatedAt    time.Time
}

// commitStorage implements CommitStorage for database persistence
type commitStorage struct {
	db     *sql.DB
	logger logging.Logger
}

// NewCommitStorage creates a new commit storage instance
func NewCommitStorage(db *sql.DB, logger logging.Logger) (CommitStorage, error) {
	if db == nil {
		return nil, fmt.Errorf("database cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	// Use component-specific logger
	logger = logger.With("component", "commit_storage")

	return &commitStorage{
		db:     db,
		logger: logger,
	}, nil
}

// StoreCommit stores a commit and all its file changes in a single transaction
func (cs *commitStorage) StoreCommit(commit *Commit, diff *CommitDiff, correlation *CommitSessionCorrelation, repository *Repository, sessionID string) error {
	if commit == nil {
		return fmt.Errorf("commit cannot be nil")
	}
	if repository == nil {
		return fmt.Errorf("repository cannot be nil")
	}

	// Calculate file count safely, handling nil diff
	fileCount := 0
	if diff != nil {
		fileCount = len(diff.Files)
	}

	cs.logger.Debug("storing commit", "hash", commit.Hash, "session_id", sessionID, "file_count", fileCount)

	// Verify session exists if sessionID is provided
	if sessionID != "" {
		var exists bool
		err := cs.db.QueryRow("SELECT EXISTS(SELECT 1 FROM sessions WHERE id = ?)", sessionID).Scan(&exists)
		if err != nil {
			cs.logger.Error("failed to verify session exists", "session_id", sessionID, "error", err)
			return fmt.Errorf("failed to verify session exists: %w", err)
		}
		if !exists {
			cs.logger.Error("session not found", "session_id", sessionID, "commit_hash", commit.Hash)
			return fmt.Errorf("session not found: %s", sessionID)
		}
	}

	// Begin transaction
	cs.logger.Debug("starting transaction for commit storage", "hash", commit.Hash)
	tx, err := cs.db.Begin()
	if err != nil {
		cs.logger.Error("failed to begin transaction", "hash", commit.Hash, "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Marshal parent hashes to JSON
	var parentHashesJSON sql.NullString
	if len(commit.Parents) > 0 {
		parentHashesBytes, err := json.Marshal(commit.Parents)
		if err != nil {
			cs.logger.Warn("failed to marshal parent hashes", "hash", commit.Hash, "error", err)
			return fmt.Errorf("failed to marshal parent hashes: %w", err)
		}
		parentHashesJSON = sql.NullString{String: string(parentHashesBytes), Valid: true}
	}

	// Convert boolean flags to integers
	isMergeInt := 0
	if commit.IsMerge {
		isMergeInt = 1
	}

	diffTruncatedInt := 0
	if diff != nil && diff.IsTruncated {
		diffTruncatedInt = 1
	}

	// Handle nullable fields
	var sessionIDNull sql.NullString
	if sessionID != "" {
		sessionIDNull = sql.NullString{String: sessionID, Valid: true}
	}

	var correlationTypeNull sql.NullString
	if correlation != nil && correlation.CorrelationType != "" {
		correlationTypeNull = sql.NullString{String: correlation.CorrelationType, Valid: true}
	}

	var diffTruncatedAtNull sql.NullInt64
	if diff != nil && diff.IsTruncated && diff.TruncatedAt > 0 {
		diffTruncatedAtNull = sql.NullInt64{Int64: int64(diff.TruncatedAt), Valid: true}
	}

	var fullDiffNull sql.NullString
	if diff != nil && diff.FullDiff != "" {
		fullDiffNull = sql.NullString{String: diff.FullDiff, Valid: true}
	}

	now := time.Now()

	// Store commit (use commit hash as primary key)
	_, err = tx.Exec(`
		INSERT INTO commits (
			id, session_id, repository_path, repository_name, hash, message,
			author_name, author_email, timestamp, branch, is_merge, parent_hashes,
			full_diff, diff_truncated, diff_truncated_at, correlation_type,
			created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			session_id = excluded.session_id,
			repository_path = excluded.repository_path,
			repository_name = excluded.repository_name,
			message = excluded.message,
			author_name = excluded.author_name,
			author_email = excluded.author_email,
			timestamp = excluded.timestamp,
			branch = excluded.branch,
			is_merge = excluded.is_merge,
			parent_hashes = excluded.parent_hashes,
			full_diff = excluded.full_diff,
			diff_truncated = excluded.diff_truncated,
			diff_truncated_at = excluded.diff_truncated_at,
			correlation_type = excluded.correlation_type,
			updated_at = excluded.updated_at
	`,
		commit.Hash, // id = commit hash
		sessionIDNull,
		repository.Path,
		repository.Name,
		commit.Hash,
		commit.Message,
		commit.Author,
		commit.Email,
		commit.Timestamp,
		commit.Branch,
		isMergeInt,
		parentHashesJSON,
		fullDiffNull,
		diffTruncatedInt,
		diffTruncatedAtNull,
		correlationTypeNull,
		now,
		now,
	)
	if err != nil {
		cs.logger.Error("failed to store commit", "hash", commit.Hash, "session_id", sessionID, "error", err)
		return fmt.Errorf("failed to store commit: %w", err)
	}

	// Store all file changes
	if diff != nil {
		for _, fileDiff := range diff.Files {
			if err := cs.storeFileDiffInTx(tx, &fileDiff, commit.Hash); err != nil {
				cs.logger.Error("failed to store file diff", "hash", commit.Hash, "file_path", fileDiff.Path, "error", err)
				return fmt.Errorf("failed to store file diff %s: %w", fileDiff.Path, err)
			}
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		cs.logger.Error("failed to commit transaction", "hash", commit.Hash, "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	cs.logger.Info("stored commit", "hash", commit.Hash, "session_id", sessionID, "file_count", fileCount)
	return nil
}

// storeFileDiffInTx stores a file diff within an existing transaction
func (cs *commitStorage) storeFileDiffInTx(tx *sql.Tx, fileDiff *FileDiff, commitID string) error {
	// Generate UUID for file diff ID
	fileDiffID := uuid.New().String()

	var diffNull sql.NullString
	if fileDiff.Diff != "" {
		diffNull = sql.NullString{String: fileDiff.Diff, Valid: true}
	}

	now := time.Now()

	_, err := tx.Exec(`
		INSERT INTO commit_files (
			id, commit_id, file_path, lines_added, lines_removed, diff, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(commit_id, file_path) DO UPDATE SET
			lines_added = excluded.lines_added,
			lines_removed = excluded.lines_removed,
			diff = excluded.diff
	`,
		fileDiffID,
		commitID,
		fileDiff.Path,
		fileDiff.LinesAdded,
		fileDiff.LinesRemoved,
		diffNull,
		now,
	)
	if err != nil {
		cs.logger.Error("failed to insert file diff", "commit_id", commitID, "file_path", fileDiff.Path, "error", err)
		return fmt.Errorf("failed to insert file diff: %w", err)
	}

	cs.logger.Debug("stored file diff", "commit_id", commitID, "file_path", fileDiff.Path, "lines_added", fileDiff.LinesAdded, "lines_removed", fileDiff.LinesRemoved)
	return nil
}

// GetCommit retrieves a commit by its hash
func (cs *commitStorage) GetCommit(commitHash string) (*StoredCommit, error) {
	if commitHash == "" {
		return nil, fmt.Errorf("commit hash cannot be empty")
	}

	cs.logger.Debug("retrieving commit by hash", "hash", commitHash)

	// Query commit
	var commit StoredCommit
	var sessionIDNull, correlationTypeNull, parentHashesJSON, fullDiffNull sql.NullString
	var diffTruncatedAtNull sql.NullInt64
	var isMergeInt, diffTruncatedInt int

	err := cs.db.QueryRow(`
		SELECT id, session_id, repository_path, repository_name, hash, message,
			author_name, author_email, timestamp, branch, is_merge, parent_hashes,
			full_diff, diff_truncated, diff_truncated_at, correlation_type,
			created_at, updated_at
		FROM commits
		WHERE hash = ?
	`, commitHash).Scan(
		&commit.ID,
		&sessionIDNull,
		&commit.RepositoryPath,
		&commit.RepositoryName,
		&commit.Hash,
		&commit.Message,
		&commit.AuthorName,
		&commit.AuthorEmail,
		&commit.Timestamp,
		&commit.Branch,
		&isMergeInt,
		&parentHashesJSON,
		&fullDiffNull,
		&diffTruncatedInt,
		&diffTruncatedAtNull,
		&correlationTypeNull,
		&commit.CreatedAt,
		&commit.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			cs.logger.Debug("commit not found", "hash", commitHash)
			return nil, fmt.Errorf("commit not found: %s", commitHash)
		}
		cs.logger.Error("failed to query commit", "hash", commitHash, "error", err)
		return nil, fmt.Errorf("failed to query commit: %w", err)
	}

	// Parse nullable fields
	if sessionIDNull.Valid {
		commit.SessionID = &sessionIDNull.String
	}
	if correlationTypeNull.Valid {
		commit.CorrelationType = &correlationTypeNull.String
	}
	if diffTruncatedAtNull.Valid {
		truncatedAt := int(diffTruncatedAtNull.Int64)
		commit.DiffTruncatedAt = &truncatedAt
	}
	if fullDiffNull.Valid {
		commit.FullDiff = fullDiffNull.String
	}

	commit.IsMerge = isMergeInt == 1
	commit.DiffTruncated = diffTruncatedInt == 1

	// Parse parent hashes JSON
	if parentHashesJSON.Valid && parentHashesJSON.String != "" {
		if err := json.Unmarshal([]byte(parentHashesJSON.String), &commit.ParentHashes); err != nil {
			cs.logger.Warn("failed to parse parent hashes JSON, using empty slice", "hash", commitHash, "error", err)
			commit.ParentHashes = []string{}
		}
	} else {
		commit.ParentHashes = []string{}
	}

	// Query file changes
	files, err := cs.getFileDiffsByCommitID(commitHash)
	if err != nil {
		cs.logger.Error("failed to get file diffs", "hash", commitHash, "error", err)
		return nil, fmt.Errorf("failed to get file diffs: %w", err)
	}

	commit.Files = files
	cs.logger.Info("retrieved commit", "hash", commitHash, "file_count", len(files))
	return &commit, nil
}

// GetCommitsBySession retrieves all commits for a session
func (cs *commitStorage) GetCommitsBySession(sessionID string) ([]*StoredCommit, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}

	cs.logger.Debug("retrieving commits by session", "session_id", sessionID)

	// Query commits
	rows, err := cs.db.Query(`
		SELECT id, session_id, repository_path, repository_name, hash, message,
			author_name, author_email, timestamp, branch, is_merge, parent_hashes,
			full_diff, diff_truncated, diff_truncated_at, correlation_type,
			created_at, updated_at
		FROM commits
		WHERE session_id = ?
		ORDER BY timestamp ASC
	`, sessionID)
	if err != nil {
		cs.logger.Error("failed to query commits", "session_id", sessionID, "error", err)
		return nil, fmt.Errorf("failed to query commits: %w", err)
	}
	defer rows.Close()

	var commits []*StoredCommit
	var skippedCount int
	for rows.Next() {
		commit, err := cs.scanCommitRow(rows)
		if err != nil {
			cs.logger.Warn("failed to scan commit row, skipping", "session_id", sessionID, "error", err)
			skippedCount++
			continue
		}

		// Query file changes for this commit
		files, err := cs.getFileDiffsByCommitID(commit.Hash)
		if err != nil {
			cs.logger.Warn("failed to get file diffs for commit, skipping", "session_id", sessionID, "hash", commit.Hash, "error", err)
			skippedCount++
			continue
		}

		commit.Files = files
		commits = append(commits, commit)
	}

	if err := rows.Err(); err != nil {
		cs.logger.Error("error iterating commits", "session_id", sessionID, "error", err)
		return nil, fmt.Errorf("error iterating commits: %w", err)
	}

	if skippedCount > 0 {
		cs.logger.Warn("retrieved commits with skipped entries", "session_id", sessionID, "successful", len(commits), "skipped", skippedCount)
	} else {
		cs.logger.Info("retrieved commits", "session_id", sessionID, "count", len(commits))
	}
	return commits, nil
}

// GetCommitsByRepository retrieves all commits for a repository
func (cs *commitStorage) GetCommitsByRepository(repoPath string) ([]*StoredCommit, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("repository path cannot be empty")
	}

	cs.logger.Debug("retrieving commits by repository", "repository_path", repoPath)

	// Query commits
	rows, err := cs.db.Query(`
		SELECT id, session_id, repository_path, repository_name, hash, message,
			author_name, author_email, timestamp, branch, is_merge, parent_hashes,
			full_diff, diff_truncated, diff_truncated_at, correlation_type,
			created_at, updated_at
		FROM commits
		WHERE repository_path = ?
		ORDER BY timestamp ASC
	`, repoPath)
	if err != nil {
		cs.logger.Error("failed to query commits", "repository_path", repoPath, "error", err)
		return nil, fmt.Errorf("failed to query commits: %w", err)
	}
	defer rows.Close()

	var commits []*StoredCommit
	var skippedCount int
	for rows.Next() {
		commit, err := cs.scanCommitRow(rows)
		if err != nil {
			cs.logger.Warn("failed to scan commit row, skipping", "repository_path", repoPath, "error", err)
			skippedCount++
			continue
		}

		// Query file changes for this commit
		files, err := cs.getFileDiffsByCommitID(commit.Hash)
		if err != nil {
			cs.logger.Warn("failed to get file diffs for commit, skipping", "repository_path", repoPath, "hash", commit.Hash, "error", err)
			skippedCount++
			continue
		}

		commit.Files = files
		commits = append(commits, commit)
	}

	if err := rows.Err(); err != nil {
		cs.logger.Error("error iterating commits", "repository_path", repoPath, "error", err)
		return nil, fmt.Errorf("error iterating commits: %w", err)
	}

	if skippedCount > 0 {
		cs.logger.Warn("retrieved commits with skipped entries", "repository_path", repoPath, "successful", len(commits), "skipped", skippedCount)
	} else {
		cs.logger.Info("retrieved commits", "repository_path", repoPath, "count", len(commits))
	}
	return commits, nil
}

// scanCommitRow scans a commit row from the database
func (cs *commitStorage) scanCommitRow(rows *sql.Rows) (*StoredCommit, error) {
	var commit StoredCommit
	var sessionIDNull, correlationTypeNull, parentHashesJSON, fullDiffNull sql.NullString
	var diffTruncatedAtNull sql.NullInt64
	var isMergeInt, diffTruncatedInt int

	err := rows.Scan(
		&commit.ID,
		&sessionIDNull,
		&commit.RepositoryPath,
		&commit.RepositoryName,
		&commit.Hash,
		&commit.Message,
		&commit.AuthorName,
		&commit.AuthorEmail,
		&commit.Timestamp,
		&commit.Branch,
		&isMergeInt,
		&parentHashesJSON,
		&fullDiffNull,
		&diffTruncatedInt,
		&diffTruncatedAtNull,
		&correlationTypeNull,
		&commit.CreatedAt,
		&commit.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Parse nullable fields
	if sessionIDNull.Valid {
		commit.SessionID = &sessionIDNull.String
	}
	if correlationTypeNull.Valid {
		commit.CorrelationType = &correlationTypeNull.String
	}
	if diffTruncatedAtNull.Valid {
		truncatedAt := int(diffTruncatedAtNull.Int64)
		commit.DiffTruncatedAt = &truncatedAt
	}
	if fullDiffNull.Valid {
		commit.FullDiff = fullDiffNull.String
	}

	commit.IsMerge = isMergeInt == 1
	commit.DiffTruncated = diffTruncatedInt == 1

	// Parse parent hashes JSON
	if parentHashesJSON.Valid && parentHashesJSON.String != "" {
		if err := json.Unmarshal([]byte(parentHashesJSON.String), &commit.ParentHashes); err != nil {
			cs.logger.Warn("failed to parse parent hashes JSON, using empty slice", "hash", commit.Hash, "error", err)
			commit.ParentHashes = []string{}
		}
	} else {
		commit.ParentHashes = []string{}
	}

	return &commit, nil
}

// getFileDiffsByCommitID retrieves all file diffs for a commit
func (cs *commitStorage) getFileDiffsByCommitID(commitID string) ([]StoredFileDiff, error) {
	rows, err := cs.db.Query(`
		SELECT id, commit_id, file_path, lines_added, lines_removed, diff, created_at
		FROM commit_files
		WHERE commit_id = ?
		ORDER BY file_path ASC
	`, commitID)
	if err != nil {
		cs.logger.Error("failed to query file diffs", "commit_id", commitID, "error", err)
		return nil, fmt.Errorf("failed to query file diffs: %w", err)
	}
	defer rows.Close()

	var files []StoredFileDiff
	var skippedCount int
	for rows.Next() {
		var file StoredFileDiff
		var diffNull sql.NullString

		err := rows.Scan(
			&file.ID,
			&file.CommitID,
			&file.FilePath,
			&file.LinesAdded,
			&file.LinesRemoved,
			&diffNull,
			&file.CreatedAt,
		)
		if err != nil {
			cs.logger.Warn("failed to scan file diff row, skipping", "commit_id", commitID, "error", err)
			skippedCount++
			continue
		}

		if diffNull.Valid {
			file.Diff = diffNull.String
		}

		files = append(files, file)
	}

	if err := rows.Err(); err != nil {
		cs.logger.Error("error iterating file diffs", "commit_id", commitID, "error", err)
		return nil, fmt.Errorf("error iterating file diffs: %w", err)
	}

	if skippedCount > 0 {
		cs.logger.Warn("retrieved file diffs with skipped entries", "commit_id", commitID, "successful", len(files), "skipped", skippedCount)
	}

	return files, nil
}
