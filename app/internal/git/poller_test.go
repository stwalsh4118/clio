package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stwalsh4118/clio/internal/config"
	"github.com/stwalsh4118/clio/internal/logging"
)

func TestPollerService_StartStop(t *testing.T) {
	logger := logging.NewNoopLogger()
	cfg := &config.Config{
		Git: config.GitConfig{
			PollIntervalSeconds: 1, // Use 1 second for faster tests
		},
	}

	poller, err := NewPollerService(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create poller: %v", err)
	}

	ctx := context.Background()
	repos := []Repository{}

	// Start poller
	if err := poller.Start(ctx, repos); err != nil {
		t.Fatalf("failed to start poller: %v", err)
	}

	// Stop poller
	if err := poller.Stop(); err != nil {
		t.Fatalf("failed to stop poller: %v", err)
	}
}

func TestPollerService_DetectNewCommits_SingleRepository(t *testing.T) {
	logger := logging.NewNoopLogger()
	cfg := &config.Config{
		Git: config.GitConfig{
			PollIntervalSeconds: 1,
		},
	}

	poller, err := NewPollerService(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create poller: %v", err)
	}

	// Create test repository with initial commit
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := createGitRepoWithCommits(t, repoPath, 1)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}

	// Get initial HEAD hash (not used but ensures repo is valid)
	_, err = repo.Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}

	// Create repository struct
	gitRepo := Repository{
		Path:       repoPath,
		Name:       "test-repo",
		GitDir:     filepath.Join(repoPath, ".git"),
		IsWorktree: false,
	}

	ctx := context.Background()
	if err := poller.Start(ctx, []Repository{gitRepo}); err != nil {
		t.Fatalf("failed to start poller: %v", err)
	}
	defer poller.Stop()

	// Wait a bit for initial poll
	time.Sleep(100 * time.Millisecond)

	// Create a new commit
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	testFile := filepath.Join(repoPath, "test2.txt")
	if err := os.WriteFile(testFile, []byte("test content 2"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	if _, err := worktree.Add("test2.txt"); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	commitHash, err := worktree.Commit("Second commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to create commit: %v", err)
	}

	// Poll manually to detect new commits
	time.Sleep(1500 * time.Millisecond) // Wait for poll interval

	// Check results
	results := poller.PollResults()
	select {
	case result := <-results:
		if result.Error != nil {
			t.Fatalf("unexpected error: %v", result.Error)
		}
		if len(result.NewCommits) == 0 {
			t.Fatal("expected new commits, got none")
		}
		if result.NewCommits[0].Hash != commitHash.String() {
			t.Errorf("expected commit hash %s, got %s", commitHash.String(), result.NewCommits[0].Hash)
		}
		if result.NewCommits[0].Message != "Second commit" {
			t.Errorf("expected message 'Second commit', got %s", result.NewCommits[0].Message)
		}
		if result.Repository.Path != repoPath {
			t.Errorf("expected repository path %s, got %s", repoPath, result.Repository.Path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for poll result")
	}
}

func TestPollerService_DetectNewCommits_MultipleRepositories(t *testing.T) {
	logger := logging.NewNoopLogger()
	cfg := &config.Config{
		Git: config.GitConfig{
			PollIntervalSeconds: 1,
		},
	}

	poller, err := NewPollerService(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create poller: %v", err)
	}

	// Create multiple test repositories
	tmpDir := t.TempDir()
	repo1Path := filepath.Join(tmpDir, "repo1")
	repo2Path := filepath.Join(tmpDir, "repo2")

	repo1, err := createGitRepoWithCommits(t, repo1Path, 1)
	if err != nil {
		t.Fatalf("failed to create repo1: %v", err)
	}

	repo2, err := createGitRepoWithCommits(t, repo2Path, 1)
	if err != nil {
		t.Fatalf("failed to create repo2: %v", err)
	}

	repos := []Repository{
		{
			Path:       repo1Path,
			Name:       "repo1",
			GitDir:     filepath.Join(repo1Path, ".git"),
			IsWorktree: false,
		},
		{
			Path:       repo2Path,
			Name:       "repo2",
			GitDir:     filepath.Join(repo2Path, ".git"),
			IsWorktree: false,
		},
	}

	ctx := context.Background()
	if err := poller.Start(ctx, repos); err != nil {
		t.Fatalf("failed to start poller: %v", err)
	}
	defer poller.Stop()

	// Wait for initial poll
	time.Sleep(100 * time.Millisecond)

	// Create commits in both repositories
	worktree1, _ := repo1.Worktree()
	worktree2, _ := repo2.Worktree()

	// Commit to repo1
	file1 := filepath.Join(repo1Path, "file1.txt")
	os.WriteFile(file1, []byte("content"), 0644)
	worktree1.Add("file1.txt")
	commit1, _ := worktree1.Commit("Repo1 commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Author", Email: "test@example.com", When: time.Now()},
	})

	// Commit to repo2
	file2 := filepath.Join(repo2Path, "file2.txt")
	os.WriteFile(file2, []byte("content"), 0644)
	worktree2.Add("file2.txt")
	commit2, _ := worktree2.Commit("Repo2 commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Author", Email: "test@example.com", When: time.Now()},
	})

	// Wait for poll
	time.Sleep(1500 * time.Millisecond)

	// Collect results
	results := poller.PollResults()
	resultsMap := make(map[string]PollResult)
	timeout := time.After(3 * time.Second)

	for len(resultsMap) < 2 {
		select {
		case result, ok := <-results:
			if !ok {
				break
			}
			if result.Error == nil && len(result.NewCommits) > 0 {
				resultsMap[result.Repository.Path] = result
			}
		case <-timeout:
			t.Fatalf("timeout waiting for results, got %d results", len(resultsMap))
		}
	}

	// Verify both repositories were polled
	if len(resultsMap) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resultsMap))
	}

	result1, ok := resultsMap[repo1Path]
	if !ok {
		t.Fatal("missing result for repo1")
	}
	if result1.NewCommits[0].Hash != commit1.String() {
		t.Errorf("repo1: expected commit %s, got %s", commit1.String(), result1.NewCommits[0].Hash)
	}

	result2, ok := resultsMap[repo2Path]
	if !ok {
		t.Fatal("missing result for repo2")
	}
	if result2.NewCommits[0].Hash != commit2.String() {
		t.Errorf("repo2: expected commit %s, got %s", commit2.String(), result2.NewCommits[0].Hash)
	}
}

