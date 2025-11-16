# go-git Library Research Guide

**Date**: 2025-11-16  
**Library**: `github.com/go-git/go-git/v5`  
**Original Documentation**: https://pkg.go.dev/github.com/go-git/go-git/v5

## Overview

This document provides a practical guide for using the go-git library for git repository operations, commit monitoring, and diff extraction. It serves as a reference for implementing git commit capture functionality in the clio project.

## PRD Open Questions - Answers

Based on research and project requirements, the following decisions have been made:

1. **Commit Filtering**: Capture all commits on all branches
   - Provides complete development history
   - Filtering can be applied later if needed
   - Ensures no commits are missed during development sessions

2. **Diff Size Limits**: Truncate diffs >5000 lines with note
   - Prevents memory issues with very large commits
   - Full diff always available via git CLI
   - Note indicates truncation occurred: `[Diff truncated: X lines total, showing first 5000 lines]`

3. **Merge Commits**: Include merge commits with special marking
   - Merge commits are part of development history
   - Mark with `IsMerge: true` flag for clarity
   - Include merge commit message

4. **Commit Correlation**: 5-minute window for correlation precision
   - Balances precision with practical matching
   - Accounts for slight timing differences between git commits and Cursor conversations
   - Configurable if needed in future

5. **Repository Changes**: Periodic re-scan of watched directories, handle missing repos gracefully
   - Re-scan on configurable interval (e.g., every 5 minutes)
   - Log warnings for missing repositories
   - Continue monitoring other repositories despite individual failures

## Polling Strategy Design

### Overview

Use periodic polling (Option 3 from PRD) for simplicity and reliability. This can be enhanced to watch `.git` directories later if needed.

### Polling Interval

- **Default**: 30 seconds
- **Configurable**: Via `config.Git.PollIntervalSeconds`
- **Minimum**: 1 second (to prevent excessive polling)
- **Rationale**: Balances responsiveness with resource usage

### Commit Detection

**Strategy**: Compare HEAD commit hash with last seen commit hash per repository

1. On each poll:
   - Open repository
   - Get current HEAD commit hash
   - Compare with stored last seen hash
   - If different, fetch commits between last seen and HEAD
   - Update last seen hash

2. Initial state:
   - On first poll, store current HEAD hash as last seen
   - On subsequent polls, detect new commits

### State Management

**Storage**: In-memory map `map[string]string` (repository path → last seen commit hash)

**Thread Safety**: Use mutex for concurrent access

**Initialization**: 
- Initialize state on poller start
- For each repository, get current HEAD hash and store as last seen

**Future Enhancement**: Persist state to database for recovery across restarts

### Concurrent Handling

**Pattern**: Poll repositories in parallel using goroutines

1. Each repository gets its own goroutine
2. Poll repositories concurrently (not sequentially)
3. Use `sync.WaitGroup` to wait for all polls to complete
4. Individual repository errors don't stop entire poller

**Example Structure**:
```go
func (p *poller) pollAllRepositories(repos []Repository) {
    var wg sync.WaitGroup
    
    for _, repo := range repos {
        wg.Add(1)
        go func(r Repository) {
            defer wg.Done()
            p.pollRepository(r)
        }(repo)
    }
    
    wg.Wait()
}
```

### Error Handling

**Individual Repository Errors**:
- Log error with repository path
- Continue polling other repositories
- Don't stop entire poller

**Common Error Scenarios**:
- Repository not found: Log warning, skip repository
- Invalid repository: Log error, skip repository
- HEAD not found (empty repo): Log info, skip polling
- Commit extraction failure: Log error, skip commit

### Integration Points

- Use `config.WatchedDirectories` for repository discovery
- Follow cursor poller pattern (`internal/cursor/poller.go`) for structure
- Use `internal/logging` for structured logging
- Component tag: `component=git_poller`

## Repository Operations

### Opening Repositories

