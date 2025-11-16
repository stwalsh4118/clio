package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stwalsh4118/clio/internal/logging"
)

func TestDiscoveryService_DiscoverRepositories(t *testing.T) {
	logger := logging.NewNoopLogger()
	ds := NewDiscoveryService(logger)

	t.Run("single watched directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		// Create a git repository
		repo1 := filepath.Join(tmpDir, "repo1")
		createTestGitRepo(t, repo1, false)

		repos, err := ds.DiscoverRepositories([]string{tmpDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(repos) != 1 {
			t.Fatalf("expected 1 repository, got %d", len(repos))
		}

		if repos[0].Path != repo1 {
			t.Errorf("expected path %s, got %s", repo1, repos[0].Path)
		}

		if repos[0].Name != "repo1" {
			t.Errorf("expected name 'repo1', got %s", repos[0].Name)
		}

		if repos[0].IsWorktree {
			t.Error("expected IsWorktree to be false")
		}
	})

	t.Run("multiple watched directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		repo1 := filepath.Join(tmpDir, "repo1")
		repo2 := filepath.Join(tmpDir, "repo2")
		createTestGitRepo(t, repo1, false)
		createTestGitRepo(t, repo2, false)

		repos, err := ds.DiscoverRepositories([]string{tmpDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(repos) != 2 {
			t.Fatalf("expected 2 repositories, got %d", len(repos))
		}
	})

	t.Run("nested repositories (submodules)", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		repo1 := filepath.Join(tmpDir, "repo1")
		subRepo := filepath.Join(repo1, "submodule")
		createTestGitRepo(t, repo1, false)
		createTestGitRepo(t, subRepo, false)

		repos, err := ds.DiscoverRepositories([]string{tmpDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(repos) != 2 {
			t.Fatalf("expected 2 repositories, got %d", len(repos))
		}
	})

	t.Run("worktree repositories", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		repo1 := filepath.Join(tmpDir, "repo1")
		createTestGitRepo(t, repo1, false)
		
		// Create a worktree
		worktreePath := filepath.Join(tmpDir, "worktree")
		createTestWorktree(t, repo1, worktreePath)

		repos, err := ds.DiscoverRepositories([]string{tmpDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(repos) != 2 {
			t.Fatalf("expected 2 repositories, got %d", len(repos))
		}

		// Find the worktree repository
		var worktreeRepo *Repository
		for i := range repos {
			if repos[i].IsWorktree {
				worktreeRepo = &repos[i]
				break
			}
		}

		if worktreeRepo == nil {
			t.Fatal("expected to find a worktree repository")
		}

		if worktreeRepo.Path != worktreePath {
			t.Errorf("expected worktree path %s, got %s", worktreePath, worktreeRepo.Path)
		}
	})

	t.Run("skip non-git directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		// Create a non-git directory
		nonRepo := filepath.Join(tmpDir, "not-a-repo")
		if err := os.MkdirAll(nonRepo, 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}

		repos, err := ds.DiscoverRepositories([]string{tmpDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(repos) != 0 {
			t.Fatalf("expected 0 repositories, got %d", len(repos))
		}
	})

	t.Run("handle inaccessible directories gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		// Create a directory with restricted permissions (if possible)
		restrictedDir := filepath.Join(tmpDir, "restricted")
		if err := os.MkdirAll(restrictedDir, 0000); err != nil {
			t.Skip("cannot create restricted directory on this system")
		}
		defer os.Chmod(restrictedDir, 0755) // Cleanup

		// Should not error, just skip the inaccessible directory
		repos, err := ds.DiscoverRepositories([]string{tmpDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should still work, just skip the restricted directory
		_ = repos
	})

	t.Run("handle non-existent directories", func(t *testing.T) {
		repos, err := ds.DiscoverRepositories([]string{"/nonexistent/path/12345"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(repos) != 0 {
			t.Fatalf("expected 0 repositories, got %d", len(repos))
		}
	})

	t.Run("deduplicate overlapping directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		repo1 := filepath.Join(tmpDir, "repo1")
		createTestGitRepo(t, repo1, false)

		// Scan both parent and child directory - should only find repo once
		repos, err := ds.DiscoverRepositories([]string{tmpDir, repo1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(repos) != 1 {
			t.Fatalf("expected 1 repository (deduplicated), got %d", len(repos))
		}
	})
}

func TestDiscoveryService_FindGitRepositories(t *testing.T) {
	logger := logging.NewNoopLogger()
	ds := NewDiscoveryService(logger)

	t.Run("find single repository", func(t *testing.T) {
		tmpDir := t.TempDir()
		repo1 := filepath.Join(tmpDir, "repo1")
		createTestGitRepo(t, repo1, false)

		repos, err := ds.FindGitRepositories(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(repos) != 1 {
			t.Fatalf("expected 1 repository, got %d", len(repos))
		}
	})

	t.Run("skip scanning into .git directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		repo1 := filepath.Join(tmpDir, "repo1")
		createTestGitRepo(t, repo1, false)

		// Create a fake .git directory structure that shouldn't be scanned
		gitDir := filepath.Join(repo1, ".git")
		fakeRepo := filepath.Join(gitDir, "fake-repo")
		if err := os.MkdirAll(fakeRepo, 0755); err != nil {
			t.Fatalf("failed to create fake repo: %v", err)
		}
		createTestGitRepo(t, fakeRepo, false)

		repos, err := ds.FindGitRepositories(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should only find repo1, not the fake repo inside .git
		if len(repos) != 1 {
			t.Fatalf("expected 1 repository, got %d", len(repos))
		}
	})

	t.Run("extract correct repository metadata", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoName := "my-awesome-repo"
		repoPath := filepath.Join(tmpDir, repoName)
		createTestGitRepo(t, repoPath, false)

		repos, err := ds.FindGitRepositories(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(repos) != 1 {
			t.Fatalf("expected 1 repository, got %d", len(repos))
		}

		repo := repos[0]
		if repo.Name != repoName {
			t.Errorf("expected name %s, got %s", repoName, repo.Name)
		}

		if repo.Path != repoPath {
			t.Errorf("expected path %s, got %s", repoPath, repo.Path)
		}

		expectedGitDir := filepath.Join(repoPath, ".git")
		if repo.GitDir != expectedGitDir {
			t.Errorf("expected git dir %s, got %s", expectedGitDir, repo.GitDir)
		}

		if repo.IsWorktree {
			t.Error("expected IsWorktree to be false")
		}
	})

	t.Run("handle corrupted repository gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		// Create a directory with .git but invalid structure (corrupted)
		corruptedRepo := filepath.Join(tmpDir, "corrupted-repo")
		if err := os.MkdirAll(corruptedRepo, 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		
		// Create .git directory but make it invalid (empty or missing critical files)
		gitDir := filepath.Join(corruptedRepo, ".git")
		if err := os.MkdirAll(gitDir, 0755); err != nil {
			t.Fatalf("failed to create .git directory: %v", err)
		}
		// Don't create HEAD or other required files - this makes it invalid

		// Create a valid repo in the same directory to ensure discovery continues
		validRepo := filepath.Join(tmpDir, "valid-repo")
		createTestGitRepo(t, validRepo, false)

		repos, err := ds.FindGitRepositories(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should only find the valid repository, corrupted one should be skipped
		if len(repos) != 1 {
			t.Fatalf("expected 1 repository (valid one), got %d", len(repos))
		}

		if repos[0].Path != validRepo {
			t.Errorf("expected valid repository, got %s", repos[0].Path)
		}
	})

	t.Run("handle invalid worktree .git file gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		// Create a directory with invalid .git file
		invalidWorktree := filepath.Join(tmpDir, "invalid-worktree")
		if err := os.MkdirAll(invalidWorktree, 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		
		// Create .git file with invalid format
		gitFile := filepath.Join(invalidWorktree, ".git")
		if err := os.WriteFile(gitFile, []byte("invalid content\n"), 0644); err != nil {
			t.Fatalf("failed to create .git file: %v", err)
		}

		// Create a valid repo to ensure discovery continues
		validRepo := filepath.Join(tmpDir, "valid-repo")
		createTestGitRepo(t, validRepo, false)

		repos, err := ds.FindGitRepositories(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should only find the valid repository
		if len(repos) != 1 {
			t.Fatalf("expected 1 repository (valid one), got %d", len(repos))
		}

		if repos[0].Path != validRepo {
			t.Errorf("expected valid repository, got %s", repos[0].Path)
		}
	})

	t.Run("handle missing repository directory gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		// Create a valid repo
		validRepo := filepath.Join(tmpDir, "valid-repo")
		createTestGitRepo(t, validRepo, false)

		// Create a worktree with missing git directory
		missingWorktree := filepath.Join(tmpDir, "missing-worktree")
		if err := os.MkdirAll(missingWorktree, 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		
		// Create .git file pointing to non-existent directory
		gitFile := filepath.Join(missingWorktree, ".git")
		gitFileContent := "gitdir: /nonexistent/path/to/git\n"
		if err := os.WriteFile(gitFile, []byte(gitFileContent), 0644); err != nil {
			t.Fatalf("failed to create .git file: %v", err)
		}

		repos, err := ds.FindGitRepositories(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should only find the valid repository
		if len(repos) != 1 {
			t.Fatalf("expected 1 repository (valid one), got %d", len(repos))
		}

		if repos[0].Path != validRepo {
			t.Errorf("expected valid repository, got %s", repos[0].Path)
		}
	})
}

// Helper functions

// createTestGitRepo creates a test git repository by creating a .git directory
func createTestGitRepo(t *testing.T, repoPath string, isBare bool) {
	t.Helper()

	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("failed to create repo directory: %v", err)
	}

	var gitDir string
	if isBare {
		gitDir = repoPath
	} else {
		gitDir = filepath.Join(repoPath, ".git")
	}

	// Create minimal .git directory structure
	dirs := []string{
		filepath.Join(gitDir, "objects"),
		filepath.Join(gitDir, "refs", "heads"),
		filepath.Join(gitDir, "refs", "tags"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create git dir: %v", err)
		}
	}

	// Create HEAD file
	headFile := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headFile, []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatalf("failed to create HEAD file: %v", err)
	}
}

// createTestWorktree creates a test git worktree
func createTestWorktree(t *testing.T, repoPath, worktreePath string) {
	t.Helper()

	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatalf("failed to create worktree directory: %v", err)
	}

	// Create .git file pointing to the main repository's .git directory
	gitFile := filepath.Join(worktreePath, ".git")
	gitDir := filepath.Join(repoPath, ".git")
	
	// For worktrees, the .git file contains a relative or absolute path
	// We'll use an absolute path for simplicity
	gitFileContent := "gitdir: " + gitDir + "\n"
	if err := os.WriteFile(gitFile, []byte(gitFileContent), 0644); err != nil {
		t.Fatalf("failed to create .git file: %v", err)
	}
}

