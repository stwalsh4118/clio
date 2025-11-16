# Git API

Last Updated: 2025-01-19

## Overview

This document specifies the API for git repository discovery, commit monitoring, and diff extraction functionality. This API enables automatic capture of git commit activity from configured repositories.

## Design Decisions

### PRD Open Questions - Answers

1. **Git Strategy**: Use periodic polling (Option 3 from PRD)
   - Simple and reliable
   - No modification of repositories needed
   - Can be enhanced to watch `.git` directories later if needed

2. **Commit Filtering**: Capture all commits on all branches
   - Provides complete history
   - Filtering can be applied later if needed
   - Ensures no commits are missed

3. **Diff Size Limits**: Truncate diffs >5000 lines with note
   - Prevents memory issues with very large commits
   - Full diff always available via git CLI
   - Note indicates truncation occurred

4. **Merge Commits**: Include merge commits with special marking
   - Merge commits are part of development history
   - Mark as merge commits for clarity
   - Include merge commit message

5. **Commit Correlation**: 5-minute window for correlation precision
   - Balances precision with practical matching
   - Accounts for slight timing differences
   - Configurable if needed

6. **Repository Changes**: Periodic re-scan of watched directories, handle missing repos gracefully
   - Re-scan on configurable interval (e.g., every 5 minutes)
   - Log warnings for missing repositories
   - Continue monitoring other repositories

### Polling Strategy

**Interval**: Default 30 seconds, configurable via `config.Git.PollIntervalSeconds`

**Commit Detection**:
- Compare current HEAD commit hash with last seen commit hash per repository
- If different, fetch commits between last seen and HEAD
- Store last seen commit hash in memory (can be persisted to database later)

**Concurrent Handling**:
- Poll repositories in parallel using goroutines
- Each repository gets its own goroutine
- Individual repository errors don't stop entire poller

**State Management**:
- In-memory map: `map[string]string` (repository path → last seen commit hash)
- Thread-safe access using mutex
- Initialize state on poller start

## Data Structures

### Repository

```go
type Repository struct {
    Path       string // Repository root path
    Name       string // Repository name (derived from directory name)
    GitDir     string // Path to .git directory or file (for worktrees)
    IsWorktree bool   // Whether this is a git worktree
}
```

### Commit

```go
type Commit struct {
    Hash      string    // Commit hash (full SHA-1)
    Message   string    // Commit message
    Author    string    // Author name
    Email     string    // Author email
    Timestamp time.Time // Commit timestamp
    Branch    string    // Branch name (e.g., "main", "feature-branch")
    IsMerge   bool      // Whether this is a merge commit
    Parents   []string  // Parent commit hashes
}
```

### CommitMetadata

```go
type CommitMetadata struct {
    Hash         string      // Commit hash (full SHA-1)
    Message      string      // Commit message (including multi-line)
    Timestamp    time.Time   // Commit timestamp (author time)
    Author       AuthorInfo  // Author information
    Branch       string      // Branch name (or "detached" if in detached HEAD state)
    IsMerge      bool        // Whether this is a merge commit
    ParentHashes []string    // Parent commit hashes
}

type AuthorInfo struct {
    Name  string // Author name
    Email string // Author email
}
```

### FileChange

```go
type FileChange struct {
    Path      string // File path relative to repository root
    Additions int    // Number of lines added
    Deletions int    // Number of lines deleted
}
```

### Diff

```go
type Diff struct {
    Content string       // Full diff content (may be truncated)
    Files   []FileChange // File-level statistics
    Truncated bool      // Whether diff was truncated due to size
    TotalLines int       // Total lines in diff (if truncated)
    ShownLines  int     // Lines shown (if truncated)
}
```

### CommitInfo

```go
type CommitInfo struct {
    Commit   CommitMetadata // Commit metadata
    Diff     Diff           // Commit diff
}
```

## Service Interfaces

### DiscoveryService

**Package**: `github.com/stwalsh4118/clio/internal/git`

