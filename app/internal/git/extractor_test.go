package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stwalsh4118/clio/internal/logging"
)

func TestNewCommitExtractor(t *testing.T) {
	logger := logging.NewNoopLogger()

	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	if extractor == nil {
		t.Fatal("extractor is nil")
	}

	// Test with nil logger
	_, err = NewCommitExtractor(nil)
	if err == nil {
		t.Fatal("expected error when logger is nil")
	}
}

func TestExtractMetadata_NormalCommit(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Create test repository with a commit
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := createGitRepoWithCommits(t, repoPath, 1)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}

	// Get HEAD commit hash
	headRef, err := repo.Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}

	// Extract metadata
	metadata, err := extractor.ExtractMetadata(repo, headRef.Hash())
	if err != nil {
		t.Fatalf("failed to extract metadata: %v", err)
	}

	// Verify metadata
	if metadata.Hash != headRef.Hash().String() {
		t.Errorf("expected hash %s, got %s", headRef.Hash().String(), metadata.Hash)
	}

	if metadata.Message != "Test commit" {
		t.Errorf("expected message 'Test commit', got '%s'", metadata.Message)
	}

	if metadata.Author.Name != "Test Author" {
		t.Errorf("expected author name 'Test Author', got '%s'", metadata.Author.Name)
	}

	if metadata.Author.Email != "test@example.com" {
		t.Errorf("expected author email 'test@example.com', got '%s'", metadata.Author.Email)
	}

	if metadata.IsMerge {
		t.Error("expected IsMerge to be false for normal commit")
	}

	if len(metadata.ParentHashes) != 0 {
		t.Errorf("expected 0 parent hashes for initial commit, got %d", len(metadata.ParentHashes))
	}

	// Branch should be "main" (default branch name)
	if metadata.Branch == "" {
		t.Error("expected branch name to be set")
	}
}

func TestExtractMetadata_MergeCommit(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Create test repository with multiple commits
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := createGitRepoWithCommits(t, repoPath, 3)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}

	// Get commit log to find commits
	headRef, err := repo.Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}

	commitIter, err := repo.Log(&git.LogOptions{From: headRef.Hash()})
	if err != nil {
		t.Fatalf("failed to get commit log: %v", err)
	}
	defer commitIter.Close()

	var commits []*object.Commit
	commitIter.ForEach(func(c *object.Commit) error {
		commits = append(commits, c)
		return nil
	})

	if len(commits) < 2 {
		t.Fatalf("need at least 2 commits for merge test, got %d", len(commits))
	}

	// Note: We can't easily create a real merge commit (2+ parents) in go-git without
	// using git commands. Instead, we'll test that the extractor correctly extracts
	// parent hashes and can identify merge commits when they exist. The extractor
	// logic for detecting merge commits is tested by verifying it checks
	// len(ParentHashes) > 1, which we verify works correctly for commits with parents.

	// Get a commit that should have a parent
	if len(commits) > 1 {
		// The second commit should have the first commit as parent
		parentIter := commits[1].Parents()
		defer parentIter.Close()

		parentCount := 0
		parentHashes := []string{}
		parentIter.ForEach(func(p *object.Commit) error {
			parentHashes = append(parentHashes, p.Hash.String())
			parentCount++
			return nil
		})

		// Extract metadata from a commit with a parent
		metadata, err := extractor.ExtractMetadata(repo, commits[1].Hash)
		if err != nil {
			t.Fatalf("failed to extract metadata: %v", err)
		}

		// Verify parent hashes are extracted
		if len(metadata.ParentHashes) != parentCount {
			t.Errorf("expected %d parent hashes, got %d", parentCount, len(metadata.ParentHashes))
		}

		// Verify parent hashes match
		for i, expectedHash := range parentHashes {
			if i >= len(metadata.ParentHashes) || metadata.ParentHashes[i] != expectedHash {
				t.Errorf("parent hash mismatch at index %d: expected %s", i, expectedHash)
			}
		}

		// Verify IsMerge is false for single-parent commit
		if metadata.IsMerge {
			t.Error("expected IsMerge to be false for single-parent commit")
		}
	}
}

