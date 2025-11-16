package git

import "time"

// Repository represents a discovered git repository
type Repository struct {
	Path       string // Repository root path
	Name       string // Repository name (derived from directory name)
	GitDir     string // Path to .git directory or file (for worktrees)
	IsWorktree bool   // Whether this is a git worktree
}

// Commit represents a git commit with metadata
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