**Interface**:
```go
type DiscoveryService interface {
    DiscoverRepositories(dirs []string) ([]Repository, error)
    FindGitRepositories(dir string) ([]Repository, error)
}
```

**Methods**:

- **DiscoverRepositories**: Scans multiple watched directories for git repositories
  - Input: `dirs []string` - List of watched directory paths
  - Output: `[]Repository` - List of discovered repositories
  - Output: `error` - Error if discovery fails
  - Behavior: Recursively scans directories, skips `.git` directories during traversal
  - Behavior: Detects both regular repositories and worktrees

- **FindGitRepositories**: Scans a single directory for git repositories
  - Input: `dir string` - Directory path to scan
  - Output: `[]Repository` - List of repositories found in this directory
  - Output: `error` - Error if scan fails
  - Behavior: Recursive scan, handles nested repositories (submodules)

**Usage Pattern**:
```go
discovery := git.NewDiscoveryService(logger)
repos, err := discovery.DiscoverRepositories(cfg.WatchedDirectories)
if err != nil {
    return fmt.Errorf("failed to discover repositories: %w", err)
}
```

**Implementation Notes**:
- Uses `filepath.WalkDir` for efficient recursive directory traversal
- Skips `.git` directories during traversal to prevent scanning into git internals
- Detects worktrees by checking if `.git` is a file (contains `gitdir: <path>`)
- Handles symlinks by resolving paths before processing
- Deduplicates repositories found in overlapping watched directories
- Gracefully handles inaccessible directories (logs warning, continues scanning)
- Pure Go implementation - no external git binary or go-git library required for discovery

### PollerService

**Status**: ✅ Implemented (Task 3-3)

**Package**: `github.com/stwalsh4118/clio/internal/git`

**Interface**:
```go
type PollerService interface {
    Start(ctx context.Context, repos []Repository) error
    Stop() error
    PollResults() <-chan PollResult
}

type PollResult struct {
    Repository Repository
    NewCommits []Commit
    Error      error
}
```

**Methods**:

- **Start**: Starts periodic polling of repositories
  - Input: `ctx context.Context` - Context for cancellation
  - Input: `repos []Repository` - List of repositories to monitor
  - Output: `error` - Error if poller fails to start
  - Behavior: Starts background goroutine that polls at configured interval
  - Behavior: Initializes last seen commit hash for each repository
  - Behavior: Polls repositories concurrently

- **Stop**: Stops the poller
  - Output: `error` - Error if stop fails
  - Behavior: Cancels polling, waits for in-flight operations
  - Behavior: Graceful shutdown

- **PollResults**: Returns channel of poll results
  - Output: `<-chan PollResult` - Channel that receives poll results
  - Behavior: Channel receives results when new commits are detected
  - Behavior: Channel receives errors for individual repository failures
  - Behavior: Channel closed when poller stops

**Usage Pattern**:
```go
poller := git.NewPollerService(cfg, logger)
if err := poller.Start(ctx, repos); err != nil {
    return fmt.Errorf("failed to start poller: %w", err)
}
defer poller.Stop()

results := poller.PollResults()
for result := range results {
    if result.Error != nil {
        logger.Warn("poll error", "repo", result.Repository.Path, "error", result.Error)
        continue
    }
    
    for _, commit := range result.NewCommits {
        processCommit(commit)
    }
}
```

**Implementation Notes**:
- Uses `github.com/go-git/go-git/v5` for git operations
- Polls repositories concurrently using goroutines
- Thread-safe state management with mutex for last seen commit hashes
- Handles empty repositories gracefully (no HEAD)
- Individual repository errors don't stop the poller
- Uses buffered channel (size 10) for poll results to prevent blocking
- Follows cursor poller pattern for lifecycle management
- Configurable polling interval (default: 30 seconds, minimum: 1 second)
- Initializes last seen hash on start for each repository
- Collects commits between last seen hash and current HEAD
- Stops iteration when reaching the last seen hash using sentinel error
- Retry logic: Transient errors retried up to 3 times with exponential backoff (50ms, 100ms, 200ms)
- Error handling: Repository open failures, commit access failures handled with retries
- Logging: Comprehensive logging with repository context, retry attempts, and error details

