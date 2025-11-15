package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/stwalsh4118/clio/internal/config"
	"github.com/stwalsh4118/clio/internal/cursor"
	"github.com/stwalsh4118/clio/internal/db"
	"github.com/stwalsh4118/clio/internal/logging"
)

const (
	shutdownTimeout = 10 * time.Second
)

// Daemon represents the main daemon process structure.
type Daemon struct {
	ctx            context.Context
	cancel         context.CancelFunc
	done           chan struct{}
	db             *sql.DB
	config         *config.Config
	logger         logging.Logger
	captureService cursor.CaptureService
}

// NewDaemon creates a new daemon instance.
func NewDaemon() (*Daemon, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize database
	database, err := db.Open(cfg)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Initialize logger
	logger, err := logging.NewLogger(cfg)
	if err != nil {
		cancel()
		database.Close()
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Create capture service (may fail if Cursor log path not configured - that's OK)
	captureService, err := cursor.NewCaptureService(cfg, database)
	if err != nil {
		// Log warning but don't fail daemon creation - allows daemon to run without cursor capture
		logger.Warn("failed to create capture service", "error", err)
		captureService = nil
	}

	return &Daemon{
		ctx:            ctx,
		cancel:         cancel,
		done:           make(chan struct{}),
		db:             database,
		config:         cfg,
		logger:         logger,
		captureService: captureService,
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

	d.logger.Info("daemon started", "pid", pid)

	// Start capture service if available
	if d.captureService != nil {
		if err := d.captureService.Start(); err != nil {
			// Log error but don't crash daemon - allows daemon to run without cursor capture
			d.logger.Error("failed to start capture service", "error", err)
		} else {
			d.logger.Info("capture service started")
		}
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
	d.logger.Info("daemon shutdown initiated")

	// Stop capture service if available
	if d.captureService != nil {
		if err := d.captureService.Stop(); err != nil {
			d.logger.Error("failed to stop capture service", "error", err)
		} else {
			d.logger.Info("capture service stopped")
		}
	}

	// Cancel context to signal shutdown
	d.cancel()

	// Wait for graceful shutdown with timeout
	select {
	case <-d.done:
		// Shutdown completed
		d.logger.Info("daemon shutdown completed")
	case <-time.After(shutdownTimeout):
		// Timeout - perform cleanup before force exit
		// Note: os.Exit terminates immediately, so cleanup must happen before it
		d.logger.Warn("daemon shutdown timeout, forcing exit")
		if d.db != nil {
			_ = d.db.Close()
		}
		_ = RemovePIDFile()
		os.Exit(1)
	}

	// Close database connection
	if d.db != nil {
		if err := d.db.Close(); err != nil {
			d.logger.Error("failed to close database", "error", err)
		}
	}

	// Remove PID file
	if err := RemovePIDFile(); err != nil {
		d.logger.Error("failed to remove PID file", "error", err)
	}
}

// Wait waits for the daemon to finish.
func (d *Daemon) Wait() {
	<-d.done
}
