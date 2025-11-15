package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// IsProcessRunning checks if a process with the given PID exists.
func IsProcessRunning(pid int) (bool, error) {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, fmt.Errorf("failed to find process: %w", err)
	}

	// On Unix systems, sending signal 0 checks if the process exists
	// without actually sending a signal
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// If we get an error, the process likely doesn't exist
		// or we don't have permission to signal it
		if err == os.ErrProcessDone {
			return false, nil
		}
		// Check if it's a "process not found" error
		if err.Error() == "no such process" {
			return false, nil
		}
		// Other errors might indicate permission issues, but process exists
		// We'll return true in this case, as the process likely exists
		return true, nil
	}

	return true, nil
}

// IsClioProcess verifies that the process with the given PID is actually
// the clio daemon by checking the executable path.
func IsClioProcess(pid int) (bool, error) {
	// Check if process exists first
	running, err := IsProcessRunning(pid)
	if err != nil {
		return false, err
	}
	if !running {
		return false, nil
	}

	// Get the executable path of the current process
	currentExe, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("failed to get current executable path: %w", err)
	}

	// Resolve symlinks to get the actual path
	currentExe, err = filepath.EvalSymlinks(currentExe)
	if err != nil {
		return false, fmt.Errorf("failed to resolve current executable symlinks: %w", err)
	}

	// Get absolute path
	currentExeAbs, err := filepath.Abs(currentExe)
	if err != nil {
		return false, fmt.Errorf("failed to get absolute path of current executable: %w", err)
	}

	// Read the process's executable path from /proc/<pid>/exe on Linux
	// or use a cross-platform method
	procExePath := fmt.Sprintf("/proc/%d/exe", pid)
	procExe, err := os.Readlink(procExePath)
	if err != nil {
		// If /proc doesn't exist (e.g., on macOS), try alternative method
		// On macOS, we can use lsof or ps, but for simplicity, we'll
		// fall back to just checking if the process exists
		// This is a limitation on macOS, but acceptable for now
		return running, nil
	}

	// Resolve symlinks
	procExe, err = filepath.EvalSymlinks(procExe)
	if err != nil {
		// If symlink resolution fails, compare as-is
		procExeAbs, err := filepath.Abs(procExe)
		if err != nil {
			return running, nil
		}
		return procExeAbs == currentExeAbs, nil
	}

	// Get absolute path
	procExeAbs, err := filepath.Abs(procExe)
	if err != nil {
		return running, nil
	}

	// Compare executable paths
	return procExeAbs == currentExeAbs, nil
}

// SendSignal sends a signal to the process with the given PID.
func SendSignal(pid int, sig os.Signal) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	if err := process.Signal(sig); err != nil {
		return fmt.Errorf("failed to send signal to process: %w", err)
	}

	return nil
}

// WaitForProcessExit waits for a process to exit with a timeout.
// Returns nil if the process exits within the timeout, or an error if it times out.
// Verifies the process PID hasn't been reused (PID reuse attack protection).
func WaitForProcessExit(pid int, timeout time.Duration) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	// Verify it's still the same process before waiting
	// This protects against PID reuse attacks
	isClio, err := IsClioProcess(pid)
	if err == nil && !isClio {
		return fmt.Errorf("process verification failed - PID may have been reused")
	}

	// Channel to signal when process exits
	done := make(chan error, 1)

	// Goroutine to wait for process
	go func() {
		_, err := process.Wait()
		done <- err
	}()

	// Wait for either process exit or timeout
	select {
	case err := <-done:
		if err != nil {
			// Process exited with an error, but that's okay
			// The important thing is it exited
			return nil
		}
		return nil
	case <-time.After(timeout):
		// Before timing out, verify process is still the same
		// This catches PID reuse attacks during wait
		isClio, err := IsClioProcess(pid)
		if err == nil && !isClio {
			return fmt.Errorf("process verification failed during wait - PID may have been reused")
		}
		return fmt.Errorf("process did not exit within %v", timeout)
	}
}

// GetCurrentExecutablePath returns the absolute path of the current executable.
// This is useful for process verification.
func GetCurrentExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("failed to resolve executable symlinks: %w", err)
	}

	// Get absolute path
	absPath, err := filepath.Abs(exe)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	return absPath, nil
}

// VerifyDaemonRunning checks if the daemon is running by verifying:
// 1. PID file exists
// 2. Process with that PID exists
// 3. Process is actually the clio daemon
// Returns (isRunning, isStale, error)
func VerifyDaemonRunning() (bool, bool, error) {
	exists, err := PIDFileExists()
	if err != nil {
		return false, false, err
	}
	if !exists {
		return false, false, nil
	}

	pid, err := ReadPID()
	if err != nil {
		return false, true, nil // PID file exists but can't read it - stale
	}

	running, err := IsProcessRunning(pid)
	if err != nil {
		return false, false, err
	}
	if !running {
		return false, true, nil // Process doesn't exist - stale PID file
	}

	// Verify it's actually clio
	isClio, err := IsClioProcess(pid)
	if err != nil {
		// If we can't verify, assume it's running (better safe than sorry)
		return running, false, nil
	}

	return isClio, false, nil
}
