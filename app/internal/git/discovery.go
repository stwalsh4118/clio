package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/stwalsh4118/clio/internal/logging"
)

// DiscoveryService provides methods for discovering git repositories
type DiscoveryService interface {
	DiscoverRepositories(dirs []string) ([]Repository, error)
	FindGitRepositories(dir string) ([]Repository, error)
}

// discoveryService implements DiscoveryService
type discoveryService struct {
	logger logging.Logger
}

// NewDiscoveryService creates a new discovery service instance
func NewDiscoveryService(logger logging.Logger) DiscoveryService {
	return &discoveryService{
		logger: logger.With("component", "git_discovery"),
	}
}

// DiscoverRepositories scans multiple watched directories for git repositories
func (ds *discoveryService) DiscoverRepositories(dirs []string) ([]Repository, error) {
	ds.logger.Debug("starting repository discovery", "directory_count", len(dirs))
	var allRepos []Repository
	seenPaths := make(map[string]bool) // Deduplicate repositories found in overlapping directories
	var skippedCount int

	for _, dir := range dirs {
		if dir == "" {
			ds.logger.Debug("skipping empty directory path")
			continue
		}

		// Expand and resolve path
		expandedPath := expandHomeDir(dir)
		resolvedPath, err := filepath.EvalSymlinks(expandedPath)
		if err != nil {
			// If symlink resolution fails, use expanded path
			ds.logger.Debug("failed to resolve symlinks, using expanded path", "path", dir, "error", err)
			resolvedPath = expandedPath
		}

		// Check if directory exists
		info, err := os.Stat(resolvedPath)
		if err != nil {
			if os.IsNotExist(err) {
				ds.logger.Warn("watched directory does not exist, skipping", "path", dir, "resolved_path", resolvedPath)
				skippedCount++
				continue
			}
			if os.IsPermission(err) {
				ds.logger.Warn("permission denied accessing watched directory, skipping", "path", dir, "resolved_path", resolvedPath, "error", err)
				skippedCount++
				continue
			}
			ds.logger.Warn("failed to access watched directory, skipping", "path", dir, "resolved_path", resolvedPath, "error", err)
			skippedCount++
			continue
		}

		if !info.IsDir() {
			ds.logger.Warn("watched path is not a directory, skipping", "path", dir, "resolved_path", resolvedPath)
			skippedCount++
			continue
		}

		ds.logger.Debug("scanning directory for git repositories", "path", dir, "resolved_path", resolvedPath)
		repos, err := ds.FindGitRepositories(resolvedPath)
		if err != nil {
			ds.logger.Warn("failed to scan directory for repositories, continuing with other directories", "path", dir, "resolved_path", resolvedPath, "error", err)
			skippedCount++
			continue // Continue with other directories
		}

		ds.logger.Debug("found repositories in directory", "path", dir, "count", len(repos))

		// Deduplicate repositories by path
		for _, repo := range repos {
			if !seenPaths[repo.Path] {
				seenPaths[repo.Path] = true
				allRepos = append(allRepos, repo)
				ds.logger.Info("discovered git repository", "path", repo.Path, "name", repo.Name, "is_worktree", repo.IsWorktree)
			} else {
				ds.logger.Debug("skipping duplicate repository", "path", repo.Path)
			}
		}
	}

	ds.logger.Info("repository discovery completed", "total_discovered", len(allRepos), "directories_skipped", skippedCount)
	return allRepos, nil
}

// FindGitRepositories recursively scans a directory for git repositories
func (ds *discoveryService) FindGitRepositories(dir string) ([]Repository, error) {
	var repos []Repository

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Log error but continue scanning
			if os.IsPermission(err) {
				ds.logger.Debug("permission denied", "path", path)
				return filepath.SkipDir // Skip this directory
			}
			ds.logger.Debug("error accessing path", "path", path, "error", err)
			return nil // Continue with other paths
		}

		// Skip .git directories during traversal to avoid scanning into git internals
		if d.IsDir() && d.Name() == ".git" {
			// Found a .git directory - this is a regular git repository
			repoRoot := filepath.Dir(path)
			
			// Validate repository before creating Repository struct
			if err := ds.validateRepository(repoRoot); err != nil {
				ds.logger.Warn("invalid or corrupted repository detected, skipping", "path", repoRoot, "git_dir", path, "error", err)
				return filepath.SkipDir // Skip this directory
			}
			
			repo, err := ds.createRepository(repoRoot, path, false)
			if err != nil {
				ds.logger.Warn("failed to create repository from .git directory, skipping", "path", path, "repo_root", repoRoot, "error", err)
				return filepath.SkipDir // Skip this directory
			}
			repos = append(repos, repo)
			ds.logger.Debug("found regular git repository", "repo_root", repoRoot, "git_dir", path)
			return filepath.SkipDir // Don't scan into .git directory
		}

		// Check for .git file (worktree)
		if !d.IsDir() && d.Name() == ".git" {
			repoRoot := filepath.Dir(path)
			repo, err := ds.createRepositoryFromWorktree(repoRoot, path)
			if err != nil {
				ds.logger.Warn("failed to create repository from .git file, skipping", "path", path, "repo_root", repoRoot, "error", err)
				return nil // Continue scanning
			}
			
			// Validate worktree repository
			if err := ds.validateRepository(repoRoot); err != nil {
				ds.logger.Warn("invalid or corrupted worktree repository detected, skipping", "path", repoRoot, "git_file", path, "error", err)
				return nil // Continue scanning
			}
			
			repos = append(repos, repo)
			ds.logger.Debug("found git worktree", "repo_root", repoRoot, "git_file", path)
			return nil // Continue scanning
		}

		return nil
	})

	if err != nil {
		return repos, fmt.Errorf("error scanning directory: %w", err)
	}

	return repos, nil
}

