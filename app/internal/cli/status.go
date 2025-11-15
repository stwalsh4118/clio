package cli

import (
	"fmt"
	"os"

	"github.com/stwalsh4118/clio/internal/daemon"
)

// handleStatus implements the status command logic
func handleStatus() error {
	// Check if daemon is running
	running, stale, err := daemon.VerifyDaemonRunning()
	if err != nil {
		return fmt.Errorf("failed to check daemon status: %w", err)
	}

	if !running {
		if stale {
			// Stale PID file exists - clean it up and report stopped
			if err := daemon.RemovePIDFile(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: found stale PID file but failed to remove it: %v\n", err)
			}
			fmt.Println("Status: stopped (stale PID file removed)")
			return nil
		}
		fmt.Println("Status: stopped")
		return nil
	}

	// Daemon is running - get PID for display
	pid, err := daemon.ReadPID()
	if err != nil {
		// This shouldn't happen if VerifyDaemonRunning returned true
		return fmt.Errorf("daemon appears to be running but failed to read PID: %w", err)
	}

	fmt.Printf("Status: running (PID: %d)\n", pid)
	return nil
}