### CommitExtractor

**Status**: ✅ Implemented (Task 3-4, 3-5)
- `ExtractMetadata`: ✅ Implemented
- `ExtractDiff`: ✅ Implemented
- `ExtractCommit`: ✅ Implemented

**Package**: `github.com/stwalsh4118/clio/internal/git`

**Interface**:
```go
type CommitExtractor interface {
    ExtractMetadata(repo *git.Repository, hash plumbing.Hash) (*CommitMetadata, error)
    ExtractDiff(repo *git.Repository, hash plumbing.Hash) (*Diff, error)
    ExtractCommit(repo *git.Repository, hash plumbing.Hash) (*CommitInfo, error)
}
```

**Methods**:

- **ExtractMetadata**: Extracts commit metadata without diff
  - Input: `repo *git.Repository` - go-git repository instance
  - Input: `hash plumbing.Hash` - Commit hash
  - Output: `*CommitMetadata` - Commit metadata
  - Output: `error` - Error if extraction fails
  - Behavior: Extracts hash, message, author, timestamp, branch, merge status, parent hashes
  - Behavior: Handles detached HEAD state (returns "detached" as branch name)
  - Behavior: Detects merge commits (len(ParentHashes) > 1)
  - Behavior: Handles initial commits (no parent hashes)

- **ExtractDiff**: Extracts commit diff
  - Input: `repo *git.Repository` - go-git repository instance
  - Input: `hash plumbing.Hash` - Commit hash
  - Output: `*Diff` - Commit diff with file statistics
  - Output: `error` - Error if extraction fails
  - Behavior: Truncates diffs >5000 lines with note (preserves file statistics)
  - Behavior: Includes file-level statistics (additions/deletions)
  - Behavior: Handles initial commits (no parent) by comparing with empty tree
  - Behavior: Uses first parent for merge commits (standard git behavior)
  - Behavior: Counts additions/deletions from patch chunks
  - Implementation: Uses `object.DiffTree` for initial commits, `parent.Patch(commit)` for normal commits

- **ExtractCommit**: Extracts complete commit information
  - Input: `repo *git.Repository` - go-git repository instance
  - Input: `hash plumbing.Hash` - Commit hash
  - Output: `*CommitInfo` - Complete commit information
  - Output: `error` - Error if extraction fails
  - Behavior: Combines metadata and diff extraction
  - Behavior: Single call for complete commit data

**Usage Pattern**:
```go
extractor, err := git.NewCommitExtractor(logger)
if err != nil {
    return fmt.Errorf("failed to create extractor: %w", err)
}

repo, err := git.PlainOpen(repoPath)
if err != nil {
    return err
}

hash := plumbing.NewHash("abc123...")
metadata, err := extractor.ExtractMetadata(repo, hash)
if err != nil {
    return fmt.Errorf("failed to extract metadata: %w", err)
}

// Use metadata.Hash, metadata.Message, metadata.Author, etc.
```

**Implementation Notes**:
- Uses `github.com/go-git/go-git/v5` for git operations
- Branch detection: Checks HEAD reference to determine branch name
- Detached HEAD: Returns "detached" as branch name when HEAD is not on a branch
- Merge commit detection: Checks if commit has more than one parent hash
- Parent hash extraction: Iterates through commit parents to collect all parent hashes
- Error handling: Returns descriptive errors for invalid commits, nil repository, etc.
- Retry logic: Commit object retrieval retried up to 3 times for transient errors
- Component-specific logging: Uses `component=git_extractor` tag
- Logging: Detailed logging for extraction operations, diff generation, file processing
- Graceful degradation: Branch detection failures don't stop extraction, uses "unknown" fallback

### CommitStorage

**Status**: ✅ Implemented (Task 3-7)

**Package**: `github.com/stwalsh4118/clio/internal/git`

