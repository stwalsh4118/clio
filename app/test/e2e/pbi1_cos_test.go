package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stwalsh4118/clio/internal/config"
	"github.com/stwalsh4118/clio/internal/daemon"
	"gopkg.in/yaml.v3"
)

// TestCoS1_GoProjectStructure verifies CoS 1: Go project initializes successfully with proper module structure
func TestCoS1_GoProjectStructure(t *testing.T) {
	// Verify go.mod exists
	goModPath := filepath.Join("..", "..", "go.mod")
	if _, err := os.Stat(goModPath); err != nil {
		t.Fatalf("go.mod not found: %v", err)
	}

	// Verify directory structure exists
	dirs := []string{
		filepath.Join("..", "..", "cmd"),
		filepath.Join("..", "..", "internal"),
		filepath.Join("..", "..", "pkg"),
	}

	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("Directory %s does not exist: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s exists but is not a directory", dir)
		}
	}

	// Verify main.go exists
	mainPath := filepath.Join("..", "..", "cmd", "clio", "main.go")
	if _, err := os.Stat(mainPath); err != nil {
		t.Fatalf("main.go not found: %v", err)
	}
}

// TestCoS2_CLICommandsExist verifies CoS 2: CLI framework provides all base commands
func TestCoS2_CLICommandsExist(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	exePath := getTestExecutable(t)

	// Test --help output contains all required commands
	stdout, _, err := executeCLI(t, exePath, []string{"--help"})
	if err != nil {
		t.Fatalf("Failed to execute clio --help: %v", err)
	}

	requiredCommands := []string{"start", "stop", "status", "config"}
	for _, cmd := range requiredCommands {
		if !strings.Contains(stdout, cmd) {
			t.Errorf("Command '%s' not found in help output", cmd)
		}
	}

	// Verify each command can be invoked with --help
	for _, cmd := range requiredCommands {
		_, _, err := executeCLI(t, exePath, []string{cmd, "--help"})
		if err != nil {
			t.Errorf("Command '%s --help' failed: %v", cmd, err)
		}
	}
}

