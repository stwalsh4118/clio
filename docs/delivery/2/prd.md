# PBI-2: Cursor Conversation Capture

[View in Backlog](../backlog.md#user-content-2)

## Overview

Implement automatic monitoring and capture of Cursor AI conversation logs, parsing them and storing them in a SQLite database organized by date and project. This component enables the system to track all developer-AI interactions for later analysis.

## Problem Statement

Cursor AI conversations contain valuable insights about problem-solving approaches, debugging strategies, and technical decisions. However, these conversations are stored in Cursor's internal format and are not easily accessible for analysis or blog post generation. We need to automatically capture and convert these conversations into a structured, readable format.

## User Stories

**As a developer**, I want the system to automatically capture my Cursor conversations so that I have a complete record of my problem-solving process without manual intervention.

**As a developer**, I want captured conversations to be organized by date and project so that I can easily find relevant sessions later.

**As a developer**, I want conversations stored in a database so that they can be efficiently queried and analyzed.

## Technical Approach

### Components

**1. Cursor Log Path Configuration**
- User configures Cursor User directory path via `cursor.log_path` in config
- Path points to directory containing `globalStorage/` and `workspaceStorage/` subdirectories
- Example: `~/.config/Cursor/User/` (Linux) or `~/Library/Application Support/Cursor/User/` (macOS)
- Path is validated to exist, be a directory, and be readable

**2. File System Watcher**
- Monitor `{cursor.log_path}/globalStorage/state.vscdb` database file for modifications
- Watch for file modifications (indicates new or updated conversations)
- Handle file system events efficiently

**3. Conversation Parser**
- Query SQLite database: `{cursor.log_path}/globalStorage/state.vscdb`
- Extract conversation messages from `cursorDiskKV` table
- Query composer data: `composerData:{composerId}`
- Query message bubbles: `bubbleId:{composerId}:{bubbleId}`
- Identify user messages (type=1) vs agent responses (type=2)
- Extract metadata: timestamps, composer IDs, conversation status

**4. Session Tracking**
- Group related conversations into sessions
- Determine session boundaries (time-based or manual)
- Track session metadata: start time, end time, project, duration

**5. Database Storage**
- Store conversations directly in SQLite database
- Store session metadata in database
- Store individual messages with proper relationships
- Enable efficient querying and retrieval

### Data Flow

```
{cursor.log_path}/globalStorage/state.vscdb (SQLite Database)
    ↓
File System Watcher (fsnotify) detects modification
    ↓
Parser Queries Database for Updated Composer IDs
    ↓
Parser Extracts Conversation Messages
    ↓
Project Detector Maps Composer ID → Project (via workspaceStorage)
    ↓
Session Tracker Groups Messages by Project & Time
    ↓
Database Storage Persists Sessions & Conversations
    ↓
~/.clio/clio.db (SQLite Database)
```

## UX/UI Considerations

### Background Operation

- Runs silently in background when `clio start` is executed
- No user interaction required during capture
- Logs capture activity to system log or file

### Error Handling

- Gracefully handle Cursor log format changes
- Skip corrupted log files with warning
- Continue monitoring even if individual conversations fail to parse

## Acceptance Criteria

### Must Have

1. System successfully locates Cursor log directory on macOS and Linux
2. File system watcher detects new conversation files in Cursor log directory
3. Parser successfully extracts messages from Cursor log format
4. System correctly identifies user messages vs agent responses
5. Conversations are stored in database with proper organization by date and project
6. Session boundaries are correctly determined (time-based or manual)
7. System handles file system events without blocking
8. System continues operating stably for 8+ hour sessions
9. Multiple concurrent conversations are handled correctly
10. System gracefully handles Cursor log format variations
11. Database storage enables efficient querying and retrieval of conversations

## Dependencies

### External Dependencies

- **PBI-1**: Foundation & CLI Framework (for CLI commands and configuration)
- Cursor IDE installed and generating logs

### Go Libraries

- `github.com/fsnotify/fsnotify` - File system watching

### System Requirements

- Cursor IDE installed
- Read access to Cursor log directory

## Open Questions

1. **Cursor Log Format**: What is the exact format and location of Cursor conversation logs? Need to investigate Cursor's storage mechanism.
2. **Session Boundaries**: How do we determine when a development session ends?
   - Time-based (30 minutes of inactivity)?
   - Manual `clio end-session`?
   - Per-project tracking?
3. **Project Detection**: How do we determine which project a conversation belongs to?
   - Parse from conversation content?
   - Track active Cursor workspace?
   - User configuration?
4. **Conversation Updates**: How do we handle conversations that are updated after initial capture?
5. **Privacy**: Should we have options to exclude certain conversations or redact sensitive data?

## Related Tasks

Tasks will be created in the tasks.md file following the project policy. Initial task breakdown will include:

- Research Cursor log format and storage location
- Implement file system watcher for Cursor log directory
- Design conversation parser for Cursor log format
- Implement session tracking logic
- Implement database storage for conversations
- Add project detection mechanism
- Handle conversation updates and modifications
- Add error handling and logging