**Interface**:
```go
type CommitStorage interface {
    StoreCommit(commit *Commit, diff *CommitDiff, correlation *CommitSessionCorrelation, repository *Repository, sessionID string) error
    GetCommit(commitHash string) (*StoredCommit, error)
    GetCommitsBySession(sessionID string) ([]*StoredCommit, error)
    GetCommitsByRepository(repoPath string) ([]*StoredCommit, error)
}
```

**Methods**:

- **StoreCommit**: Stores a commit and all its file changes in a single transaction
  - Input: `commit *Commit` - Commit metadata
  - Input: `diff *CommitDiff` - Commit diff with file changes
  - Input: `correlation *CommitSessionCorrelation` - Session correlation info
  - Input: `repository *Repository` - Repository information
  - Input: `sessionID string` - Session ID (may be empty)
  - Output: `error` - Error if storage fails
  - Behavior: Verifies session exists if sessionID provided
  - Behavior: Stores commit and file changes in single transaction
  - Behavior: Handles duplicate commits with ON CONFLICT
  - Behavior: Uses commit hash as primary key

- **GetCommit**: Retrieves a commit by its hash
  - Input: `commitHash string` - Commit hash (full SHA-1)
  - Output: `*StoredCommit` - Commit with all file changes
  - Output: `error` - Error if retrieval fails
  - Behavior: Returns commit with all file changes loaded

- **GetCommitsBySession**: Retrieves all commits for a session
  - Input: `sessionID string` - Session ID
  - Output: `[]*StoredCommit` - List of commits for session
  - Output: `error` - Error if retrieval fails
  - Behavior: Returns commits ordered by timestamp

- **GetCommitsByRepository**: Retrieves all commits for a repository
  - Input: `repoPath string` - Repository path
  - Output: `[]*StoredCommit` - List of commits for repository
  - Output: `error` - Error if retrieval fails
  - Behavior: Returns commits ordered by timestamp

**Usage Pattern**:
```go
storage := git.NewCommitStorage(db, logger)

err := storage.StoreCommit(
    commit,
    diff,
    correlation,
    repository,
    sessionID,
)
if err != nil {
    return fmt.Errorf("failed to store commit: %w", err)
}

// Retrieve commit
storedCommit, err := storage.GetCommit(commitHash)
if err != nil {
    return fmt.Errorf("failed to get commit: %w", err)
}

// Retrieve commits by session
commits, err := storage.GetCommitsBySession(sessionID)
if err != nil {
    return fmt.Errorf("failed to get commits: %w", err)
}
```

**Implementation Notes**:
- Follows same transaction pattern as `ConversationStorage`
- Uses commit hash as primary key (like composer_id for conversations)
- Stores parent hashes as JSON array in TEXT field
- Handles large diffs by storing truncated version with flag
- Foreign key relationship to `sessions` table (nullable)
- File changes stored in separate `commit_files` table with CASCADE delete
- All operations use transactions for atomicity
- Error handling: Comprehensive error handling with transaction rollback on failure
- Logging: Detailed logging for transaction operations, file diff storage, commit retrieval
- Graceful degradation: Individual row scan failures log warnings and continue processing

## Database Schema

### commits table

- `id` (TEXT PRIMARY KEY) - Commit hash (full SHA-1)
- `session_id` (TEXT, FOREIGN KEY to sessions) - Correlated session (nullable)
- `repository_path` (TEXT) - Repository root path
- `repository_name` (TEXT) - Repository name
- `hash` (TEXT) - Commit hash (duplicate of id for query convenience)
- `message` (TEXT) - Commit message
- `author_name` (TEXT) - Author name
- `author_email` (TEXT) - Author email
- `timestamp` (TIMESTAMP) - Commit timestamp
- `branch` (TEXT) - Branch name
- `is_merge` (INTEGER) - Merge commit flag (0 or 1)
- `parent_hashes` (TEXT) - JSON array of parent commit hashes (nullable)
- `full_diff` (TEXT) - Full commit diff (nullable, may be truncated)
- `diff_truncated` (INTEGER) - Whether diff was truncated (0 or 1)
- `diff_truncated_at` (INTEGER) - Line count where truncated (nullable)
- `correlation_type` (TEXT) - "active", "proximate", or "none" (nullable)
- `created_at` (TIMESTAMP) - When record was created
- `updated_at` (TIMESTAMP) - When record was updated

