package cli

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/stwalsh4118/clio/internal/daemon"
)

const (
	stopTimeout = 10 * time.Second
)

// handleStop implements the stop command logic
func handleStop() error {
	// Check if PID file exists
	exists, err := daemon.PIDFileExists()
	if err != nil {
		return fmt.Errorf("failed to check PID file: %w", err)
	}

	if !exists {
		return fmt.Errorf("daemon is not running (PID file not found)")
	}

	// Read PID from file
	pid, err := daemon.ReadPID()
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	// Verify process exists
	running, err := daemon.IsProcessRunning(pid)
	if err != nil {
		return fmt.Errorf("failed to check if process is running: %w", err)
	}

	if !running {
		// Stale PID file - remove it
		if err := daemon.RemovePIDFile(); err != nil {
			return fmt.Errorf("daemon is not running, but failed to remove stale PID file: %w", err)
		}
		return fmt.Errorf("daemon is not running (stale PID file removed)")
	}

	// Verify it's actually the clio daemon
	isClio, err := daemon.IsClioProcess(pid)
	if err != nil {
		// If we can't verify, proceed anyway but warn
		fmt.Fprintf(os.Stderr, "Warning: could not verify process is clio daemon: %v\n", err)
	} else if !isClio {
		return fmt.Errorf("process with PID %d is not the clio daemon", pid)
	}

	// Send SIGTERM for graceful shutdown
	if err := daemon.SendSignal(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send shutdown signal: %w", err)
	}

	fmt.Printf("Shutdown signal sent to daemon (PID: %d), waiting for graceful shutdown...\n", pid)

	// Wait for process to exit
	if err := daemon.WaitForProcessExit(pid, stopTimeout); err != nil {
		return fmt.Errorf("daemon did not exit within %v: %w", stopTimeout, err)
	}

	// Remove PID file
	if err := daemon.RemovePIDFile(); err != nil {
		return fmt.Errorf("daemon stopped, but failed to remove PID file: %w", err)
	}

	fmt.Println("Daemon stopped successfully")
	return nil
}
