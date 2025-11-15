package cli

import (
	"fmt"

	"github.com/stwalsh4118/clio/internal/daemon"
)

// handleDaemon runs the daemon process.
// This is called internally when the daemon is started via "clio start".
func handleDaemon() error {
	d, err := daemon.NewDaemon()
	if err != nil {
		return fmt.Errorf("failed to create daemon: %w", err)
	}

	// Run the daemon (this blocks until shutdown)
	if err := d.Run(); err != nil {
		return fmt.Errorf("daemon error: %w", err)
	}

	return nil
}
