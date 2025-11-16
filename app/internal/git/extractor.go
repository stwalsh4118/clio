package git

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stwalsh4118/clio/internal/logging"
)

const (
	// MaxDiffLines is the maximum number of lines to include in a diff before truncating
	MaxDiffLines = 5000
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
	Diff   Diff           // Commit diff
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

// ExtractDiff extracts commit diff from a git commit
func (ce *commitExtractor) ExtractDiff(repo *git.Repository, hash plumbing.Hash) (*Diff, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository cannot be nil")
	}

	// Get commit object
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit object: %w", err)
	}

	// Generate patch
	var patch *object.Patch
	parentIter := commit.Parents()
	defer parentIter.Close()

	// Get first parent for merge commits, or use empty tree for initial commits
	parent, err := parentIter.Next()
	if err != nil {
		// Check if this is an initial commit (no parent)
		// ErrParentNotFound or io.EOF both indicate no parent
		if err == object.ErrParentNotFound || errors.Is(err, io.EOF) {
			// Initial commit - compare commit tree with empty tree
			commitTree, err := commit.Tree()
			if err != nil {
				return nil, fmt.Errorf("failed to get commit tree: %w", err)
			}
			// Use DiffTree to compare with empty tree (nil = empty tree)
			changes, err := object.DiffTree(nil, commitTree)
			if err != nil {
				return nil, fmt.Errorf("failed to diff trees for initial commit: %w", err)
			}
			patch, err = changes.Patch()
			if err != nil {
				return nil, fmt.Errorf("failed to generate patch for initial commit: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to get parent commit: %w", err)
		}
	} else {
		// Normal commit or merge commit (use first parent)
		patch, err = parent.Patch(commit)
		if err != nil {
			return nil, fmt.Errorf("failed to generate patch: %w", err)
		}
	}

	// Extract full diff string
	fullDiff := patch.String()

	// Extract file-level statistics
	files := []FileChange{}
	for _, filePatch := range patch.FilePatches() {
		from, to := filePatch.Files()

		// Determine file path (prefer 'to' path, fallback to 'from' path)
		var filePath string
		if to != nil {
			filePath = to.Path()
		} else if from != nil {
			filePath = from.Path()
		} else {
			// Skip if both are nil (shouldn't happen, but be safe)
			ce.logger.Debug("skipping file patch with nil files", "commit", commit.Hash.String())
			continue
		}

		// Count additions and deletions from chunks
		// Chunk types: 0=Equal, 1=Add, 2=Delete
		additions := 0
		deletions := 0
		for _, chunk := range filePatch.Chunks() {
			chunkType := chunk.Type()
			content := chunk.Content()
			lines := strings.Split(content, "\n")

			// Count non-empty lines (last line might be empty if content ends with newline)
			lineCount := len(lines)
			if lineCount > 0 && lines[lineCount-1] == "" {
				lineCount--
			}

			if chunkType == 1 { // Add
				additions += lineCount
			} else if chunkType == 2 { // Delete
				deletions += lineCount
			}
		}

		files = append(files, FileChange{
			Path:      filePath,
			Additions: additions,
			Deletions: deletions,
		})
	}

	// Handle large diffs - truncate if necessary
	diffLines := strings.Split(fullDiff, "\n")
	totalLines := len(diffLines)
	truncated := false
	shownLines := totalLines
	content := fullDiff

	if totalLines > MaxDiffLines {
		truncated = true
		shownLines = MaxDiffLines
		// Truncate diff content but keep file statistics
		truncatedLines := diffLines[:MaxDiffLines]
		truncationNote := fmt.Sprintf("\n\n[Diff truncated: %d lines total, showing first %d lines]", totalLines, MaxDiffLines)
		content = strings.Join(truncatedLines, "\n") + truncationNote

		ce.logger.Info("truncated large diff", "commit", commit.Hash.String(), "total_lines", totalLines, "shown_lines", shownLines)
	}

	return &Diff{
		Content:    content,
		Files:      files,
		Truncated:  truncated,
		TotalLines: totalLines,
		ShownLines: shownLines,
	}, nil
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