**Indexes**:
- `idx_commits_session_id` on `commits(session_id)`
- `idx_commits_timestamp` on `commits(timestamp)`
- `idx_commits_repository_path` on `commits(repository_path)`
- `idx_commits_hash` on `commits(hash)`

### commit_files table

- `id` (TEXT PRIMARY KEY) - UUID for file diff
- `commit_id` (TEXT, FOREIGN KEY to commits) - Parent commit hash
- `file_path` (TEXT) - File path relative to repository root
- `lines_added` (INTEGER) - Lines added
- `lines_removed` (INTEGER) - Lines removed
- `diff` (TEXT) - File-level diff content (nullable)
- `created_at` (TIMESTAMP) - When record was created

**Constraints**:
- `UNIQUE (commit_id, file_path)` - One row per file per commit

**Indexes**:
- `idx_commit_files_commit_id` on `commit_files(commit_id)`
- `idx_commit_files_file_path` on `commit_files(file_path)`

## Configuration

**Git Configuration**:
```go
type GitConfig struct {
    PollIntervalSeconds int `mapstructure:"poll_interval_seconds" yaml:"poll_interval_seconds"`
}
```

**Default Values**:
- `PollIntervalSeconds`: 30 seconds

**Configuration Location**: `config.Git.PollIntervalSeconds`

**Example Configuration**:
```yaml
git:
  poll_interval_seconds: 30  # Polling interval in seconds (default: 30, minimum: 1)
```

## Error Handling

### Common Errors

**Repository Not Found**:
- Error: Repository path doesn't exist or is inaccessible
- Handling: Log warning with repository path, skip repository, continue with others
- Log Level: Warn

**Invalid/Corrupted Repository**:
- Error: Repository validation fails (git.PlainOpen fails)
- Handling: Log warning with repository path and error, skip repository, continue with others
- Log Level: Warn
- Validation: Repository is validated during discovery using `git.PlainOpen()`

**Permission Errors**:
- Error: `os.IsPermission(err)` - read access denied
- Handling: Log warning with path and error, skip directory/repository, continue with others
- Log Level: Warn

**HEAD Not Found**:
- Error: `plumbing.ErrReferenceNotFound` (empty repository)
- Handling: Log debug, skip polling for this repository, continue with others
- Log Level: Debug (not an error condition)

**Commit Extraction Failure**:
- Error: Commit not found, invalid hash, or repository access error
- Handling: Log error with commit hash and error details, skip commit, continue with others
- Log Level: Error
- Retry Logic: Transient errors are retried up to 3 times with exponential backoff

**Diff Generation Failure**:
- Error: Patch generation fails, tree access fails
- Handling: Log error with commit hash and error details, return error
- Log Level: Error

**Large Diff Truncation**:
- Not an error, but logged as info
- Diff truncated with note indicating total lines
- Log Level: Info

**Database Errors**:
- Error: Query failures, transaction failures, connection errors
- Handling: Log error with context (session ID, commit hash, etc.), return error
- Log Level: Error
- Graceful Degradation: Individual row scan failures log warnings and continue

**Transient Errors**:
- Error: File locks, temporary I/O errors, database locks
- Handling: Retry with exponential backoff (50ms, 100ms, 200ms), maximum 3 retries
- Log Level: Warn for retry attempts, Error if all retries fail

### Error Handling Pattern

All services follow graceful degradation:
- Individual repository failures don't stop entire system
- Errors are logged with context (repository path, commit hash, session ID, etc.)
- System continues operating despite individual failures
- Retry logic for transient errors (file locks, temporary I/O)
- Maximum retry attempts: 3
- Exponential backoff: 50ms → 100ms → 200ms

### Retry Logic