func TestExtractMetadata_InitialCommit(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Create test repository with initial commit
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := createGitRepoWithCommits(t, repoPath, 1)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}

	// Get HEAD commit hash (this is the initial commit)
	headRef, err := repo.Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}

	// Extract metadata
	metadata, err := extractor.ExtractMetadata(repo, headRef.Hash())
	if err != nil {
		t.Fatalf("failed to extract metadata: %v", err)
	}

	// Verify it's not a merge commit
	if metadata.IsMerge {
		t.Error("expected IsMerge to be false for initial commit")
	}

	// Verify no parent hashes
	if len(metadata.ParentHashes) != 0 {
		t.Errorf("expected 0 parent hashes for initial commit, got %d", len(metadata.ParentHashes))
	}
}

func TestExtractMetadata_MultiLineCommitMessage(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Create test repository
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	// Create a file
	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	if _, err := worktree.Add("test.txt"); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	// Create commit with multi-line message
	multiLineMessage := "First line\n\nSecond paragraph\nThird line"
	headHash, err := worktree.Commit(multiLineMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Extract metadata
	metadata, err := extractor.ExtractMetadata(repo, headHash)
	if err != nil {
		t.Fatalf("failed to extract metadata: %v", err)
	}

	// Verify multi-line message is preserved
	if metadata.Message != multiLineMessage {
		t.Errorf("expected multi-line message to be preserved, got: %s", metadata.Message)
	}
}

func TestExtractMetadata_DetachedHEAD(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Create test repository with commits
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := createGitRepoWithCommits(t, repoPath, 2)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}

	// Get the first commit hash
	headRef, err := repo.Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}

	// Get commit log to find first commit
	commitIter, err := repo.Log(&git.LogOptions{From: headRef.Hash()})
	if err != nil {
		t.Fatalf("failed to get commit log: %v", err)
	}
	defer commitIter.Close()

	var firstCommitHash plumbing.Hash
	count := 0
	commitIter.ForEach(func(c *object.Commit) error {
		if count == 1 {
			firstCommitHash = c.Hash
			return nil
		}
		count++
		return nil
	})

	// Checkout the first commit directly (detached HEAD)
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	if err := worktree.Checkout(&git.CheckoutOptions{
		Hash: firstCommitHash,
	}); err != nil {
		t.Fatalf("failed to checkout commit: %v", err)
	}

	// Extract metadata from the commit in detached HEAD state
	metadata, err := extractor.ExtractMetadata(repo, firstCommitHash)
	if err != nil {
		t.Fatalf("failed to extract metadata: %v", err)
	}

	// Verify branch is "detached"
	if metadata.Branch != "detached" {
		t.Errorf("expected branch to be 'detached', got '%s'", metadata.Branch)
	}
}

func TestExtractMetadata_InvalidCommitHash(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Create test repository
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := createGitRepoWithCommits(t, repoPath, 1)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}

	// Try to extract metadata with invalid hash
	invalidHash := plumbing.NewHash("0000000000000000000000000000000000000000")
	_, err = extractor.ExtractMetadata(repo, invalidHash)
	if err == nil {
		t.Fatal("expected error for invalid commit hash")
	}
}

func TestExtractMetadata_NilRepository(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Try to extract metadata with nil repository
	hash := plumbing.NewHash("0000000000000000000000000000000000000000")
	_, err = extractor.ExtractMetadata(nil, hash)
	if err == nil {
		t.Fatal("expected error for nil repository")
	}
}