**PlainOpen** - Opens a repository from a local filesystem path:
```go
import "github.com/go-git/go-git/v5"

repo, err := git.PlainOpen("/path/to/repository")
if err != nil {
    return err
}
```

**Open** - Opens a repository with custom storage (for advanced use cases):
```go
import (
    "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/storage/filesystem"
)

// For custom storage backends
repo, err := git.Open(storage, worktree)
```

**Key Points:**
- `PlainOpen` is the standard method for opening local repositories
- Returns `*Repository` and `error`
- Works with both regular repositories and worktrees
- Does not require git to be installed (pure Go implementation)

### Checking Repository Validity

```go
// Check if path is a valid git repository
_, err := git.PlainOpen(path)
if err != nil {
    // Not a valid repository or doesn't exist
    return err
}
```

## Commit Access

### Getting HEAD Commit

```go
import (
    "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/plumbing"
)

// Get HEAD reference
ref, err := repo.Head()
if err != nil {
    return err
}

// Get commit from reference
commit, err := repo.CommitObject(ref.Hash())
if err != nil {
    return err
}

// Access commit hash
commitHash := commit.Hash.String()
```

### Iterating Commits

**Basic commit log:**
```go
import "github.com/go-git/go-git/v5/object"

// Get commit log starting from HEAD
commitIter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
if err != nil {
    return err
}
defer commitIter.Close()

// Iterate through commits
err = commitIter.ForEach(func(c *object.Commit) error {
    // Process commit
    hash := c.Hash.String()
    message := c.Message
    author := c.Author
    return nil
})
```

**Filtered commit log (since specific commit):**
```go
// Get commits since a specific hash
sinceHash := plumbing.NewHash("abc123...")
commitIter, err := repo.Log(&git.LogOptions{From: sinceHash})
```

**Limit number of commits:**
```go
commitIter, err := repo.Log(&git.LogOptions{
    From:  ref.Hash(),
    Order: git.LogOrderCommitterTime,
})
// Then limit in iteration
count := 0
err = commitIter.ForEach(func(c *object.Commit) error {
    if count >= 100 {
        return nil // Stop iteration
    }
    count++
    // Process commit
    return nil
})
```

### Commit Metadata

```go
type Commit struct {
    Hash      plumbing.Hash    // Commit hash
    Message   string           // Commit message
    Author    Signature        // Author name, email, time
    Committer Signature        // Committer name, email, time
    TreeHash  plumbing.Hash    // Tree hash
    Parents   []plumbing.Hash  // Parent commit hashes
}

// Access commit metadata
commit, err := repo.CommitObject(hash)
if err != nil {
    return err
}

hash := commit.Hash.String()
message := commit.Message
authorName := commit.Author.Name
authorEmail := commit.Author.Email
authorTime := commit.Author.When
committerName := commit.Committer.Name
committerTime := commit.Committer.When

// Check if merge commit
isMerge := len(commit.Parents) > 1
```

### Detecting New Commits

**Strategy: Compare HEAD hash with last seen hash**
```go
// Get current HEAD hash
ref, err := repo.Head()
if err != nil {
    return err
}
currentHash := ref.Hash().String()

// Compare with last seen hash
if currentHash != lastSeenHash {
    // New commits detected
    // Get commits between lastSeenHash and currentHash
    commits, err := getCommitsBetween(repo, lastSeenHash, currentHash)
}
```

**Getting commits between two hashes:**
```go
func getCommitsBetween(repo *git.Repository, fromHash, toHash string) ([]*object.Commit, error) {
    from := plumbing.NewHash(fromHash)
    to := plumbing.NewHash(toHash)
    
    commitIter, err := repo.Log(&git.LogOptions{From: to})
    if err != nil {
        return nil, err
    }
    defer commitIter.Close()
    
    var commits []*object.Commit
    err = commitIter.ForEach(func(c *object.Commit) error {
        if c.Hash == from {
            return nil // Stop at from hash
        }
        commits = append(commits, c)
        return nil
    })
    
    return commits, err
}
```

