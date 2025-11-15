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
    Logging           LoggingConfig
}
```

**Additional Functions**:
```go
func Save(cfg *Config) error
func EnsureConfigFile() error
func EnsureConfigDirectory() error
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
- Automatic config file creation on first run with sensible defaults
- Automatic home directory expansion (`~` → actual home path)
- Save configuration to file with path normalization
- Path validation with security checks (prevents traversal, validates symlinks)
- Duplicate detection for watched directories
- Comprehensive configuration validation (paths, values, permissions)
- Security: Watched directories restricted to home directory
- Security: Sensitive system directories blocked from watching
- Security: Symlink attack protection for config directory/file creation
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
    db     *sql.DB
    config *config.Config
}

func NewDaemon() (*Daemon, error)
func (d *Daemon) Run() error
func (d *Daemon) Shutdown()
```

**Database Initialization**:
- Database is initialized automatically when daemon is created
- Migrations are run automatically on daemon startup
- Database connection is closed gracefully on shutdown

**Features**:
- PID file management at `~/.clio/clio.pid` with restrictive permissions (0600)
- Process verification to ensure PID matches clio daemon
- Graceful shutdown handling (SIGTERM/SIGINT)
- Symlink attack protection for PID file paths
- PID reuse attack detection
- Stale PID file detection and cleanup

### Database Management

**Package**: `github.com/stwalsh4118/clio/internal/db`

**Main Function**:
```go
func Open(cfg *config.Config) (*sql.DB, error)
```
Opens a SQLite database connection at the configured path, ensures the directory exists, runs migrations, and returns the database connection.

**Migration Functions**:
```go
func RunMigrations(db *sql.DB) error
```
Runs all pending database migrations by reading SQL files from embed.FS and executing them directly.

```go
func RollbackMigrations(db *sql.DB, count int) (int, error)
```
Rolls back the specified number of migrations (default: 1). Returns the version after rollback. Requires `.down.sql` files for each migration.

**Features**:
- Automatic database initialization and migration on startup
- Uses WAL mode for better concurrency
- Migration files stored in `internal/db/migrations/` directory
- Migrations embedded in binary using `embed.FS`
- Works with any database/sql driver (pure Go, no CGO required)
- Migrations are idempotent (safe to run multiple times)
- Each migration runs in a transaction (all-or-nothing)
- Tracks migration versions in `schema_migrations` table
- Supports rollback via `RollbackMigrations()` function
- Both `.up.sql` and `.down.sql` files are loaded and available

**Usage Pattern**:
1. Call `db.Open(cfg)` to get database connection
2. Database is automatically migrated
3. Pass database connection to components that need it (e.g., SessionManager)
4. Close database connection on shutdown

### Logging System (Zerolog)

**Package**: `github.com/stwalsh4118/clio/internal/logging`

**Main Function**:
```go
func NewLogger(cfg *config.Config) (Logger, error)
```
Creates a new logger instance based on configuration. Supports file and console output, configurable log levels, and structured logging.

**Logger Interface**:
```go
type Logger interface {
    Debug(msg string, fields ...interface{})
    Info(msg string, fields ...interface{})
    Warn(msg string, fields ...interface{})
    Error(msg string, fields ...interface{})
    With(fields ...interface{}) Logger
    WithContext(ctx context.Context) Logger
}
```

**Configuration**:
```go
type LoggingConfig struct {
    Level      string // "debug", "info", "warn", "error" (default: "info")
    FilePath   string // Path to log file (default: ~/.clio/clio.log)
    Console    bool   // Also log to console (default: false for daemon, true for CLI)
    MaxSize    int    // Max log file size in MB before rotation (default: 10)
    MaxBackups int    // Number of rotated log files to keep (default: 3)
}
```

**Features**:
- Structured JSON logging using zerolog (zero-allocation, high performance)
- File-based logging for daemon processes (default: `~/.clio/clio.log`)
- Console output support for CLI commands
- Configurable log levels (debug, info, warn, error)
- Secure log file permissions (0600 for files, 0700 for directories)
- Automatic log directory creation
- Component-specific logging via `With()` method
- Context-aware logging via `WithContext()`

**Usage Pattern**:
```go
// Create logger
logger, err := logging.NewLogger(cfg)
if err != nil {
    return fmt.Errorf("failed to initialize logger: %w", err)
}

// Basic logging
logger.Info("daemon started", "pid", os.Getpid())
logger.Error("failed to store conversation", "error", err, "session_id", sessionID)

// Component-specific logger
componentLogger := logger.With("component", "storage")
componentLogger.Debug("storing conversation", "composer_id", composerID)

// Context-aware logging
ctxLogger := logger.WithContext(ctx)
ctxLogger.Info("processing request")
```

**Log Format**:
- JSON format for structured logging
- Includes timestamp, level, message, and custom fields
- Example: `{"level":"info","time":"2025-01-27T10:30:00Z","component":"session_manager","message":"created new session","session_id":"abc-123","project":"my-project"}`

**Log Levels**:
- **Debug**: Detailed operation flow, file paths, parsing details (development only)
- **Info**: Important events (daemon start/stop, session creation, major operations)
- **Warn**: Recoverable issues (skipped files, format variations, retries)
- **Error**: Failures that need attention (database errors, file I/O failures)

**Default Behavior**:
- Daemon mode: File logging only (no console)
- CLI commands: Console + file logging (for immediate feedback)
- Log file: `~/.clio/clio.log` with 0600 permissions
- Log level: `info` (can be overridden via config)

## Planned Infrastructure (from PRD)

The following infrastructure components are planned but not yet implemented:

- HTTP middleware (if needed)
- Error handling utilities

## Rules

- ALWAYS check this document before implementing any infrastructure functionality
- If infrastructure exists, ALWAYS import and use it
- NEVER recreate similar functionality
- When creating or modifying infrastructure, update this document

