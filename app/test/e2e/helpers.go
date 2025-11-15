package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stwalsh4118/clio/internal/daemon"
)

const (
	daemonStartTimeout = 2 * time.Second
	daemonStopTimeout  = 5 * time.Second
)

// setupTestEnv creates a temporary directory for testing and sets up the environment.
// It returns the temporary directory path and a cleanup function.
// The cleanup function restores the original HOME environment variable and removes the temp directory.
// It creates the test directory in the current working directory to avoid /tmp which is a sensitive directory.
func setupTestEnv(t *testing.T) (string, func()) {
	// Get original HOME and CURSOR_LOG_PATH
	originalHome := os.Getenv("HOME")
	originalCursorLogPath := os.Getenv("CLIO_CURSOR_LOG_PATH")

	// Get current working directory to create test dir there (avoids /tmp)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}

	// Create test directory in current working directory, not in /tmp
	// This avoids the sensitive directory check which rejects /tmp
	testBaseDir := filepath.Join(cwd, "test-e2e-tmp")
	if err := os.MkdirAll(testBaseDir, 0755); err != nil {
		t.Fatalf("Failed to create test base directory: %v", err)
	}

	// Create unique test directory using timestamp
	tmpDir := filepath.Join(testBaseDir, fmt.Sprintf("clio-e2e-test-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}

	// Create temporary cursor log directory (required for validation)
	cursorLogDir := filepath.Join(tmpDir, "cursor-logs")
	if err := os.MkdirAll(cursorLogDir, 0755); err != nil {
		t.Fatalf("Failed to create cursor log directory: %v", err)
	}

	// Set HOME to temp directory
	os.Setenv("HOME", tmpDir)
	// Set cursor log path environment variable (required for validation)
	os.Setenv("CLIO_CURSOR_LOG_PATH", cursorLogDir)

	// Ensure daemon is stopped before starting tests
	if err := ensureDaemonStopped(); err != nil {
		t.Logf("Warning: failed to ensure daemon stopped: %v", err)
	}

	// Cleanup function
	cleanup := func() {
		// Stop daemon if running
		_ = ensureDaemonStopped()

		// Restore original HOME
		if originalHome != "" {
			os.Setenv("HOME", originalHome)
		} else {
			os.Unsetenv("HOME")
		}

		// Restore original CURSOR_LOG_PATH
		if originalCursorLogPath != "" {
			os.Setenv("CLIO_CURSOR_LOG_PATH", originalCursorLogPath)
		} else {
			os.Unsetenv("CLIO_CURSOR_LOG_PATH")
		}

		// Remove temporary directory and test base directory if empty
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("Warning: failed to remove temporary directory %s: %v", tmpDir, err)
		}
		// Try to remove test base directory if it's empty (ignore errors)
		testBaseDir := filepath.Dir(tmpDir)
		_ = os.Remove(testBaseDir)
	}

	return tmpDir, cleanup
}

// getTestExecutable returns the path to the clio binary for testing.
// It builds the binary if it doesn't exist or if the source is newer.
func getTestExecutable(t *testing.T) string {
	// Try to use existing binary first
	exePath := filepath.Join("..", "..", "tmp", "clio")
	if info, err := os.Stat(exePath); err == nil {
		// Check if source is newer than binary
		mainPath := filepath.Join("..", "..", "cmd", "clio", "main.go")
		if mainInfo, err := os.Stat(mainPath); err == nil {
			if mainInfo.ModTime().After(info.ModTime()) {
				// Source is newer, rebuild
				t.Logf("Source is newer than binary, rebuilding...")
			} else {
				// Binary is up to date
				return exePath
			}
		}
	}

	// Build the binary
	t.Logf("Building clio binary for testing...")
	cmd := exec.Command("go", "build", "-o", exePath, filepath.Join("..", "..", "cmd", "clio", "main.go"))
	cmd.Dir = filepath.Join("..", "..")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build clio binary: %v", err)
	}

	return exePath
}

