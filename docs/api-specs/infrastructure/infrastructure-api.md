# Infrastructure API

Last Updated: 2025-01-27

## Overview

This document catalogs infrastructure components and reusable code that provide cross-cutting concerns across the application. These components should be reused rather than recreated.

## Infrastructure Components

### Configuration Management (Viper)

**Package**: `github.com/stwalsh4118/clio/internal/config`

**Main Function**:
```go
func Load() (*Config, error)
```
Loads configuration from file (`~/.clio/config.yaml`), environment variables (CLIO_ prefix), and defaults. Returns populated Config struct.

**Configuration Types**:
```go
type Config struct {
    WatchedDirectories []string
    BlogRepository     string
    Storage           StorageConfig
    Cursor            CursorConfig
    Session           SessionConfig
}
```

**Additional Functions**:
```go
func Save(cfg *Config) error
func ValidatePath(path string) error
func IsDuplicate(path string, paths []string) bool
func ValidateConfig(cfg *Config) error
func ValidateWatchedDirectories(dirs []string) error
func ValidateBlogRepository(path string) error
func ValidateStoragePaths(storage StorageConfig) error
func ValidateCursorPath(path string) error
func ValidateSessionConfig(session SessionConfig) error
```

**Features**:
- Reads from YAML config file at `~/.clio/config.yaml`
- Environment variable support with `CLIO_` prefix
- Default values matching PRD schema
- Automatic home directory expansion (`~` → actual home path)
- Save configuration to file with path normalization
- Path validation with security checks (prevents traversal, validates symlinks)
- Duplicate detection for watched directories
- Comprehensive configuration validation (paths, values, permissions)
- Security: Watched directories restricted to home directory
- Security: Sensitive system directories blocked from watching
- Validation integrated into loader, CLI commands, and daemon start

### Daemon Process Management

**Package**: `github.com/stwalsh4118/clio/internal/daemon`

**Main Functions**:
```go
func WritePID(pid int) error
func ReadPID() (int, error)
func RemovePIDFile() error
func PIDFileExists() (bool, error)
func IsProcessRunning(pid int) (bool, error)
func IsClioProcess(pid int) (bool, error)
func SendSignal(pid int, sig os.Signal) error
func WaitForProcessExit(pid int, timeout time.Duration) error
func VerifyDaemonRunning() (bool, bool, error)
```

**Daemon Type**:
```go
type Daemon struct {
    ctx    context.Context
    cancel context.CancelFunc
    done   chan struct{}
}

func NewDaemon() (*Daemon, error)
func (d *Daemon) Run() error
func (d *Daemon) Shutdown()
```

**Features**:
- PID file management at `~/.clio/clio.pid` with restrictive permissions (0600)
- Process verification to ensure PID matches clio daemon
- Graceful shutdown handling (SIGTERM/SIGINT)
- Symlink attack protection for PID file paths
- PID reuse attack detection
- Stale PID file detection and cleanup

## Planned Infrastructure (from PRD)

The following infrastructure components are planned but not yet implemented:

- Logging systems
- Database connection pooling
- HTTP middleware (if needed)
- Error handling utilities

## Rules

- ALWAYS check this document before implementing any infrastructure functionality
- If infrastructure exists, ALWAYS import and use it
- NEVER recreate similar functionality
- When creating or modifying infrastructure, update this document

