# E2E Tests for PBI 1

This directory contains end-to-end tests that verify all 12 Conditions of Satisfaction (CoS) from PBI 1: Foundation & CLI Framework.

## Overview

The E2E tests verify the complete CLI framework functionality including:
- Go project structure
- CLI command availability
- Configuration file management
- Daemon lifecycle management
- Error handling
- Configuration validation

## Running the Tests

### Prerequisites

- Go 1.21 or later
- The clio binary must be buildable (tests will build it automatically if needed)

### Running All E2E Tests

From the `app/` directory:

```bash
go test ./test/e2e/... -v
```

### Running Specific Tests

To run a specific CoS test:

```bash
go test ./test/e2e/... -v -run TestCoS1_GoProjectStructure
```

To run tests for a specific group:

```bash
# Test configuration commands (CoS 3-7)
go test ./test/e2e/... -v -run "TestCoS[3-7]"

# Test daemon commands (CoS 8-10)
go test ./test/e2e/... -v -run "TestCoS[8-9]|TestCoS10"
```

## Test Structure

### Test Files

- `pbi1_cos_test.go` - Main test file containing all 12 CoS test cases
- `helpers.go` - Helper functions for test setup, CLI execution, and cleanup

### Test Environment

Each test runs in isolation using:
- Temporary directory for configuration files (set via `HOME` environment variable)
- Automatic cleanup after each test
- Daemon state management (ensures daemon is stopped before tests)

## Test Coverage

### CoS 1: Go Project Structure
- Verifies `go.mod` exists
- Verifies directory structure (cmd/, internal/, pkg/)
- Verifies `main.go` exists

### CoS 2: CLI Commands Exist
- Verifies all base commands are available (start, stop, status, config)
- Verifies each command responds to `--help`

### CoS 3: Config File Creation
- Verifies config file is created on first run
- Verifies default values match PRD schema

### CoS 4: Configuration Persistence
- Verifies configuration changes persist across restarts

### CoS 5: Config Show
- Verifies `clio config --show` displays current configuration
- Verifies output is valid YAML

### CoS 6: Config Add Watch
- Verifies `clio config --add-watch` adds directories
- Verifies duplicate detection works
- Verifies invalid path handling

### CoS 7: Config Set Blog Repo
- Verifies `clio config --set-blog-repo` sets blog repository path
- Verifies invalid path handling

### CoS 8: Start Command
- Verifies `clio start` creates background daemon
- Verifies PID file is created
- Verifies process is running and is clio daemon
- Verifies error when already running

### CoS 9: Stop Command
- Verifies `clio stop` gracefully stops daemon
- Verifies PID file is removed
- Verifies error when not running

### CoS 10: Status Command
- Verifies `clio status` reports correct state when running
- Verifies `clio status` reports correct state when stopped
- Verifies stale PID file handling

### CoS 11: Error Messages
- Verifies helpful error messages for invalid commands
- Verifies helpful error messages for invalid paths
- Verifies helpful error messages for invalid operations

### CoS 12: Configuration Validation
- Verifies validation prevents invalid watched directories
- Verifies validation prevents invalid blog repository paths
- Verifies validation prevents invalid file paths (file instead of directory)

## Troubleshooting

### Tests Fail with "daemon already running"

The tests automatically ensure the daemon is stopped before running, but if you encounter this:

1. Manually stop the daemon: `clio stop`
2. Or kill the process if needed: `pkill -f clio`

### Tests Fail with "binary not found"

The tests automatically build the binary if needed. If building fails:

1. Ensure you're running from the `app/` directory
2. Ensure Go is properly installed: `go version`
3. Try building manually: `go build -o tmp/clio cmd/clio/main.go`

### Tests Leave Temporary Directories

Temporary directories should be cleaned up automatically. If they persist:

1. They are created in the system temp directory (usually `/tmp`)
2. They follow the pattern `clio-e2e-test-*`
3. They can be safely removed manually if needed

## Notes

- Tests use real file system operations (not mocks) for realistic testing
- Tests isolate themselves using temporary directories
- Tests clean up after themselves automatically
- Tests can be run independently or as a suite
- Some tests require the daemon to be stopped before running (handled automatically)