**Implementation**:
- Retry logic implemented in `poller.go` and `extractor.go`
- Constants defined in `types.go`: `maxRetries = 3`, `initialRetryDelay = 50ms`
- Transient error detection via `isTransientError()` method
- Only retries errors matching transient patterns: "locked", "busy", "temporary", "timeout", "connection", "network"

**Where Applied**:
- Repository open operations (`getCurrentHEADHash`, `getCommitsBetween`)
- Commit object retrieval (`ExtractMetadata`, `ExtractDiff`)
- Commit log iteration
- HEAD reference access

**Not Applied To**:
- Permanent failures (missing repositories, invalid commit hashes)
- Validation errors (nil repository, empty commit hash)
- User errors (invalid configuration)

## Integration Points

### With Cursor Capture

- Commit correlation uses session timestamps from `internal/cursor/session.go`
- Commits are correlated with active sessions using 5-minute window
- Commits stored in database with foreign key relationship to sessions table

### With Configuration

- Uses `config.WatchedDirectories` for repository discovery
- Uses `config.Git.PollIntervalSeconds` for polling interval
- Follows same configuration patterns as Cursor capture

### With Database

- Commits are persisted to database via `CommitStorage` interface
- Commit metadata, diffs, and file changes stored in `commits` and `commit_files` tables
- Foreign key relationship to `sessions` table for correlation
- Last seen commit hashes stored in-memory (can be persisted in future enhancement)

### With Logging

- Uses `internal/logging` package for structured logging (zerolog-based)
- Component tags: `component=git_discovery`, `component=git_poller`, `component=git_extractor`, `component=git_correlation`, `component=commit_storage`
- Log levels: Error (failures), Warn (recoverable issues), Info (important events), Debug (detailed operations)

**Logging Patterns** (following PBI 2 Cursor capture patterns):

**Discovery Service** (`component=git_discovery`):
- Debug: Directory scanning progress, repository detection details, duplicate detection
- Info: Repository discovered (with path, name, type), discovery completed (with counts)
- Warn: Skipped directories (with reason), invalid/corrupted repositories, permission errors
- Error: Critical discovery failures

**Poller Service** (`component=git_poller`):
- Debug: Polling operations, commit detection, state updates, retry attempts
- Info: New commits detected (with count), polling started/stopped, state initialization completed
- Warn: Repository polling errors (with context), retry attempts, channel full
- Error: Critical polling failures, repository open failures after retries

**Extractor Service** (`component=git_extractor`):
- Debug: Commit extraction details, diff generation, file processing, retry attempts
- Info: Commit extracted (with hash), diff truncated (with line counts)
- Warn: Extraction failures (with commit hash), skipped files, branch detection failures, retry attempts
- Error: Critical extraction failures, commit object retrieval failures after retries

**Correlation Service** (`component=git_correlation`):
- Debug: Correlation matching details, project matching, timestamp comparisons, database queries
- Info: Commit correlated (with session ID and type), correlation completed
- Warn: Correlation failures (with context), missing sessions, database query failures
- Error: Critical correlation failures, database errors

**Storage Service** (`component=commit_storage`):
- Debug: Transaction operations, file diff storage, commit retrieval details
- Info: Commit stored (with hash, session ID, file count), commits retrieved (with counts)
- Warn: Storage warnings (skipped rows, parse failures)
- Error: Storage errors (transaction failures, database errors)

**Structured Fields**:
- Consistent field names: `repository`, `commit`, `hash`, `session_id`, `file_count`, `error`, `attempt`, `delay_ms`
- Context always included: repository path, commit hash, session ID, file paths
- Operation counts and statistics logged for monitoring

## Performance Considerations

### Large Repositories

- Commit iteration processes commits one at a time
- Diffs are truncated at 5000 lines to prevent memory issues
- Iterators are properly closed to free resources

### Concurrent Access

- Each repository gets its own goroutine for polling
- Repository instances are not shared across goroutines
- Thread-safe state management using mutexes

### Polling Efficiency

