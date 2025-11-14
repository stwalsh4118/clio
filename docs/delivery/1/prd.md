# PBI-1: Foundation & CLI Framework

[View in Backlog](../backlog.md#user-content-1)

## Overview

Establish the foundational infrastructure for the insight capture system, including Go project setup, CLI framework, configuration management, and basic operational commands. This PBI provides the base upon which all other components will be built.

## Problem Statement

Before we can capture insights, we need a working application with a CLI interface, configuration management, and basic operational capabilities. Without this foundation, we cannot build the monitoring, storage, or analysis features.

## User Stories

**As a developer**, I want to install and configure the insightd tool so that I can begin using it to capture my development insights.

**As a developer**, I want to start and stop the monitoring service via CLI commands so that I can control when data capture is active.

**As a developer**, I want to configure which directories to monitor so that I can exclude personal projects from professional blog content.

**As a developer**, I want configuration to persist across restarts so that I don't have to reconfigure the system each time.

## Technical Approach

### Components

**1. Go Project Structure**
- Initialize Go module
- Set up directory structure (cmd/, internal/, pkg/)
- Define package boundaries
- Set up build configuration

**2. CLI Framework (Cobra)**
- Root command structure
- Subcommands: start, stop, status, config, analyze, list, search, show
- Command-line flag parsing
- Help text and documentation

**3. Configuration Management (Viper)**
- Configuration file location: `~/.insightd/config.yaml`
- Environment variable support
- Default configuration values
- Configuration validation

**4. Basic Commands**
- `insightd start` - Start background monitoring daemon
- `insightd stop` - Stop monitoring daemon
- `insightd status` - Check if daemon is running
- `insightd config` - View and modify configuration
  - `insightd config --show` - Display current configuration
  - `insightd config --add-watch <path>` - Add directory to watch list
  - `insightd config --set-blog-repo <path>` - Set blog repository path

**5. File Structure Setup**
- Create `~/.insightd/` directory structure
- Initialize configuration file with defaults
- Set up session storage directories

### Configuration Schema

```yaml
# ~/.insightd/config.yaml
watched_directories:
  - ~/projects/stream-tv
  - ~/projects/work-project

blog_repository: ~/repos/blog

storage:
  base_path: ~/.insightd
  sessions_path: ~/.insightd/sessions
  database_path: ~/.insightd/insightd.db

cursor:
  log_path: ~/.cursor

session:
  inactivity_timeout_minutes: 30
```

## UX/UI Considerations

### CLI Commands

```bash
# Installation
go install github.com/user/insightd@latest

# Initial setup (creates config file)
insightd config --init

# Basic operations
insightd start                    # Start monitoring daemon
insightd stop                     # Stop monitoring
insightd status                   # Check if running

# Configuration
insightd config --show            # Display current config
insightd config --add-watch ~/projects/my-project
insightd config --set-blog-repo ~/repos/blog
```

### Error Messages

- Clear error messages for missing configuration
- Helpful suggestions for common issues
- Validation errors with actionable guidance

## Acceptance Criteria

### Must Have

1. Go project initializes successfully with proper module structure
2. CLI framework provides all base commands (start, stop, status, config)
3. Configuration file is created in `~/.insightd/config.yaml` on first run
4. Configuration persists across application restarts
5. `insightd config --show` displays current configuration
6. `insightd config --add-watch` successfully adds directories to watch list
7. `insightd config --set-blog-repo` successfully sets blog repository path
8. `insightd start` creates background daemon process
9. `insightd stop` gracefully stops daemon process
10. `insightd status` accurately reports daemon state
11. All commands provide helpful error messages for invalid inputs
12. Configuration validation prevents invalid settings

## Dependencies

### External Dependencies

- Go 1.21+
- Git (for version control of project itself)

### Go Libraries

- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration management

### System Requirements

- macOS/Linux
- Write access to home directory for configuration storage

## Open Questions

1. Should we support Windows from the start, or defer to later?
2. What should the default watched directories be? Empty list or prompt user?
3. Should we validate that watched directories exist and are git repositories?
4. How should we handle daemon process management? Systemd? Launchd? Simple background process?

## Related Tasks

Tasks will be created in the tasks.md file following the project policy. Initial task breakdown will include:

- Initialize Go module and project structure
- Set up Cobra CLI framework with root command
- Implement configuration management with Viper
- Create config command with subcommands
- Implement start/stop/status commands (daemon management)
- Add configuration validation
- Create default configuration file on first run
- Write installation and setup documentation