// validateRepository checks if a repository path is valid by attempting to open it
func (ds *discoveryService) validateRepository(repoPath string) error {
	_, err := git.PlainOpen(repoPath)
	if err != nil {
		// Repository is invalid, corrupted, or doesn't exist
		return fmt.Errorf("repository validation failed: %w", err)
	}
	return nil
}

// createRepository creates a Repository struct for a regular git repository
func (ds *discoveryService) createRepository(repoRoot, gitDir string, isWorktree bool) (Repository, error) {
	// Ensure paths are absolute and cleaned
	absRepoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return Repository{}, fmt.Errorf("failed to get absolute path: %w", err)
	}

	absGitDir, err := filepath.Abs(gitDir)
	if err != nil {
		return Repository{}, fmt.Errorf("failed to get absolute git dir path: %w", err)
	}

	// Derive repository name from directory name
	repoName := filepath.Base(absRepoRoot)

	return Repository{
		Path:       absRepoRoot,
		Name:       repoName,
		GitDir:     absGitDir,
		IsWorktree: isWorktree,
	}, nil
}

// createRepositoryFromWorktree creates a Repository struct for a git worktree
func (ds *discoveryService) createRepositoryFromWorktree(repoRoot, gitFile string) (Repository, error) {
	// Read .git file to get actual git directory path
	content, err := os.ReadFile(gitFile)
	if err != nil {
		if os.IsNotExist(err) {
			return Repository{}, fmt.Errorf(".git file does not exist: %w", err)
		}
		if os.IsPermission(err) {
			return Repository{}, fmt.Errorf("permission denied reading .git file: %w", err)
		}
		return Repository{}, fmt.Errorf("failed to read .git file: %w", err)
	}

	// Parse worktree .git file format: "gitdir: <path>"
	contentStr := strings.TrimSpace(string(content))
	if contentStr == "" {
		return Repository{}, fmt.Errorf("empty .git file")
	}
	if !strings.HasPrefix(contentStr, "gitdir: ") {
		return Repository{}, fmt.Errorf("invalid .git file format: expected 'gitdir: <path>' prefix")
	}

	gitDirPath := strings.TrimSpace(strings.TrimPrefix(contentStr, "gitdir: "))
	if gitDirPath == "" {
		return Repository{}, fmt.Errorf("empty git directory path in .git file")
	}
	
	// Resolve relative paths (worktree .git files often contain relative paths)
	if !filepath.IsAbs(gitDirPath) {
		gitDirPath = filepath.Join(repoRoot, gitDirPath)
	}

	// Resolve symlinks and clean path
	resolvedGitDir, err := filepath.EvalSymlinks(gitDirPath)
	if err != nil {
		// If resolution fails, use the path as-is but log a warning
		ds.logger.Debug("failed to resolve symlinks for git directory, using path as-is", "git_dir_path", gitDirPath, "error", err)
		resolvedGitDir = filepath.Clean(gitDirPath)
	} else {
		resolvedGitDir = filepath.Clean(resolvedGitDir)
	}

	// Verify the git directory exists
	info, err := os.Stat(resolvedGitDir)
	if err != nil {
		if os.IsNotExist(err) {
			return Repository{}, fmt.Errorf("git directory does not exist: %w", err)
		}
		if os.IsPermission(err) {
			return Repository{}, fmt.Errorf("permission denied accessing git directory: %w", err)
		}
		return Repository{}, fmt.Errorf("failed to stat git directory: %w", err)
	}

	if !info.IsDir() {
		return Repository{}, fmt.Errorf("git directory path is not a directory: %s", resolvedGitDir)
	}

	return ds.createRepository(repoRoot, resolvedGitDir, true)
}

// expandHomeDir expands ~ in a path to the user's home directory
func expandHomeDir(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		// If we can't get home dir, return path as-is
		return path
	}

	if path == "~" {
		return homeDir
	}

	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, path[2:])
	}

	return path
}