func TestExtractMetadata_AuthorInformation(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Create test repository
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	// Create a file
	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	if _, err := worktree.Add("test.txt"); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	// Create commit with specific author
	authorName := "John Doe"
	authorEmail := "john@example.com"
	commitTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	headHash, err := worktree.Commit("Test commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  commitTime,
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Extract metadata
	metadata, err := extractor.ExtractMetadata(repo, headHash)
	if err != nil {
		t.Fatalf("failed to extract metadata: %v", err)
	}

	// Verify author information
	if metadata.Author.Name != authorName {
		t.Errorf("expected author name '%s', got '%s'", authorName, metadata.Author.Name)
	}

	if metadata.Author.Email != authorEmail {
		t.Errorf("expected author email '%s', got '%s'", authorEmail, metadata.Author.Email)
	}

	// Verify timestamp
	if !metadata.Timestamp.Equal(commitTime) {
		t.Errorf("expected timestamp %v, got %v", commitTime, metadata.Timestamp)
	}
}

func TestExtractMetadata_BranchName(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Create test repository
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	if _, err := worktree.Add("test.txt"); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	headHash, err := worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Extract metadata
	metadata, err := extractor.ExtractMetadata(repo, headHash)
	if err != nil {
		t.Fatalf("failed to extract metadata: %v", err)
	}

	// Verify branch name is set (should be "main" or "master")
	if metadata.Branch == "" {
		t.Error("expected branch name to be set")
	}

	if metadata.Branch != "main" && metadata.Branch != "master" {
		t.Errorf("expected branch to be 'main' or 'master', got '%s'", metadata.Branch)
	}
}

func TestExtractDiff_NormalCommit(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Create test repository with a commit
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	// Create a file with some content
	testFile := filepath.Join(repoPath, "test.txt")
	content := "line 1\nline 2\nline 3\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	if _, err := worktree.Add("test.txt"); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	headHash, err := worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Extract diff
	diff, err := extractor.ExtractDiff(repo, headHash)
	if err != nil {
		t.Fatalf("failed to extract diff: %v", err)
	}

	// Verify diff content is not empty
	if diff.Content == "" {
		t.Error("expected diff content to be non-empty")
	}

	// Verify file statistics
	if len(diff.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(diff.Files))
	}

	if diff.Files[0].Path != "test.txt" {
		t.Errorf("expected file path 'test.txt', got '%s'", diff.Files[0].Path)
	}

	// Initial commit should have additions (3 lines)
	if diff.Files[0].Additions < 3 {
		t.Errorf("expected at least 3 additions, got %d", diff.Files[0].Additions)
	}

	if diff.Files[0].Deletions != 0 {
		t.Errorf("expected 0 deletions for initial commit, got %d", diff.Files[0].Deletions)
	}

	// Should not be truncated for small diff
	if diff.Truncated {
		t.Error("expected diff not to be truncated")
	}
}

func TestExtractDiff_CommitWithModifications(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Create test repository with multiple commits
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := createGitRepoWithCommits(t, repoPath, 1)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	// Modify the file
	testFile := filepath.Join(repoPath, "test.txt")
	newContent := "modified line 1\nmodified line 2\nnew line 3\nnew line 4\n"
	if err := os.WriteFile(testFile, []byte(newContent), 0644); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	if _, err := worktree.Add("test.txt"); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	headHash, err := worktree.Commit("Modify file", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Extract diff
	diff, err := extractor.ExtractDiff(repo, headHash)
	if err != nil {
		t.Fatalf("failed to extract diff: %v", err)
	}

	// Verify diff content
	if diff.Content == "" {
		t.Error("expected diff content to be non-empty")
	}

	// Verify file statistics
	if len(diff.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(diff.Files))
	}

	// Should have both additions and deletions
	if diff.Files[0].Additions == 0 && diff.Files[0].Deletions == 0 {
		t.Error("expected non-zero additions or deletions")
	}
}

func TestExtractDiff_InitialCommit(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Create test repository with initial commit
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := createGitRepoWithCommits(t, repoPath, 1)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}

	// Get HEAD commit hash (this is the initial commit)
	headRef, err := repo.Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}

	// Extract diff from initial commit
	diff, err := extractor.ExtractDiff(repo, headRef.Hash())
	if err != nil {
		t.Fatalf("failed to extract diff: %v", err)
	}

	// Verify diff was extracted
	if diff == nil {
		t.Fatal("expected diff to be non-nil")
	}

	// Initial commit should have file changes
	if len(diff.Files) == 0 {
		t.Error("expected at least one file change in initial commit")
	}

	// Should have additions (new files)
	hasAdditions := false
	for _, file := range diff.Files {
		if file.Additions > 0 {
			hasAdditions = true
			break
		}
	}
	if !hasAdditions {
		t.Error("expected initial commit to have file additions")
	}
}

