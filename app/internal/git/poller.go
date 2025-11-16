package git

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stwalsh4118/clio/internal/config"
	"github.com/stwalsh4118/clio/internal/logging"
)

const (
	// defaultPollInterval is the default polling interval if not configured
	defaultPollInterval = 30 * time.Second
	// minPollInterval is the minimum allowed polling interval
	minPollInterval = 1 * time.Second
	// pollResultChanBuffer is the buffer size for the poll results channel
	pollResultChanBuffer = 10
)

// PollerService defines the interface for polling git repositories for new commits
type PollerService interface {
	Start(ctx context.Context, repos []Repository) error
	Stop() error
	PollResults() <-chan PollResult
}

// PollResult represents the result of polling a repository
type PollResult struct {
	Repository Repository
	NewCommits []Commit
	Error      error
}

// poller implements PollerService for polling git repositories
type poller struct {
	config         *config.Config
	logger         logging.Logger
	interval       time.Duration
	ticker         *time.Ticker
	done           chan struct{}
	pollResults    chan PollResult
	started        bool
	mu             sync.Mutex
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
	lastSeenHashes map[string]string // Repository path -> last seen commit hash
	stateMu        sync.RWMutex      // Mutex for lastSeenHashes
}

// NewPollerService creates a new poller service instance
func NewPollerService(cfg *config.Config, logger logging.Logger) (PollerService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	// Create component-specific logger
	componentLogger := logger.With("component", "git_poller")

	// Determine polling interval
	intervalSeconds := cfg.Git.PollIntervalSeconds
	if intervalSeconds < 1 {
		intervalSeconds = int(defaultPollInterval.Seconds())
		componentLogger.Debug("using default polling interval", "interval_seconds", intervalSeconds)
	}
	interval := time.Duration(intervalSeconds) * time.Second

	// Ensure interval is at least minimum
	if interval < minPollInterval {
		interval = minPollInterval
		componentLogger.Warn("polling interval too small, using minimum", "requested_seconds", intervalSeconds, "minimum_seconds", int(minPollInterval.Seconds()))
	}

	return &poller{
		config:         cfg,
		logger:         componentLogger,
		interval:       interval,
		done:           make(chan struct{}),
		pollResults:    make(chan PollResult, pollResultChanBuffer),
		started:        false,
		lastSeenHashes: make(map[string]string),
	}, nil
}

// Start begins polling git repositories for new commits
func (p *poller) Start(ctx context.Context, repos []Repository) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return fmt.Errorf("poller is already started")
	}

	// Create context with cancellation
	p.ctx, p.cancel = context.WithCancel(ctx)

	// Initialize state: get current HEAD hash for each repository
	p.logger.Debug("initializing poller state", "repository_count", len(repos))
	for _, repo := range repos {
		hash, err := p.getCurrentHEADHash(repo.Path)
		if err != nil {
			// Log error but continue - repository might be empty or invalid
			p.logger.Warn("failed to get initial HEAD hash", "repository", repo.Path, "error", err)
			continue
		}
		if hash != "" {
			p.stateMu.Lock()
			p.lastSeenHashes[repo.Path] = hash
			p.stateMu.Unlock()
			p.logger.Debug("initialized repository state", "repository", repo.Path, "hash", hash)
		}
	}

	// Create ticker with configured interval
	p.ticker = time.NewTicker(p.interval)

	// Start polling goroutine
	p.wg.Add(1)
	go p.pollLoop(repos)

	p.started = true
	p.logger.Info("poller started", "interval_seconds", int(p.interval.Seconds()), "repository_count", len(repos))
	return nil
}

// pollLoop runs the polling loop in a separate goroutine
func (p *poller) pollLoop(repos []Repository) {
	defer p.wg.Done()

	p.logger.Debug("polling loop started", "interval_seconds", int(p.interval.Seconds()))

	for {
		select {
		case <-p.ctx.Done():
			p.logger.Debug("polling loop stopped (shutdown requested)")
			return
		case <-p.done:
			p.logger.Debug("polling loop stopped (done signal)")
			return
		case <-p.ticker.C:
			// Perform poll
			p.pollAllRepositories(repos)
		}
	}
}

// pollAllRepositories polls all repositories concurrently
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

