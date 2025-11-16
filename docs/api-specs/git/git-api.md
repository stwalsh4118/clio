# Git API

Last Updated: 2025-11-16

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
- Component-specific logging: Uses `component=git_extractor` tag

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
- Error: `git.ErrRepositoryNotExists`
- Handling: Log warning, skip repository, continue with others

**Invalid Repository**:
- Error: Repository open fails
- Handling: Log error, skip repository, continue with others

**HEAD Not Found**:
- Error: `plumbing.ErrReferenceNotFound` (empty repository)
- Handling: Log info, skip polling, continue with others

**Commit Extraction Failure**:
- Error: Commit not found or invalid
- Handling: Log error, skip commit, continue with others

**Large Diff Truncation**:
- Not an error, but logged as info
- Diff truncated with note indicating total lines

### Error Handling Pattern

All services follow graceful degradation:
- Individual repository failures don't stop entire system
- Errors are logged with context (repository path, commit hash, etc.)
- System continues operating despite individual failures

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

- Uses `internal/logging` package for structured logging
- Component tag: `component=git_<service_name>`
- Log levels: Error, Warn, Info, Debug

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

## Notes

- All git operations use pure Go implementation (go-git)
- No external git binary required
- Supports both regular repositories and worktrees
- Polling strategy can be enhanced to watch `.git` directories if needed
- Commits are persisted to database following same pattern as conversations
- Database schema supports session correlation via foreign key