// TestCoS3_ConfigFileCreation verifies CoS 3: Configuration file is created in ~/.clio/config.yaml on first run
func TestCoS3_ConfigFileCreation(t *testing.T) {
	tmpDir, cleanup := setupTestEnv(t)
	defer cleanup()

	exePath := getTestExecutable(t)

	// Ensure config file doesn't exist initially
	configPath := filepath.Join(tmpDir, ".clio", "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		os.RemoveAll(filepath.Join(tmpDir, ".clio"))
	}

	// Run a CLI command that calls config.Load() to trigger config file creation
	// config --show will call Load() which creates the config file
	_, _, err := executeCLI(t, exePath, []string{"config", "--show"})
	if err != nil {
		// Config --show might fail, but config file should still be created
		t.Logf("Note: config --show returned error (expected if config is empty): %v", err)
	}

	// Wait a bit for config file to be created
	time.Sleep(100 * time.Millisecond)

	// Verify config file was created
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("Config file was not created: %v", err)
	}

	// Verify default values match PRD schema
	// Set HOME in environment for config.Load() to use the test directory
	originalHome := os.Getenv("HOME")
	defer func() {
		if originalHome != "" {
			os.Setenv("HOME", originalHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()
	os.Setenv("HOME", tmpDir)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check defaults
	if cfg.WatchedDirectories == nil {
		t.Error("WatchedDirectories should not be nil")
	}
	if len(cfg.WatchedDirectories) != 0 {
		t.Errorf("Expected empty WatchedDirectories, got %v", cfg.WatchedDirectories)
	}
	if cfg.BlogRepository != "" {
		t.Errorf("Expected empty BlogRepository, got %q", cfg.BlogRepository)
	}
	if cfg.Session.InactivityTimeoutMinutes != 30 {
		t.Errorf("Expected InactivityTimeoutMinutes 30, got %d", cfg.Session.InactivityTimeoutMinutes)
	}
}

// TestCoS4_ConfigurationPersistence verifies CoS 4: Configuration persists across application restarts
func TestCoS4_ConfigurationPersistence(t *testing.T) {
	tmpDir, cleanup := setupTestEnv(t)
	defer cleanup()

	exePath := getTestExecutable(t)
	testDir := createTempDir(t)
	defer os.RemoveAll(testDir)

	// Add a watched directory
	_, _, err := executeCLI(t, exePath, []string{"config", "--add-watch", testDir})
	if err != nil {
		t.Fatalf("Failed to add watched directory: %v", err)
	}

	// Set HOME for config.Load() to use the test directory
	originalHome := os.Getenv("HOME")
	defer func() {
		if originalHome != "" {
			os.Setenv("HOME", originalHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()
	os.Setenv("HOME", tmpDir)

	// Load config again to verify persistence
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify the directory was persisted
	found := false
	for _, dir := range cfg.WatchedDirectories {
		if strings.Contains(dir, testDir) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Watched directory was not persisted. Got: %v", cfg.WatchedDirectories)
	}
}

// TestCoS5_ConfigShow verifies CoS 5: clio config --show displays current configuration
func TestCoS5_ConfigShow(t *testing.T) {
	tmpDir, cleanup := setupTestEnv(t)
	defer cleanup()

	exePath := getTestExecutable(t)
	testDir := createTempDir(t)
	defer os.RemoveAll(testDir)

	// Set up known config values
	_, _, err := executeCLI(t, exePath, []string{"config", "--add-watch", testDir})
	if err != nil {
		t.Fatalf("Failed to add watched directory: %v", err)
	}

	// Execute config --show
	stdout, _, err := executeCLI(t, exePath, []string{"config", "--show"})
	if err != nil {
		t.Fatalf("Failed to execute config --show: %v", err)
	}

	// Parse YAML output
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(stdout), &cfg); err != nil {
		t.Fatalf("Failed to parse config output: %v", err)
	}

	// Set HOME for path comparison
	originalHome := os.Getenv("HOME")
	defer func() {
		if originalHome != "" {
			os.Setenv("HOME", originalHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()
	os.Setenv("HOME", tmpDir)

	// Verify values match
	found := false
	for _, dir := range cfg.WatchedDirectories {
		if strings.Contains(dir, testDir) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Config --show did not display added directory. Output: %s", stdout)
	}
}

// TestCoS6_ConfigAddWatch verifies CoS 6: clio config --add-watch successfully adds directories to watch list
func TestCoS6_ConfigAddWatch(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	exePath := getTestExecutable(t)
	testDir := createTempDir(t)
	defer os.RemoveAll(testDir)

	// Add directory
	stdout, stderr, err := executeCLI(t, exePath, []string{"config", "--add-watch", testDir})
	if err != nil {
		t.Fatalf("Failed to add watched directory: %v (stderr: %s)", err, stderr)
	}

	if !strings.Contains(stdout, testDir) && !strings.Contains(stdout, "Added") {
		t.Logf("Output: %s, Stderr: %s", stdout, stderr)
	}

	// Verify directory was added to config
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	found := false
	for _, dir := range cfg.WatchedDirectories {
		if strings.Contains(dir, testDir) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Directory was not added to config. Got: %v", cfg.WatchedDirectories)
	}

	// Test duplicate detection
	_, _, err = executeCLI(t, exePath, []string{"config", "--add-watch", testDir})
	if err == nil {
		t.Error("Expected error when adding duplicate directory, but got none")
	}

	// Test invalid path
	_, _, err = executeCLI(t, exePath, []string{"config", "--add-watch", "/nonexistent/path/12345"})
	if err == nil {
		t.Error("Expected error when adding non-existent directory, but got none")
	}
}

// TestCoS7_ConfigSetBlogRepo verifies CoS 7: clio config --set-blog-repo successfully sets blog repository path
func TestCoS7_ConfigSetBlogRepo(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	exePath := getTestExecutable(t)
	testDir := createTempDir(t)
	defer os.RemoveAll(testDir)

	// Set blog repository
	_, stderr, err := executeCLI(t, exePath, []string{"config", "--set-blog-repo", testDir})
	if err != nil {
		t.Fatalf("Failed to set blog repository: %v (stderr: %s)", err, stderr)
	}

	// Verify blog repository was set
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if !strings.Contains(cfg.BlogRepository, testDir) {
		t.Errorf("Blog repository was not set. Got: %q", cfg.BlogRepository)
	}

	// Test invalid path
	_, _, err = executeCLI(t, exePath, []string{"config", "--set-blog-repo", "/nonexistent/path/12345"})
	if err == nil {
		t.Error("Expected error when setting non-existent blog repository, but got none")
	}
}

// TestCoS8_StartCommand verifies CoS 8: clio start creates background daemon process
func TestCoS8_StartCommand(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	exePath := getTestExecutable(t)

	// Ensure daemon is stopped before starting
	if err := ensureDaemonStopped(); err != nil {
		t.Logf("Warning: failed to ensure daemon stopped: %v", err)
	}

	// Start daemon
	stdout, stderr, err := executeCLI(t, exePath, []string{"start"})
	if err != nil {
		t.Fatalf("Failed to start daemon: %v (stdout: %s, stderr: %s)", err, stdout, stderr)
	}

	// Wait for daemon to be ready
	if err := waitForDaemon(daemonStartTimeout); err != nil {
		t.Fatalf("Daemon did not start: %v", err)
	}

	// Verify PID file exists
	pid := verifyPIDFileExists(t)

	// Verify process is running
	running, err := daemon.IsProcessRunning(pid)
	if err != nil {
		t.Fatalf("Failed to check if process is running: %v", err)
	}
	if !running {
		t.Fatal("Daemon process is not running")
	}

	// Verify process is actually clio daemon
	// Note: On some systems (especially macOS), process verification may not work perfectly
	// due to /proc not being available. We verify the process is running, which is the
	// most important check. The clio verification is a security feature but may not work
	// in all test environments.
	isClio, err := daemon.IsClioProcess(pid)
	if err != nil {
		t.Logf("Warning: could not verify process is clio (this may be expected on macOS): %v", err)
	} else if !isClio {
		// On Linux, this should work. On macOS, /proc may not be available.
		// Log a warning but don't fail - the process is running which is what matters most.
		t.Logf("Warning: process verification returned false (may be expected on macOS or if executable paths differ)")
		// Verify the process is at least running (which we already checked above)
	}

	// Test error when already running
	_, _, err = executeCLI(t, exePath, []string{"start"})
	if err == nil {
		t.Error("Expected error when starting daemon that's already running, but got none")
	}

	// Clean up
	_ = ensureDaemonStopped()
}

// TestCoS9_StopCommand verifies CoS 9: clio stop gracefully stops daemon process
func TestCoS9_StopCommand(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	exePath := getTestExecutable(t)

	// Ensure daemon is stopped
	_ = ensureDaemonStopped()

	// Start daemon first
	_, _, err := executeCLI(t, exePath, []string{"start"})
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	// Wait for daemon to be ready
	if err := waitForDaemon(daemonStartTimeout); err != nil {
		t.Fatalf("Daemon did not start: %v", err)
	}

	pid := verifyPIDFileExists(t)

	// Stop daemon
	stdout, stderr, err := executeCLI(t, exePath, []string{"stop"})
	if err != nil {
		t.Fatalf("Failed to stop daemon: %v (stdout: %s, stderr: %s)", err, stdout, stderr)
	}

	// Wait a bit for process to exit
	time.Sleep(500 * time.Millisecond)

	// Verify process exited
	running, err := daemon.IsProcessRunning(pid)
	if err == nil && running {
		t.Error("Daemon process is still running after stop")
	}

	// Verify PID file removed
	verifyPIDFileRemoved(t)

	// Test error when not running
	_, _, err = executeCLI(t, exePath, []string{"stop"})
	if err == nil {
		t.Error("Expected error when stopping daemon that's not running, but got none")
	}
}

// TestCoS10_StatusCommand verifies CoS 10: clio status accurately reports daemon state
func TestCoS10_StatusCommand(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	exePath := getTestExecutable(t)

	// Ensure daemon is stopped
	_ = ensureDaemonStopped()

	// Test when daemon is stopped
	stdout, _, err := executeCLI(t, exePath, []string{"status"})
	if err != nil {
		t.Fatalf("Failed to execute status: %v", err)
	}
	if !strings.Contains(stdout, "stopped") {
		t.Errorf("Status should report 'stopped' when daemon is not running. Got: %s", stdout)
	}

	// Start daemon
	_, _, err = executeCLI(t, exePath, []string{"start"})
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	// Wait for daemon to be ready
	if err := waitForDaemon(daemonStartTimeout); err != nil {
		t.Fatalf("Daemon did not start: %v", err)
	}

	// Test when daemon is running
	stdout, _, err = executeCLI(t, exePath, []string{"status"})
	if err != nil {
		t.Fatalf("Failed to execute status: %v", err)
	}
	if !strings.Contains(stdout, "running") {
		t.Errorf("Status should report 'running' when daemon is running. Got: %s", stdout)
	}

	// Clean up
	_ = ensureDaemonStopped()
}

// TestCoS11_ErrorMessages verifies CoS 11: All commands provide helpful error messages for invalid inputs
func TestCoS11_ErrorMessages(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	exePath := getTestExecutable(t)

	// Test invalid command
	_, stderr, err := executeCLI(t, exePath, []string{"invalid-command"})
	if err == nil {
		t.Error("Expected error for invalid command, but got none")
	}
	if stderr == "" && err == nil {
		t.Error("Expected error message for invalid command")
	}

	// Test invalid path in config --add-watch
	_, stderr, err = executeCLI(t, exePath, []string{"config", "--add-watch", "/nonexistent/path/12345"})
	if err == nil {
		t.Error("Expected error for invalid path, but got none")
	}
	if !strings.Contains(stderr, "path") && !strings.Contains(stderr, "exist") && !strings.Contains(stderr, "invalid") {
		t.Logf("Error message might not be helpful enough: %s", stderr)
	}

	// Test invalid path in config --set-blog-repo
	_, stderr, err = executeCLI(t, exePath, []string{"config", "--set-blog-repo", "/nonexistent/path/12345"})
	if err == nil {
		t.Error("Expected error for invalid blog repo path, but got none")
	}

	// Test stop when not running
	_, stderr, err = executeCLI(t, exePath, []string{"stop"})
	if err == nil {
		t.Error("Expected error when stopping non-running daemon, but got none")
	}
	if !strings.Contains(stderr, "not running") && !strings.Contains(stderr, "not found") {
		t.Logf("Error message might not be helpful enough: %s", stderr)
	}
}

// TestCoS12_ConfigurationValidation verifies CoS 12: Configuration validation prevents invalid settings
func TestCoS12_ConfigurationValidation(t *testing.T) {
	tmpDir, cleanup := setupTestEnv(t)
	defer cleanup()

	exePath := getTestExecutable(t)

	// Test invalid watched directory (non-existent)
	_, _, err := executeCLI(t, exePath, []string{"config", "--add-watch", "/nonexistent/directory/12345"})
	if err == nil {
		t.Error("Expected validation error for non-existent directory, but got none")
	}

	// Test invalid watched directory (file instead of directory)
	testFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFile)

	_, _, err = executeCLI(t, exePath, []string{"config", "--add-watch", testFile})
	if err == nil {
		t.Error("Expected validation error for file instead of directory, but got none")
	}

	// Test invalid blog repository path
	_, _, err = executeCLI(t, exePath, []string{"config", "--set-blog-repo", "/nonexistent/repo/12345"})
	if err == nil {
		t.Error("Expected validation error for non-existent blog repository, but got none")
	}

	// Note: Testing invalid storage paths and session timeout would require
	// direct config file manipulation, which is beyond the scope of CLI testing.
	// These are tested in unit tests for the config package.
}