// pollRepository polls a single repository for new commits
func (p *poller) pollRepository(repo Repository) {
	// Get current HEAD hash
	currentHash, err := p.getCurrentHEADHash(repo.Path)
	if err != nil {
		// Emit error result
		p.emitResult(PollResult{
			Repository: repo,
			NewCommits: nil,
			Error:      fmt.Errorf("failed to get HEAD hash: %w", err),
		})
		return
	}

	// Handle empty repository (no HEAD)
	if currentHash == "" {
		p.logger.Debug("repository has no HEAD", "repository", repo.Path)
		return
	}

	// Get last seen hash
	p.stateMu.RLock()
	lastSeenHash, hasLastSeen := p.lastSeenHashes[repo.Path]
	p.stateMu.RUnlock()

	// If no last seen hash, this is the first poll - store current hash
	if !hasLastSeen || lastSeenHash == "" {
		p.stateMu.Lock()
		p.lastSeenHashes[repo.Path] = currentHash
		p.stateMu.Unlock()
		p.logger.Debug("first poll for repository, storing HEAD", "repository", repo.Path, "hash", currentHash)
		return
	}

	// Compare hashes
	if currentHash == lastSeenHash {
		// No new commits
		return
	}

	// New commits detected - get commits between last seen and current
	p.logger.Debug("new commits detected", "repository", repo.Path, "last_seen", lastSeenHash, "current", currentHash)
	commits, err := p.getCommitsBetween(repo.Path, lastSeenHash, currentHash)
	if err != nil {
		// Emit error result but don't update last seen hash
		p.emitResult(PollResult{
			Repository: repo,
			NewCommits: nil,
			Error:      fmt.Errorf("failed to get commits: %w", err),
		})
		return
	}

	// Update last seen hash
	p.stateMu.Lock()
	p.lastSeenHashes[repo.Path] = currentHash
	p.stateMu.Unlock()

	// Emit result with new commits
	if len(commits) > 0 {
		p.logger.Info("detected new commits", "repository", repo.Path, "count", len(commits))
		p.emitResult(PollResult{
			Repository: repo,
			NewCommits: commits,
			Error:      nil,
		})
	}
}

// getCurrentHEADHash gets the current HEAD commit hash for a repository
func (p *poller) getCurrentHEADHash(repoPath string) (string, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open repository: %w", err)
	}

	ref, err := repo.Head()
	if err != nil {
		if err == plumbing.ErrReferenceNotFound {
			// Empty repository - no HEAD
			return "", nil
		}
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	return ref.Hash().String(), nil
}

// getCommitsBetween gets all commits between fromHash (exclusive) and toHash (inclusive)
func (p *poller) getCommitsBetween(repoPath, fromHash, toHash string) ([]Commit, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	from := plumbing.NewHash(fromHash)
	to := plumbing.NewHash(toHash)

	// Get HEAD reference for branch name
	headRef, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}
	branchName := headRef.Name().Short()

	// Get commit log starting from toHash
	commitIter, err := repo.Log(&git.LogOptions{From: to})
	if err != nil {
		return nil, fmt.Errorf("failed to get commit log: %w", err)
	}
	defer commitIter.Close()

	var commits []Commit
	foundFrom := false

	// Use a sentinel error to stop iteration
	var stopIteration = errors.New("stop iteration")

	err = commitIter.ForEach(func(c *object.Commit) error {
		// Stop if we've reached the from hash
		if c.Hash == from {
			foundFrom = true
			return stopIteration // Stop iteration
		}

		// Collect parent hashes
		parentHashes := []string{}
		parentCount := 0
		parentIter := c.Parents()
		defer parentIter.Close() // Ensure iterator is closed
		err := parentIter.ForEach(func(parent *object.Commit) error {
			parentHashes = append(parentHashes, parent.Hash.String())
			parentCount++
			return nil
		})
		if err != nil {
			// Log error but continue processing this commit
			p.logger.Debug("failed to iterate parent commits", "commit", c.Hash.String(), "error", err)
		}

		// Convert to Commit type
		commit := Commit{
			Hash:      c.Hash.String(),
			Message:   c.Message,
			Author:    c.Author.Name,
			Email:     c.Author.Email,
			Timestamp: c.Author.When,
			Branch:    branchName,
			IsMerge:   parentCount > 1,
			Parents:   parentHashes,
		}

		commits = append(commits, commit)
		return nil
	})

	// Check if error is our stop iteration sentinel
	if err != nil && !errors.Is(err, stopIteration) {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}

	// If we didn't find the from hash, that's okay - we got all commits up to HEAD
	// This can happen if the repository was reset or rebased
	if !foundFrom && fromHash != "" {
		p.logger.Debug("from hash not found in commit history", "repository", repoPath, "from_hash", fromHash)
	}

	return commits, nil
}

// emitResult emits a poll result to the results channel (non-blocking)
func (p *poller) emitResult(result PollResult) {
	select {
	case p.pollResults <- result:
		// Result sent successfully
	default:
		// Channel full - log warning but don't block
		p.logger.Warn("poll results channel full, dropping result", "repository", result.Repository.Path)
	}
}

// Stop stops polling and cleans up resources
func (p *poller) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started {
		return nil // Already stopped
	}

	p.logger.Info("stopping poller")

	// Cancel context
	if p.cancel != nil {
		p.cancel()
	}

	// Stop ticker
	if p.ticker != nil {
		p.ticker.Stop()
	}

	// Signal shutdown
	close(p.done)

	// Wait for polling goroutine to finish
	p.wg.Wait()

	// Close poll results channel
	close(p.pollResults)

	p.started = false
	p.logger.Info("poller stopped")
	return nil
}

// PollResults returns the channel for receiving poll results
func (p *poller) PollResults() <-chan PollResult {
	return p.pollResults
}
