# Clio

Capture and analyze development insights from Cursor conversations and git activity.

## Overview

Clio is a Go-based CLI tool that automatically monitors and captures your development workflow, including:
- Cursor AI conversations
- Git commit activity
- Session tracking and organization

The captured data is stored in a queryable format and can be analyzed to extract insights for blog content and technical writing.

## Project Structure

This is a monorepo with the following structure:
- `app/` - Go CLI application (this is where the main codebase lives)
- `docs/` - Project documentation, PBIs, and task definitions

## Installation

### Prerequisites

- Go 1.21 or later
- Git
- macOS/Linux (Windows support coming later)

### Building from Source

```bash
cd app
go build ./cmd/clio
```

The binary will be created in the `app/` directory.

## Current Status

This project is in early development. The foundation (Go module structure, CLI framework, configuration management) is being established. CLI commands and features will be added in subsequent development phases.

## Development

See `docs/delivery/` for Product Backlog Items (PBIs) and task definitions.

## License

[To be determined]