// executeCLI executes a CLI command and returns stdout, stderr, and error.
// It uses the test executable and sets HOME and CLIO_CURSOR_LOG_PATH to the test environment.
func executeCLI(t *testing.T, exePath string, args []string) (string, string, error) {
	cmd := exec.Command(exePath, args...)

	// Set HOME and CLIO_CURSOR_LOG_PATH to current test environment
	cmd.Env = append(os.Environ(), "HOME="+os.Getenv("HOME"))
	if cursorLogPath := os.Getenv("CLIO_CURSOR_LOG_PATH"); cursorLogPath != "" {
		cmd.Env = append(cmd.Env, "CLIO_CURSOR_LOG_PATH="+cursorLogPath)
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// createTempDir creates a temporary directory for test data within the test HOME directory.
// This ensures paths are within the home directory for validation purposes.
// It creates the directory directly in HOME to avoid /tmp which is a sensitive directory.
func createTempDir(t *testing.T) string {
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		t.Fatalf("HOME environment variable not set")
	}

	// Create a test directory directly in HOME, not using MkdirTemp which might use /tmp
	testBaseDir := filepath.Join(homeDir, "test-dirs")
	if err := os.MkdirAll(testBaseDir, 0755); err != nil {
		t.Fatalf("Failed to create test base directory: %v", err)
	}

	// Create a unique subdirectory using a timestamp-based name
	testDir := filepath.Join(testBaseDir, fmt.Sprintf("test-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	return testDir
}

// waitForDaemon waits for the daemon to be ready by checking if PID file exists.
func waitForDaemon(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		exists, err := daemon.PIDFileExists()
		if err != nil {
			return fmt.Errorf("failed to check PID file: %w", err)
		}
		if exists {
			// Verify process is actually running
			pid, err := daemon.ReadPID()
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			running, err := daemon.IsProcessRunning(pid)
			if err == nil && running {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not start within %v", timeout)
}

// ensureDaemonStopped ensures the daemon is stopped.
// It attempts to stop the daemon gracefully if it's running.
func ensureDaemonStopped() error {
	running, stale, err := daemon.VerifyDaemonRunning()
	if err != nil {
		return fmt.Errorf("failed to check daemon status: %w", err)
	}

	if !running {
		if stale {
			// Remove stale PID file
			_ = daemon.RemovePIDFile()
		}
		return nil
	}

	// Daemon is running, try to stop it
	pid, err := daemon.ReadPID()
	if err != nil {
		return fmt.Errorf("failed to read PID: %w", err)
	}

	// Try to stop gracefully
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	cmd := exec.Command(exePath, "stop")
	cmd.Env = append(os.Environ(), "HOME="+os.Getenv("HOME"))
	_ = cmd.Run() // Ignore errors, just try to stop

	// Wait for process to exit
	deadline := time.Now().Add(daemonStopTimeout)
	for time.Now().Before(deadline) {
		running, err := daemon.IsProcessRunning(pid)
		if err != nil || !running {
			_ = daemon.RemovePIDFile()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Force kill if still running
	running, _ = daemon.IsProcessRunning(pid)
	if running {
		process, _ := os.FindProcess(pid)
		_ = process.Kill()
		time.Sleep(500 * time.Millisecond)
		_ = daemon.RemovePIDFile()
	}

	return nil
}

// readConfigFile reads and parses the config file from the test environment.
func readConfigFile(t *testing.T) map[string]interface{} {
	homeDir := os.Getenv("HOME")
	configPath := filepath.Join(homeDir, ".clio", "config.yaml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	// Simple YAML parsing - for full parsing we'd use yaml.v3
	// For now, just check file exists and has content
	_ = data
	return make(map[string]interface{})
}

// verifyPIDFileExists verifies that the PID file exists and contains a valid PID.
func verifyPIDFileExists(t *testing.T) int {
	exists, err := daemon.PIDFileExists()
	if err != nil {
		t.Fatalf("Failed to check PID file: %v", err)
	}
	if !exists {
		t.Fatal("PID file does not exist")
	}

	pid, err := daemon.ReadPID()
	if err != nil {
		t.Fatalf("Failed to read PID: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("Invalid PID: %d", pid)
	}

	return pid
}

// verifyPIDFileRemoved verifies that the PID file has been removed.
func verifyPIDFileRemoved(t *testing.T) {
	exists, err := daemon.PIDFileExists()
	if err != nil {
		t.Fatalf("Failed to check PID file: %v", err)
	}
	if exists {
		t.Fatal("PID file still exists after stop")
	}
}