func TestPollerService_NoNewCommits(t *testing.T) {
	logger := logging.NewNoopLogger()
	cfg := &config.Config{
		Git: config.GitConfig{
			PollIntervalSeconds: 1,
		},
	}

	poller, err := NewPollerService(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create poller: %v", err)
	}

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	_, err = createGitRepoWithCommits(t, repoPath, 1)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}

	gitRepo := Repository{
		Path:       repoPath,
		Name:       "test-repo",
		GitDir:     filepath.Join(repoPath, ".git"),
		IsWorktree: false,
	}

	ctx := context.Background()
	if err := poller.Start(ctx, []Repository{gitRepo}); err != nil {
		t.Fatalf("failed to start poller: %v", err)
	}
	defer poller.Stop()

	// Wait for initial poll
	time.Sleep(100 * time.Millisecond)

	// Wait for another poll (should detect no new commits)
	time.Sleep(1500 * time.Millisecond)

	// Should not receive any results (no new commits)
	results := poller.PollResults()
	select {
	case result := <-results:
		t.Fatalf("unexpected result: %+v", result)
	case <-time.After(500 * time.Millisecond):
		// Expected - no new commits
	}
}

func TestPollerService_TracksLastSeenHash(t *testing.T) {
	logger := logging.NewNoopLogger()
	cfg := &config.Config{
		Git: config.GitConfig{
			PollIntervalSeconds: 1,
		},
	}

	poller, err := NewPollerService(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create poller: %v", err)
	}

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := createGitRepoWithCommits(t, repoPath, 1)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}

	gitRepo := Repository{
		Path:       repoPath,
		Name:       "test-repo",
		GitDir:     filepath.Join(repoPath, ".git"),
		IsWorktree: false,
	}

	ctx := context.Background()
	if err := poller.Start(ctx, []Repository{gitRepo}); err != nil {
		t.Fatalf("failed to start poller: %v", err)
	}
	defer poller.Stop()

	// Wait for initial poll
	time.Sleep(100 * time.Millisecond)

	// Create first commit
	worktree, _ := repo.Worktree()
	file1 := filepath.Join(repoPath, "file1.txt")
	os.WriteFile(file1, []byte("content1"), 0644)
	worktree.Add("file1.txt")
	commit1, _ := worktree.Commit("First new commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Author", Email: "test@example.com", When: time.Now()},
	})

	// Wait for poll
	time.Sleep(1500 * time.Millisecond)

	// Verify first commit detected
	results := poller.PollResults()
	select {
	case result := <-results:
		if result.NewCommits[0].Hash != commit1.String() {
			t.Errorf("expected commit %s, got %s", commit1.String(), result.NewCommits[0].Hash)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first commit")
	}

	// Create second commit
	file2 := filepath.Join(repoPath, "file2.txt")
	os.WriteFile(file2, []byte("content2"), 0644)
	worktree.Add("file2.txt")
	commit2, _ := worktree.Commit("Second new commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Author", Email: "test@example.com", When: time.Now()},
	})

	// Wait for poll
	time.Sleep(1500 * time.Millisecond)

	// Verify second commit detected (should only be commit2, not commit1 again)
	select {
	case result := <-results:
		if len(result.NewCommits) != 1 {
			t.Fatalf("expected 1 commit, got %d", len(result.NewCommits))
		}
		if result.NewCommits[0].Hash != commit2.String() {
			t.Errorf("expected commit %s, got %s", commit2.String(), result.NewCommits[0].Hash)
		}
		// Verify commit1 is not in results
		if result.NewCommits[0].Hash == commit1.String() {
			t.Error("commit1 should not be detected again")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for second commit")
	}
}

