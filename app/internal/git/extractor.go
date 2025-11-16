package git

import (
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stwalsh4118/clio/internal/logging"
)

// CommitExtractor defines the interface for extracting commit metadata and diffs
type CommitExtractor interface {
	ExtractMetadata(repo *git.Repository, hash plumbing.Hash) (*CommitMetadata, error)
	ExtractDiff(repo *git.Repository, hash plumbing.Hash) (*Diff, error)
	ExtractCommit(repo *git.Repository, hash plumbing.Hash) (*CommitInfo, error)
}

// CommitInfo represents complete commit information (metadata + diff)
type CommitInfo struct {
	Commit CommitMetadata // Commit metadata
	Diff   Diff          // Commit diff
}

// Diff represents a commit diff (to be implemented in task 3-5)
type Diff struct {
	Content    string       // Full diff content (may be truncated)
	Files      []FileChange // File-level statistics
	Truncated  bool         // Whether diff was truncated due to size
	TotalLines int          // Total lines in diff (if truncated)
	ShownLines int          // Lines shown (if truncated)
}

// FileChange represents file-level change statistics
type FileChange struct {
	Path      string // File path relative to repository root
	Additions int    // Number of lines added
	Deletions int    // Number of lines deleted
}

// commitExtractor implements CommitExtractor
type commitExtractor struct {
	logger logging.Logger
}

// NewCommitExtractor creates a new commit extractor instance
func NewCommitExtractor(logger logging.Logger) (CommitExtractor, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	return &commitExtractor{
		logger: logger.With("component", "git_extractor"),
	}, nil
}

// ExtractMetadata extracts commit metadata from a git commit
func (ce *commitExtractor) ExtractMetadata(repo *git.Repository, hash plumbing.Hash) (*CommitMetadata, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	// Get commit object
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit object: %w", err)
	}

	// Extract basic metadata
	metadata := &CommitMetadata{
		Hash:      commit.Hash.String(),
		Message:   commit.Message,
		Timestamp: commit.Author.When,
		Author: AuthorInfo{
			Name:  commit.Author.Name,
			Email: commit.Author.Email,
		},
	}

	// Extract parent hashes
	parentHashes := []string{}
	parentIter := commit.Parents()
	defer parentIter.Close()

	err = parentIter.ForEach(func(parent *object.Commit) error {
		parentHashes = append(parentHashes, parent.Hash.String())
		return nil
	})
	if err != nil {
		// Log error but continue - parent iteration failure shouldn't stop extraction
		ce.logger.Debug("failed to iterate parent commits", "commit", commit.Hash.String(), "error", err)
	}

	metadata.ParentHashes = parentHashes

	// Detect merge commits
	metadata.IsMerge = len(parentHashes) > 1

	// Determine branch name
	branchName, err := ce.determineBranchName(repo, hash)
	if err != nil {
		// Log warning but continue - branch detection failure shouldn't stop extraction
		ce.logger.Warn("failed to determine branch name", "commit", commit.Hash.String(), "error", err)
		branchName = "unknown"
	}
	metadata.Branch = branchName

	return metadata, nil
}

// determineBranchName determines the branch name for a commit
// Returns "detached" if HEAD is in detached state, or the branch name if found
func (ce *commitExtractor) determineBranchName(repo *git.Repository, commitHash plumbing.Hash) (string, error) {
	// Get HEAD reference
	headRef, err := repo.Head()
	if err != nil {
		// If HEAD doesn't exist (empty repo), return "detached"
		if err == plumbing.ErrReferenceNotFound {
			return "detached", nil
		}
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Check if HEAD is a branch reference
	if headRef.Name().IsBranch() {
		// Check if the commit is on the current HEAD branch
		if headRef.Hash() == commitHash {
			return headRef.Name().Short(), nil
		}

		// Commit might be on a different branch, try to find which branch contains it
		branchName, found := ce.findBranchContainingCommit(repo, commitHash)
		if found {
			return branchName, nil
		}

		// If commit is not on any branch, check if HEAD is detached
		// This can happen if we're checking a commit that's not on current branch
		// For now, return the current branch name as fallback
		return headRef.Name().Short(), nil
	}

	// HEAD is not a branch reference (detached HEAD)
	return "detached", nil
}

// findBranchContainingCommit finds which branch contains the given commit
func (ce *commitExtractor) findBranchContainingCommit(repo *git.Repository, commitHash plumbing.Hash) (string, bool) {
	branches, err := repo.Branches()
	if err != nil {
		ce.logger.Debug("failed to get branches", "error", err)
		return "", false
	}
	defer branches.Close()

	var foundBranch string
	found := false

	err = branches.ForEach(func(ref *plumbing.Reference) error {
		// Check if commit is reachable from this branch
		commitIter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
		if err != nil {
			// Skip this branch if we can't get its log
			return nil
		}
		defer commitIter.Close()

		err = commitIter.ForEach(func(c *object.Commit) error {
			if c.Hash == commitHash {
				foundBranch = ref.Name().Short()
				found = true
				return fmt.Errorf("found") // Stop iteration
			}
			return nil
		})

		// If we found the commit, stop iterating branches
		if found {
			return fmt.Errorf("found")
		}

		return nil
	})

	if err != nil && !found {
		// Error occurred but we didn't find the commit
		return "", false
	}

	return foundBranch, found
}

// ExtractDiff extracts commit diff (to be implemented in task 3-5)
func (ce *commitExtractor) ExtractDiff(repo *git.Repository, hash plumbing.Hash) (*Diff, error) {
	// TODO: Implement in task 3-5
	return nil, fmt.Errorf("ExtractDiff not yet implemented")
}

// ExtractCommit extracts complete commit information (metadata + diff)
func (ce *commitExtractor) ExtractCommit(repo *git.Repository, hash plumbing.Hash) (*CommitInfo, error) {
	// Extract metadata
	metadata, err := ce.ExtractMetadata(repo, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to extract metadata: %w", err)
	}

	// Extract diff
	diff, err := ce.ExtractDiff(repo, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to extract diff: %w", err)
	}

	return &CommitInfo{
		Commit: *metadata,
		Diff:   *diff,
	}, nil
}

