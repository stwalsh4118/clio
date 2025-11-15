package daemon

import (
	"context"
	"fmt"
	"os"
	"time"
)

const (
	shutdownTimeout = 10 * time.Second
)

// Daemon represents the main daemon process structure.
type Daemon struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// NewDaemon creates a new daemon instance.
func NewDaemon() (*Daemon, error) {
	ctx, cancel := context.WithCancel(context.Background())

	return &Daemon{
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}, nil
}

// Run starts the daemon main loop.
// This is a placeholder implementation that runs indefinitely until shutdown is requested.
// The actual monitoring logic will be implemented in later tasks.
func (d *Daemon) Run() error {
	// Set up signal handlers for graceful shutdown
	SetupSignalHandlers(d.Shutdown)

	// Write PID file
	pid := os.Getpid()
	if err := WritePID(pid); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	// Main daemon loop (placeholder)
	// This will be replaced with actual monitoring logic in future tasks
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			// Shutdown requested
			close(d.done)
			return nil
		case <-ticker.C:
			// Placeholder: daemon is running
			// In future tasks, this will contain actual monitoring logic
		}
	}
}

// Shutdown gracefully shuts down the daemon.
func (d *Daemon) Shutdown() {
	// Cancel context to signal shutdown
	d.cancel()

	// Wait for graceful shutdown with timeout
	select {
	case <-d.done:
		// Shutdown completed
	case <-time.After(shutdownTimeout):
		// Timeout - force exit
		os.Exit(1)
	}

	// Remove PID file
	_ = RemovePIDFile()
}

// Wait waits for the daemon to finish.
func (d *Daemon) Wait() {
	<-d.done
}