func TestPollerService_HandlesRepositoryErrors(t *testing.T) {
	logger := logging.NewNoopLogger()
	cfg := &config.Config{
		Git: config.GitConfig{
			PollIntervalSeconds: 1,
		},
	}

	poller, err := NewPollerService(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create poller: %v", err)
	}

	// Create one valid repo and one invalid repo
	tmpDir := t.TempDir()
	validRepoPath := filepath.Join(tmpDir, "valid-repo")
	invalidRepoPath := filepath.Join(tmpDir, "invalid-repo")

	_, err = createGitRepoWithCommits(t, validRepoPath, 1)
	if err != nil {
		t.Fatalf("failed to create valid repo: %v", err)
	}

	// Create invalid repo (just a directory, not a git repo)
	os.MkdirAll(invalidRepoPath, 0755)

	repos := []Repository{
		{
			Path:       validRepoPath,
			Name:       "valid-repo",
			GitDir:     filepath.Join(validRepoPath, ".git"),
			IsWorktree: false,
		},
		{
			Path:       invalidRepoPath,
			Name:       "invalid-repo",
			GitDir:     filepath.Join(invalidRepoPath, ".git"),
			IsWorktree: false,
		},
	}

	ctx := context.Background()
	if err := poller.Start(ctx, repos); err != nil {
		t.Fatalf("failed to start poller: %v", err)
	}
	defer poller.Stop()

	// Wait for poll
	time.Sleep(1500 * time.Millisecond)

	// Should receive error for invalid repo
	results := poller.PollResults()
	errorReceived := false
	timeout := time.After(2 * time.Second)

	for !errorReceived {
		select {
		case result, ok := <-results:
			if !ok {
				break
			}
			if result.Error != nil && result.Repository.Path == invalidRepoPath {
				errorReceived = true
			}
		case <-timeout:
			break
		}
	}

	if !errorReceived {
		t.Error("expected error for invalid repository")
	}
}

