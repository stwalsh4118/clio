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
clio config
```
- Short: "View and modify configuration"
- Status: Placeholder (task 1-4)
- Returns: Error indicating not yet implemented

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
Factory functions that create individual subcommands. Currently return placeholder commands that error when executed.

## Constants

```go
const version = "0.1.0"
```
Current CLI version string.