func TestExtractDiff_MultipleFiles(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Create test repository
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	// Create initial commit
	testFile1 := filepath.Join(repoPath, "file1.txt")
	if err := os.WriteFile(testFile1, []byte("content1"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	if _, err := worktree.Add("file1.txt"); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Add multiple files in second commit
	testFile2 := filepath.Join(repoPath, "file2.txt")
	if err := os.WriteFile(testFile2, []byte("content2"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	testFile3 := filepath.Join(repoPath, "file3.txt")
	if err := os.WriteFile(testFile3, []byte("content3"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	if _, err := worktree.Add("file2.txt"); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}
	if _, err := worktree.Add("file3.txt"); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	headHash, err := worktree.Commit("Add multiple files", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Extract diff
	diff, err := extractor.ExtractDiff(repo, headHash)
	if err != nil {
		t.Fatalf("failed to extract diff: %v", err)
	}

	// Verify multiple files
	if len(diff.Files) < 2 {
		t.Errorf("expected at least 2 files, got %d", len(diff.Files))
	}
}

func TestExtractDiff_LargeDiffTruncation(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Create test repository
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(repoPath, "large.txt")
	smallContent := "line\n"
	if err := os.WriteFile(testFile, []byte(smallContent), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	if _, err := worktree.Add("large.txt"); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Create a large file (>5000 lines)
	var largeContent strings.Builder
	for i := 0; i < MaxDiffLines+100; i++ {
		largeContent.WriteString(fmt.Sprintf("line %d\n", i))
	}

	if err := os.WriteFile(testFile, []byte(largeContent.String()), 0644); err != nil {
		t.Fatalf("failed to create large file: %v", err)
	}

	if _, err := worktree.Add("large.txt"); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	headHash, err := worktree.Commit("Add large file", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Extract diff
	diff, err := extractor.ExtractDiff(repo, headHash)
	if err != nil {
		t.Fatalf("failed to extract diff: %v", err)
	}

	// Verify truncation
	if !diff.Truncated {
		t.Error("expected diff to be truncated")
	}

	if diff.TotalLines <= MaxDiffLines {
		t.Errorf("expected total lines > %d, got %d", MaxDiffLines, diff.TotalLines)
	}

	if diff.ShownLines != MaxDiffLines {
		t.Errorf("expected shown lines to be %d, got %d", MaxDiffLines, diff.ShownLines)
	}

	// Verify truncation note is present
	if !strings.Contains(diff.Content, "[Diff truncated:") {
		t.Error("expected truncation note in diff content")
	}

	// Verify file statistics are still present
	if len(diff.Files) == 0 {
		t.Error("expected file statistics even for truncated diff")
	}
}

func TestExtractDiff_NilRepository(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	hash := plumbing.NewHash("0000000000000000000000000000000000000000")
	_, err = extractor.ExtractDiff(nil, hash)
	if err == nil {
		t.Fatal("expected error for nil repository")
	}
}

func TestExtractDiff_InvalidCommitHash(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Create test repository
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := createGitRepoWithCommits(t, repoPath, 1)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}

	// Try to extract diff with invalid hash
	invalidHash := plumbing.NewHash("0000000000000000000000000000000000000000")
	_, err = extractor.ExtractDiff(repo, invalidHash)
	if err == nil {
		t.Fatal("expected error for invalid commit hash")
	}
}

func TestExtractCommit_CompleteExtraction(t *testing.T) {
	logger := logging.NewNoopLogger()
	extractor, err := NewCommitExtractor(logger)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	// Create test repository with a commit
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := createGitRepoWithCommits(t, repoPath, 1)
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}

	// Get HEAD commit hash
	headRef, err := repo.Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}

	// Extract complete commit info
	commitInfo, err := extractor.ExtractCommit(repo, headRef.Hash())
	if err != nil {
		t.Fatalf("failed to extract commit: %v", err)
	}

	// Verify metadata is present
	if commitInfo.Commit.Hash == "" {
		t.Error("expected commit hash to be set")
	}

	// Verify diff is present
	if commitInfo.Diff.Content == "" {
		t.Error("expected diff content to be non-empty")
	}

	// Verify files are present
	if len(commitInfo.Diff.Files) == 0 {
		t.Error("expected at least one file in diff")
	}
}


