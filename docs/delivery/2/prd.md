# PBI-2: Cursor Conversation Capture

[View in Backlog](../backlog.md#user-content-2)

## Overview

Implement automatic monitoring and capture of Cursor AI conversation logs, parsing them into structured markdown files organized by date and project. This component enables the system to track all developer-AI interactions for later analysis.

## Problem Statement

Cursor AI conversations contain valuable insights about problem-solving approaches, debugging strategies, and technical decisions. However, these conversations are stored in Cursor's internal format and are not easily accessible for analysis or blog post generation. We need to automatically capture and convert these conversations into a structured, readable format.

## User Stories

**As a developer**, I want the system to automatically capture my Cursor conversations so that I have a complete record of my problem-solving process without manual intervention.

**As a developer**, I want captured conversations to be organized by date and project so that I can easily find relevant sessions later.

**As a developer**, I want conversations exported to markdown format so that cursor-agent can easily read and analyze them.

## Technical Approach

### Components

**1. Cursor Log Discovery**
- Locate Cursor log directory (`~/.cursor/` or platform-specific location)
- Identify conversation log files
- Detect new conversation files as they are created

**2. File System Watcher**
- Monitor Cursor log directory for new files
- Watch for file modifications (conversation updates)
- Handle file system events efficiently

**3. Conversation Parser**
- Parse Cursor's log format (JSON or other format - TBD during research)
- Extract conversation messages
- Identify user messages vs agent responses
- Extract metadata: timestamps, session IDs, project context

**4. Session Tracking**
- Group related conversations into sessions
- Determine session boundaries (time-based or manual)
- Track session metadata: start time, end time, project, duration

**5. Markdown Export**
- Convert conversations to structured markdown format
- Organize by date: `~/.clio/sessions/YYYY-MM-DD/`
- Organize by project: `~/.clio/sessions/YYYY-MM-DD/<project-name>/`
- File naming: `conversation-001.md`, `conversation-002.md`, etc.

### Conversation Markdown Format

```markdown
# Conversation Session: 2025-01-27 14:30:00

**Project**: stream-tv
**Session ID**: abc123
**Duration**: 45 minutes
**Message Count**: 12

---

## User Message (14:30:15)

How do I debug websocket connection issues in Go?

## Agent Response (14:30:22)

To debug websocket connection issues, you can...

[conversation continues]
```

### Data Flow

```
Cursor Log Directory
    ↓
File System Watcher (fsnotify)
    ↓
New/Modified File Detected
    ↓
Parser Extracts Conversation
    ↓
Session Tracker Groups Messages
    ↓
Markdown Exporter Writes Files
    ↓
~/.clio/sessions/YYYY-MM-DD/<project>/conversation-001.md
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
5. Conversations are exported to markdown files in correct directory structure
6. Markdown files are human-readable and well-formatted
7. Session boundaries are correctly determined (time-based or manual)
8. System handles file system events without blocking
9. System continues operating stably for 8+ hour sessions
10. Multiple concurrent conversations are handled correctly
11. System gracefully handles Cursor log format variations

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
- Create markdown export functionality
- Add project detection mechanism
- Handle conversation updates and modifications
- Add error handling and logging