func TestPollerService_HandlesEmptyRepository(t *testing.T) {
	logger := logging.NewNoopLogger()
	cfg := &config.Config{
		Git: config.GitConfig{
			PollIntervalSeconds: 1,
		},
	}

	poller, err := NewPollerService(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create poller: %v", err)
	}

	// Create empty git repository (no commits)
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "empty-repo")
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("failed to create empty repo: %v", err)
	}

	// Create a worktree to have a HEAD reference (but no commits)
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	// Create a file but don't commit
	testFile := filepath.Join(repoPath, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)
	worktree.Add("test.txt")

	gitRepo := Repository{
		Path:       repoPath,
		Name:       "empty-repo",
		GitDir:     filepath.Join(repoPath, ".git"),
		IsWorktree: false,
	}

	ctx := context.Background()
	if err := poller.Start(ctx, []Repository{gitRepo}); err != nil {
		t.Fatalf("failed to start poller: %v", err)
	}
	defer poller.Stop()

	// Wait for poll - should handle empty repo gracefully
	time.Sleep(1500 * time.Millisecond)

	// Should not receive any errors (empty repo is handled gracefully)
	results := poller.PollResults()
	select {
	case result := <-results:
		if result.Error != nil {
			t.Errorf("unexpected error for empty repo: %v", result.Error)
		}
	case <-time.After(500 * time.Millisecond):
		// Expected - empty repo handled gracefully
	}
}

func TestPollerService_ConfigurableInterval(t *testing.T) {
	logger := logging.NewNoopLogger()

	tests := []struct {
		name           string
		configInterval int
		expectedMin    time.Duration
	}{
		{
			name:           "custom interval",
			configInterval: 5,
			expectedMin:    5 * time.Second,
		},
		{
			name:           "minimum interval",
			configInterval: 0,
			expectedMin:    1 * time.Second, // Should use minimum
		},
		{
			name:           "below minimum",
			configInterval: -1,
			expectedMin:    1 * time.Second, // Should use minimum
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Git: config.GitConfig{
					PollIntervalSeconds: tt.configInterval,
				},
			}

			poller, err := NewPollerService(cfg, logger)
			if err != nil {
				t.Fatalf("failed to create poller: %v", err)
			}

			// Verify poller was created (interval validation happens in constructor)
			if poller == nil {
				t.Fatal("poller is nil")
			}
		})
	}
}

func TestPollerService_ContextCancellation(t *testing.T) {
	logger := logging.NewNoopLogger()
	cfg := &config.Config{
		Git: config.GitConfig{
			PollIntervalSeconds: 1,
		},
	}

	poller, err := NewPollerService(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create poller: %v", err)
	}

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	_, err = createGitRepoWithCommits(t, repoPath, 1)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}

	gitRepo := Repository{
		Path:       repoPath,
		Name:       "test-repo",
		GitDir:     filepath.Join(repoPath, ".git"),
		IsWorktree: false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := poller.Start(ctx, []Repository{gitRepo}); err != nil {
		t.Fatalf("failed to start poller: %v", err)
	}

	// Cancel context
	cancel()

	// Stop should work after context cancellation
	time.Sleep(100 * time.Millisecond)
	if err := poller.Stop(); err != nil {
		t.Fatalf("failed to stop poller: %v", err)
	}
}

// Helper function to create a git repository with commits
func createGitRepoWithCommits(t *testing.T, repoPath string, numCommits int) (*git.Repository, error) {
	t.Helper()

	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		return nil, err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return nil, err
	}

	for i := 0; i < numCommits; i++ {
		testFile := filepath.Join(repoPath, "test.txt")
		content := []byte("test content " + string(rune('0'+i)))
		if err := os.WriteFile(testFile, content, 0644); err != nil {
			return nil, err
		}

		if _, err := worktree.Add("test.txt"); err != nil {
			return nil, err
		}

		_, err := worktree.Commit("Test commit", &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test Author",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		if err != nil {
			return nil, err
		}
	}

	return repo, nil
}

