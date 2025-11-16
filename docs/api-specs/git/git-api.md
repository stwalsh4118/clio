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
    Hash      string    // Commit hash
    Message   string    // Commit message
    Author    string    // Author name
    Email     string    // Author email
    Timestamp time.Time // Commit timestamp
    Branch    string    // Branch name
    IsMerge   bool      // Whether this is a merge commit
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

### CommitExtractor

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
  - Behavior: Extracts hash, message, author, timestamp, branch, merge status

- **ExtractDiff**: Extracts commit diff
  - Input: `repo *git.Repository` - go-git repository instance
  - Input: `hash plumbing.Hash` - Commit hash
  - Output: `*Diff` - Commit diff with file statistics
  - Output: `error` - Error if extraction fails
  - Behavior: Truncates diffs >5000 lines with note
  - Behavior: Includes file-level statistics (additions/deletions)

- **ExtractCommit**: Extracts complete commit information
  - Input: `repo *git.Repository` - go-git repository instance
  - Input: `hash plumbing.Hash` - Commit hash
  - Output: `*CommitInfo` - Complete commit information
  - Output: `error` - Error if extraction fails
  - Behavior: Combines metadata and diff extraction
  - Behavior: Single call for complete commit data

**Usage Pattern**:
```go
extractor := git.NewCommitExtractor(logger)

repo, err := git.PlainOpen(repoPath)
if err != nil {
    return err
}

hash := plumbing.NewHash("abc123...")
commitInfo, err := extractor.ExtractCommit(repo, hash)
if err != nil {
    return fmt.Errorf("failed to extract commit: %w", err)
}

// Use commitInfo.Commit for metadata
// Use commitInfo.Diff for diff content
```

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
- Commit export uses same directory structure as conversation export

### With Configuration

- Uses `config.WatchedDirectories` for repository discovery
- Uses `config.Git.PollIntervalSeconds` for polling interval
- Follows same configuration patterns as Cursor capture

### With Database

- Last seen commit hashes can be persisted to database (future enhancement)
- Commit metadata can be stored in database (future enhancement)
- Current implementation uses in-memory state

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
- State persistence to database can be added in future tasks