## Branch Information

### Getting Current Branch

```go
ref, err := repo.Head()
if err != nil {
    return err
}

branchName := ref.Name().Short() // e.g., "main", "feature-branch"
fullRefName := ref.Name().String() // e.g., "refs/heads/main"
```

### Listing All Branches

```go
branches, err := repo.Branches()
if err != nil {
    return err
}

err = branches.ForEach(func(ref *plumbing.Reference) error {
    branchName := ref.Name().Short()
    // Process branch
    return nil
})
```

## Diff Extraction

### Getting Commit Diff

**Full commit patch:**
```go
import "github.com/go-git/go-git/v5/object"

commit, err := repo.CommitObject(hash)
if err != nil {
    return err
}

// Get parent commit (first parent for merge commits)
var parent *object.Commit
if len(commit.Parents) > 0 {
    parent, err = commit.Parent(0)
    if err != nil {
        return err
    }
}

// Generate patch
var patch *object.Patch
if parent != nil {
    patch, err = parent.Patch(commit)
} else {
    // Initial commit - compare with empty tree
    patch, err = commit.Patch()
}

if err != nil {
    return err
}

// Get diff as string
diff := patch.String()

// Get file-level statistics
for _, filePatch := range patch.FilePatches() {
    from, to := filePatch.Files()
    // from and to can be nil for new/deleted files
    
    // Get statistics
    stats := filePatch.Stats()
    additions := stats.Addition
    deletions := stats.Deletion
}
```

**File-level diffs:**
```go
patch, err := parent.Patch(commit)
if err != nil {
    return err
}

for _, filePatch := range patch.FilePatches() {
    from, to := filePatch.Files()
    
    var fileName string
    if to != nil {
        fileName = to.Path()
    } else if from != nil {
        fileName = from.Path()
    }
    
    // Get chunks (hunks) for this file
    for _, chunk := range filePatch.Chunks() {
        content := chunk.Content()
        op := chunk.Type() // Add, Delete, Equal
    }
}
```

### Handling Large Diffs

**Truncation strategy:**
```go
const MaxDiffLines = 5000

diff := patch.String()
lines := strings.Split(diff, "\n")

if len(lines) > MaxDiffLines {
    truncated := strings.Join(lines[:MaxDiffLines], "\n")
    truncated += fmt.Sprintf("\n\n[Diff truncated: %d lines total, showing first %d lines]", 
        len(lines), MaxDiffLines)
    diff = truncated
}
```

## Worktree Support

### Detecting Worktrees

```go
import "os"

gitDir := filepath.Join(repoPath, ".git")
gitInfo, err := os.Stat(gitDir)
if err != nil {
    return err
}

isWorktree := !gitInfo.IsDir() // Worktrees have .git as a file
```

### Opening Worktrees

```go
// PlainOpen works with worktrees automatically
repo, err := git.PlainOpen(worktreePath)
if err != nil {
    return err
}

// Access worktree if needed
worktree, err := repo.Worktree()
if err != nil {
    return err
}
```

## Performance Considerations

### Large Repositories

**Efficient commit iteration:**
- Use `LogOptions` to limit scope
- Process commits incrementally rather than loading all at once
- Close iterators properly with `defer commitIter.Close()`

**Memory management:**
- Don't store all commits in memory
- Process commits one at a time in iteration
- Use streaming patterns for large diffs

**Example efficient processing:**
```go
commitIter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
if err != nil {
    return err
}
defer commitIter.Close()

count := 0
err = commitIter.ForEach(func(c *object.Commit) error {
    // Process one commit at a time
    processCommit(c)
    
    count++
    if count >= maxCommits {
        return nil // Stop early if needed
    }
    return nil
})
```

### Concurrent Repository Access

