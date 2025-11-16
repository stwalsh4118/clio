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

// CommitDiff represents a commit diff with file-level changes
type CommitDiff struct {
	CommitHash  string    // Commit hash this diff belongs to
	FullDiff    string    // Full commit diff (may be truncated)
	Files       []FileDiff // File-level diffs
	IsTruncated bool      // Whether diff was truncated
	TruncatedAt int       // Line count where truncated (if applicable)
}

// FileDiff represents file-level diff information
type FileDiff struct {
	Path        string // File path relative to repository root
	LinesAdded  int    // Lines added
	LinesRemoved int   // Lines removed
	Diff        string // File-level diff content
}

// CommitSessionCorrelation represents correlation between a commit and a session
type CommitSessionCorrelation struct {
	CommitHash      string        // Commit hash
	SessionID       string        // Session ID (may be empty if no correlation)
	Project         string        // Project name
	CorrelationType string        // "active", "proximate", or "none"
	TimeDiff        time.Duration // Time difference to nearest conversation
}

