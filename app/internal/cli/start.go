package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/stwalsh4118/clio/internal/config"
	"github.com/stwalsh4118/clio/internal/daemon"
)

// handleStart implements the start command logic
func handleStart() error {
	// Load and validate configuration before starting daemon
	// Load() validates configuration automatically, so if it succeeds, config is valid
	_, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Check if daemon is already running
	running, stale, err := daemon.VerifyDaemonRunning()
	if err != nil {
		return fmt.Errorf("failed to check daemon status: %w", err)
	}

	if running {
		pid, _ := daemon.ReadPID()
		return fmt.Errorf("daemon is already running (PID: %d)", pid)
	}

	// If PID file exists but process doesn't, remove stale PID file
	if stale {
		if err := daemon.RemovePIDFile(); err != nil {
			return fmt.Errorf("failed to remove stale PID file: %w", err)
		}
	}

	// Get the current executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks to get actual path
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable symlinks: %w", err)
	}

	// Get absolute path
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute executable path: %w", err)
	}

	// Check if running in dev mode (for Air hot reload)
	isDevMode := os.Getenv("CLIO_DEV") == "true"

	// Create command to run daemon
	cmd := exec.Command(exePath, "daemon")

	// Set environment variables
	env := []string{
		"HOME=" + os.Getenv("HOME"),
		"USER=" + os.Getenv("USER"),
		"CLIO_DAEMON=true",
	}

	// In dev mode, enable console logging
	if isDevMode {
		env = append(env, "CLIO_LOGGING_CONSOLE=true")
		// Don't redirect output - let it go to terminal
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		// Production mode: minimal PATH for security
		env = append(env, "PATH=/usr/bin:/bin")
		// Set up process attributes for daemonization
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true, // Create new session
		}
		// Redirect stdin, stdout, stderr to /dev/null
		devNull, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("failed to open /dev/null: %w", err)
		}
		defer devNull.Close()
		cmd.Stdin = devNull
		cmd.Stdout = devNull
		cmd.Stderr = devNull
	}

	cmd.Env = env

	if isDevMode {
		// In dev mode, run in foreground and wait for it
		// This allows Air to kill the process on hot reload
		fmt.Println("Starting daemon in dev mode (foreground, console logging enabled)...")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("daemon exited with error: %w", err)
		}
		return nil
	}

	// Production mode: Start the daemon process in background
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Detach from parent - don't wait for the process
	// The daemon will write its own PID file

	// Wait briefly for daemon to write PID file, then read actual PID
	// This fixes the race condition where parent reports intermediate PID
	// but daemon writes its own PID to the file
	time.Sleep(100 * time.Millisecond)

	// Try to read the actual daemon PID from file
	daemonPID, err := daemon.ReadPID()
	if err != nil {
		// If PID file not ready yet, fall back to process PID
		// This can happen if daemon is slow to start
		daemonPID = cmd.Process.Pid
	}

	fmt.Printf("Daemon started (PID: %d)\n", daemonPID)
	return nil
}