**Thread safety:**
- Each repository instance should be opened per goroutine
- Don't share repository instances across goroutines
- Use separate repository instances for concurrent polling

**Example concurrent polling:**
```go
func pollRepositories(repos []string) {
    var wg sync.WaitGroup
    
    for _, repoPath := range repos {
        wg.Add(1)
        go func(path string) {
            defer wg.Done()
            
            // Open repository in goroutine
            repo, err := git.PlainOpen(path)
            if err != nil {
                log.Error("failed to open repo", "path", path, "error", err)
                return
            }
            
            // Poll this repository
            pollRepository(repo, path)
        }(repoPath)
    }
    
    wg.Wait()
}
```

## Error Handling

### Common Errors

**Repository not found:**
```go
repo, err := git.PlainOpen(path)
if err == git.ErrRepositoryNotExists {
    // Repository doesn't exist
    return fmt.Errorf("repository not found: %s", path)
}
```

**Invalid repository:**
```go
repo, err := git.PlainOpen(path)
if err != nil {
    // Could be corrupted, invalid format, etc.
    return fmt.Errorf("invalid repository: %w", err)
}
```

**Reference not found:**
```go
ref, err := repo.Head()
if err == plumbing.ErrReferenceNotFound {
    // Repository has no HEAD (empty repo)
    return nil // Handle gracefully
}
```

## Best Practices

1. **Always close iterators**: Use `defer commitIter.Close()` to ensure cleanup
2. **Handle empty repositories**: Check for `plumbing.ErrReferenceNotFound` when accessing HEAD
3. **Validate hashes**: Check hash validity before using in operations
4. **Error wrapping**: Wrap errors with context for better debugging
5. **Resource cleanup**: Close iterators and repositories when done
6. **Concurrent access**: Open separate repository instances per goroutine
7. **Large diff handling**: Truncate or stream large diffs to avoid memory issues
8. **Worktree detection**: Check if `.git` is file or directory to detect worktrees

## Example: Complete Commit Extraction

```go
func extractCommit(repo *git.Repository, hash plumbing.Hash) (*CommitInfo, error) {
    commit, err := repo.CommitObject(hash)
    if err != nil {
        return nil, fmt.Errorf("failed to get commit: %w", err)
    }
    
    // Get branch
    ref, err := repo.Head()
    if err != nil {
        return nil, fmt.Errorf("failed to get HEAD: %w", err)
    }
    branch := ref.Name().Short()
    
    // Get diff
    var patch *object.Patch
    if len(commit.Parents) > 0 {
        parent, err := commit.Parent(0)
        if err != nil {
            return nil, fmt.Errorf("failed to get parent: %w", err)
        }
        patch, err = parent.Patch(commit)
    } else {
        patch, err = commit.Patch()
    }
    if err != nil {
        return nil, fmt.Errorf("failed to get patch: %w", err)
    }
    
    // Extract file statistics
    var files []FileChange
    for _, filePatch := range patch.FilePatches() {
        from, to := filePatch.Files()
        var fileName string
        if to != nil {
            fileName = to.Path()
        } else if from != nil {
            fileName = from.Path()
        }
        
        stats := filePatch.Stats()
        files = append(files, FileChange{
            Path:      fileName,
            Additions: stats.Addition,
            Deletions: stats.Deletion,
        })
    }
    
    return &CommitInfo{
        Hash:      commit.Hash.String(),
        Message:   commit.Message,
        Author:    commit.Author.Name,
        Email:     commit.Author.Email,
        Timestamp: commit.Author.When,
        Branch:    branch,
        IsMerge:   len(commit.Parents) > 1,
        Files:     files,
        Diff:      patch.String(),
    }, nil
}
```

## References

- **Official Documentation**: https://pkg.go.dev/github.com/go-git/go-git/v5
- **Repository**: https://github.com/go-git/go-git
- **Examples**: https://github.com/go-git/go-git/tree/master/_examples