- Only compares HEAD hash, doesn't scan entire history
- Fetches commits incrementally between last seen and HEAD
- Configurable polling interval balances responsiveness and resource usage

### CorrelationService

**Status**: ✅ Implemented (Task 3-6)

**Package**: `github.com/stwalsh4118/clio/internal/git`

**Interface**:
```go
type CorrelationService interface {
    CorrelateCommit(commit CommitMetadata, repository Repository, sessionManager cursor.SessionManager) (*CommitSessionCorrelation, error)
    CorrelateCommits(commits []CommitMetadata, repository Repository, sessionManager cursor.SessionManager) ([]CommitSessionCorrelation, error)
    GroupCommitsBySession(correlations []CommitSessionCorrelation) (map[string][]CommitSessionCorrelation, error)
}
```

**Methods**:

- **CorrelateCommit**: Correlates a single commit with sessions
  - Input: `commit CommitMetadata` - Commit metadata to correlate
  - Input: `repository Repository` - Repository information
  - Input: `sessionManager cursor.SessionManager` - Session manager for accessing sessions
  - Output: `*CommitSessionCorrelation` - Correlation result
  - Output: `error` - Error if correlation fails
  - Behavior: Matches commit to sessions by project name and timestamp proximity
  - Behavior: Determines correlation type: "active", "proximate", or "none"
  - Behavior: Calculates time difference to nearest conversation message

- **CorrelateCommits**: Correlates multiple commits with sessions
  - Input: `commits []CommitMetadata` - Commits to correlate
  - Input: `repository Repository` - Repository information
  - Input: `sessionManager cursor.SessionManager` - Session manager
  - Output: `[]CommitSessionCorrelation` - Correlation results
  - Output: `error` - Error if correlation fails
  - Behavior: Correlates each commit individually
  - Behavior: Continues processing even if individual commits fail

- **GroupCommitsBySession**: Groups correlated commits by session ID
  - Input: `correlations []CommitSessionCorrelation` - Correlation results
  - Output: `map[string][]CommitSessionCorrelation` - Commits grouped by session ID
  - Output: `error` - Error if grouping fails
  - Behavior: Groups commits with same session ID together
  - Behavior: Commits with no correlation are grouped under empty string key

**Usage Pattern**:
```go
correlationService, err := git.NewCorrelationService(logger, database)
if err != nil {
    return fmt.Errorf("failed to create correlation service: %w", err)
}

correlation, err := correlationService.CorrelateCommit(commit, repository, sessionManager)
if err != nil {
    return fmt.Errorf("failed to correlate commit: %w", err)
}

// Use correlation.SessionID, correlation.CorrelationType, etc.
```

**Correlation Logic**:

1. **Project Matching**: Normalizes repository path to project name and matches against session project names
2. **Timestamp Correlation**: Checks if commit timestamp is within 5-minute window of any conversation message
3. **Correlation Types**:
   - **"active"**: Commit timestamp falls within session time window AND within 5 minutes of conversation message
   - **"proximate"**: Commit timestamp is within 5 minutes of conversation message but NOT during active session window
   - **"none"**: No correlation found
4. **Best Match Selection**: Prefers "active" over "proximate" over "none", and closer timestamps for same type

**Implementation Notes**:
- Uses 5-minute correlation window (configurable via `correlationWindow` constant)
- Normalizes project names using same logic as `cursor.ProjectDetector.NormalizeProjectName()`
- Loads all sessions (active + ended) from database for correlation
- Loads conversations and messages for each session to check timestamp proximity
- Handles edge cases: commits before/after sessions, overlapping sessions, no matching projects
- Gracefully handles missing conversations table (returns empty slice)

## Notes

- All git operations use pure Go implementation (go-git)
- No external git binary required
- Supports both regular repositories and worktrees
- Polling strategy can be enhanced to watch `.git` directories if needed
- Commits are persisted to database following same pattern as conversations
- Database schema supports session correlation via foreign key
- Commit-to-session correlation uses 5-minute window for timestamp matching

