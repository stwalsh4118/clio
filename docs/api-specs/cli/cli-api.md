# CLI API

Last Updated: 2025-01-27

## CLI Commands

### Root Command
```bash
clio [flags] [command]
```
- Short: "Capture and analyze development insights"
- Version: 0.1.0

### Subcommands

#### start
```bash
clio start
```
- Short: "Start the monitoring daemon"
- Status: Implemented (task 1-5)
- Creates a background daemon process
- Stores PID in `~/.clio/clio.pid`
- Returns error if daemon is already running
- Handles stale PID files automatically

#### stop
```bash
clio stop
```
- Short: "Stop the monitoring daemon"
- Status: Implemented (task 1-5)
- Reads PID from `~/.clio/clio.pid`
- Verifies process exists and is clio daemon
- Sends SIGTERM for graceful shutdown
- Waits up to 10 seconds for process exit
- Removes PID file after successful shutdown
- Returns error if daemon is not running

#### status
```bash
clio status
```
- Short: "Check daemon status"
- Status: Implemented (task 1-5)
- Checks if PID file exists
- Verifies process is running
- Reports "running" or "stopped" status
- Handles stale PID files automatically

#### config
```bash
clio config [--show] [--add-watch <path>] [--set-blog-repo <path>]
```
- Short: "View and modify configuration"
- Flags:
  - `--show`, `-s`: Display current configuration in YAML format
  - `--add-watch <path>`: Add directory to watched directories list
  - `--set-blog-repo <path>`: Set blog repository path
- Status: Implemented (task 1-4)
- Validates paths and persists changes to `~/.clio/config.yaml`

## Service Interfaces

### CLI Root Command Factory (Go)
```go
func NewRootCmd() *cobra.Command
```
Creates and configures the root Cobra command with all subcommands.

### Command Factories (Go)
```go
func newStartCmd() *cobra.Command
func newStopCmd() *cobra.Command
func newStatusCmd() *cobra.Command
func newConfigCmd() *cobra.Command
func newDaemonCmd() *cobra.Command  // Hidden, internal use only
```
Factory functions that create individual subcommands. All commands are fully implemented. `newDaemonCmd()` is hidden and used internally by `start` command.

### Command Handlers (Go)
```go
func handleStart() error
func handleStop() error
func handleStatus() error
func handleDaemon() error  // Internal use only
```
Handler functions that implement command logic. `handleDaemon()` runs the daemon process and is called internally.

## Constants

```go
const version = "0.1.0"
```
Current CLI version string.

