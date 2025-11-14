# PBI-5: Data Query Interface

[View in Backlog](../backlog.md#user-content-5)

## Overview

Implement CLI commands for querying and displaying captured development sessions, conversations, and commits. This component provides users with the ability to explore their captured data and verify that the system is working correctly.

## Problem Statement

Users need a way to verify that data is being captured correctly and to explore their captured sessions. Without query capabilities, users cannot confirm the system is working or find specific sessions they want to review.

## User Stories

**As a developer**, I want to list recent sessions so that I can see what has been captured.

**As a developer**, I want to search sessions by keywords so that I can find conversations about specific topics.

**As a developer**, I want to filter sessions by project so that I can focus on specific work.

**As a developer**, I want to view details of a specific session so that I can review captured conversations and commits.

## Technical Approach

### Components

**1. List Command**
- `insightd list` - List recent sessions
- Support filtering by: `--hours`, `--days`, `--project`
- Display: session ID, project, start time, duration, conversation count, commit count
- Pagination for large result sets

**2. Search Command**
- `insightd search <query>` - Full-text search across conversations and commits
- Highlight matching content in results
- Support filtering by date range and project
- Rank results by relevance

**3. Show Command**
- `insightd show session <id>` - Display full details of a session
- Show conversation summaries
- Show commit list with messages
- Link to markdown files for full content

**4. Query Implementation**
- Use SQLite query interface from PBI-4
- Format output for terminal display
- Handle large result sets efficiently

### Command Examples

```bash
# List recent sessions
insightd list --hours 24
insightd list --project stream-tv
insightd list --days 7

# Search for keywords
insightd search "websocket error"
insightd search "database connection" --project stream-tv
insightd search "authentication" --hours 48

# Show session details
insightd show session abc123
```

### Output Format

**List Command Output:**
```
Session ID    Project      Start Time          Duration  Conversations  Commits
abc123        stream-tv    2025-01-27 14:30    45m       3              5
def456        work-proj    2025-01-27 10:15    120m      8              12
```

**Search Command Output:**
```
Found 3 sessions matching "websocket error":

Session abc123 (stream-tv) - 2025-01-27 14:30
  Conversation 1: "How do I debug websocket connection issues..."
  Commit def789: "Fix websocket connection handling"
  
[more results...]
```

**Show Command Output:**
```
Session: abc123
Project: stream-tv
Start: 2025-01-27 14:30:00
Duration: 45 minutes
Conversations: 3
Commits: 5

Conversations:
  - conversation-001.md (12 messages)
  - conversation-002.md (8 messages)
  - conversation-003.md (5 messages)

Commits:
  - abc123def: Fix websocket connection handling
  - def456ghi: Add connection retry logic
  ...
```

## UX/UI Considerations

### Output Formatting

- Clean, readable table formatting
- Color coding for different session states (optional)
- Truncate long messages with "..." and full text available via show command

### Performance

- Commands should return results quickly (<1 second)
- Pagination for large result sets
- Progress indicators for long-running queries

### Error Handling

- Clear error messages for invalid session IDs
- Helpful messages when no results found
- Suggestions for similar queries

## Acceptance Criteria

### Must Have

1. `insightd list` command displays recent sessions in readable format
2. `insightd list --hours N` filters sessions by time range
3. `insightd list --project <name>` filters sessions by project
4. `insightd search <query>` performs full-text search across conversations and commits
5. Search results show relevant context (matching conversation snippets)
6. `insightd show session <id>` displays full session details
7. Show command includes links to markdown files
8. Commands handle invalid inputs gracefully with helpful error messages
9. Commands return results in reasonable time (<1 second for typical queries)
10. Output is formatted clearly and is easy to read
11. Commands handle empty result sets gracefully

## Dependencies

### External Dependencies

- **PBI-1**: Foundation & CLI Framework (for CLI structure)
- **PBI-4**: Data Indexing & Storage (for query interface)

### Go Libraries

- (Already included in PBI-1 and PBI-4)

### System Requirements

- SQLite database must exist and be populated

## Open Questions

1. **Output Format**: Should we support JSON output for scripting/automation?
2. **Pagination**: What should the default page size be? Should it be configurable?
3. **Search Ranking**: How should we rank search results? By relevance? By recency?
4. **Export**: Should we support exporting search results to files?
5. **Interactive Mode**: Should we support an interactive mode for browsing sessions?

## Related Tasks

Tasks will be created in the tasks.md file following the project policy. Initial task breakdown will include:

- Implement list command with filtering options
- Implement search command with full-text search
- Implement show command for session details
- Create output formatting functions
- Add pagination support
- Implement error handling
- Add performance optimizations
- Write command documentation and help text

