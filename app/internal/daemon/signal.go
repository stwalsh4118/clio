package daemon

import (
	"os"
	"os/signal"
	"syscall"
)

// SetupSignalHandlers sets up signal handlers for graceful shutdown.
// It listens for SIGTERM and SIGINT signals and calls the shutdown function when received.
func SetupSignalHandlers(shutdown func()) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigChan
		shutdown()
	}()
}
