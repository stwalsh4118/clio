package git

// Repository represents a discovered git repository
type Repository struct {
	Path       string // Repository root path
	Name       string // Repository name (derived from directory name)
	GitDir     string // Path to .git directory or file (for worktrees)
	IsWorktree bool   // Whether this is a git worktree
}

