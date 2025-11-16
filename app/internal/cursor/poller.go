package cursor

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stwalsh4118/clio/internal/config"
	"github.com/stwalsh4118/clio/internal/logging"
)

// PollerService defines the interface for polling conversation updates
type PollerService interface {
	Start() error
	Stop() error
	Poll() (<-chan struct{}, error) // Returns channel that receives on each poll
}

// poller implements PollerService for polling Cursor database updates
type poller struct {
	config    *config.Config
	updater   ConversationUpdater
	interval  time.Duration
	ticker    *time.Ticker
	done      chan struct{}
	pollChan  chan struct{}
	started   bool
	mu        sync.Mutex
	logger    logging.Logger
	wg        sync.WaitGroup
	pollCount int64 // Track number of polls for periodic logging
}

const (
	// defaultPollInterval is the default polling interval if not configured
	defaultPollInterval = 7 * time.Second
	// minPollInterval is the minimum allowed polling interval
	minPollInterval = 1 * time.Second
)

// NewPoller creates a new poller instance
func NewPoller(cfg *config.Config, updater ConversationUpdater) (PollerService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if updater == nil {
		return nil, fmt.Errorf("updater cannot be nil")
	}

	// Create logger
	logger, err := logging.NewLogger(cfg)
	if err != nil {
		// If logger creation fails, use no-op logger (don't fail poller creation)
		logger = logging.NewNoopLogger()
	}
	logger = logger.With("component", "poller")

	// Determine polling interval
	intervalSeconds := cfg.Cursor.PollIntervalSeconds
	if intervalSeconds < 1 {
		intervalSeconds = int(defaultPollInterval.Seconds())
		logger.Debug("using default polling interval", "interval_seconds", intervalSeconds)
	}
	interval := time.Duration(intervalSeconds) * time.Second

	// Ensure interval is at least minimum
	if interval < minPollInterval {
		interval = minPollInterval
		logger.Warn("polling interval too small, using minimum", "requested_seconds", intervalSeconds, "minimum_seconds", int(minPollInterval.Seconds()))
	}

	return &poller{
		config:   cfg,
		updater:  updater,
		interval: interval,
		done:     make(chan struct{}),
		pollChan: make(chan struct{}, 1), // Buffered channel to prevent blocking
		started:  false,
		logger:   logger,
	}, nil
}

// Start begins polling for conversation updates
func (p *poller) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return fmt.Errorf("poller is already started")
	}

	// Create ticker with configured interval
	p.ticker = time.NewTicker(p.interval)

	// Start polling goroutine
	p.wg.Add(1)
	go p.pollLoop()

	p.started = true
	p.logger.Info("poller started", "interval_seconds", int(p.interval.Seconds()))
	return nil
}

// pollLoop runs the polling loop in a separate goroutine
func (p *poller) pollLoop() {
	defer p.wg.Done()

	p.logger.Debug("polling loop started", "interval_seconds", int(p.interval.Seconds()))

	for {
		select {
		case <-p.done:
			p.logger.Debug("polling loop stopped (shutdown requested)")
			return
		case <-p.ticker.C:
			// Perform poll
			p.performPoll()
		}
	}
}

// performPoll performs a single poll operation
func (p *poller) performPoll() {
	pollNum := atomic.AddInt64(&p.pollCount, 1)
	p.logger.Debug("performing poll", "poll_number", pollNum)

	// Call DetectUpdatedComposers to check for updates
	updatedComposers, err := p.updater.DetectUpdatedComposers()
	if err != nil {
		// Log error but continue polling (graceful degradation)
		p.logger.Error("failed to detect updated composers during poll", "error", err)
		return
	}

	// Log poll results - INFO when updates found, DEBUG otherwise
	// Also log periodically (every 10 polls) at INFO level to show polling is active
	if len(updatedComposers) > 0 {
		p.logger.Info("poll completed - detected updated conversations", "updated_count", len(updatedComposers))
	} else if pollNum%10 == 0 {
		// Log every 10th poll at INFO level to show polling is working
		p.logger.Info("poll completed - no updates detected", "poll_number", pollNum)
	} else {
		p.logger.Debug("poll completed - no updates detected")
	}

	// Send poll signal (non-blocking due to buffered channel)
	select {
	case p.pollChan <- struct{}{}:
		p.logger.Debug("poll signal sent")
	default:
		// Channel full - log warning but don't block
		p.logger.Warn("poll channel full, dropping poll signal")
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

	// Stop ticker
	if p.ticker != nil {
		p.ticker.Stop()
	}

	// Signal shutdown
	close(p.done)

	// Wait for polling goroutine to finish
	p.wg.Wait()

	// Close poll channel
	close(p.pollChan)

	p.started = false
	p.logger.Info("poller stopped")
	return nil
}

// Poll returns the channel for receiving poll signals
func (p *poller) Poll() (<-chan struct{}, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started {
		return nil, fmt.Errorf("poller is not started")
	}

	return p.pollChan, nil
}
