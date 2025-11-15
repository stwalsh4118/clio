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
- Status: Placeholder (task 1-5)
- Returns: Error indicating not yet implemented

#### stop
```bash
clio stop
```
- Short: "Stop the monitoring daemon"
- Status: Placeholder (task 1-5)
- Returns: Error indicating not yet implemented

#### status
```bash
clio status
```
- Short: "Check daemon status"
- Status: Placeholder (task 1-5)
- Returns: Error indicating not yet implemented

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
```
Factory functions that create individual subcommands. `newConfigCmd()` is fully implemented; others are placeholders.

## Constants

```go
const version = "0.1.0"
```
Current CLI version string.

