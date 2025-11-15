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
```

**Features**:
- Reads from YAML config file at `~/.clio/config.yaml`
- Environment variable support with `CLIO_` prefix
- Default values matching PRD schema
- Automatic home directory expansion (`~` → actual home path)
- Save configuration to file with path normalization
- Path validation with security checks (prevents traversal, validates symlinks)
- Duplicate detection for watched directories

## Planned Infrastructure (from PRD)

The following infrastructure components are planned but not yet implemented:

- Logging systems
- Database connection pooling
- HTTP middleware (if needed)
- Error handling utilities
- Common validation functions

## Rules

- ALWAYS check this document before implementing any infrastructure functionality
- If infrastructure exists, ALWAYS import and use it
- NEVER recreate similar functionality
- When creating or modifying infrastructure, update this document

