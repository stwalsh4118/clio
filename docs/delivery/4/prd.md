# PBI-4: Data Indexing & Storage

[View in Backlog](../backlog.md#user-content-4)

## Overview

Implement SQLite-based indexing system for fast querying of captured conversations and git commits. This component enables efficient searching, filtering, and retrieval of development sessions without requiring cursor-agent to read all markdown files.

## Problem Statement

While markdown files provide human-readable storage, we need a fast, queryable index to enable efficient searching and filtering of captured data. Without indexing, finding relevant sessions would require reading all markdown files, which is slow and inefficient.

## User Stories

**As a developer**, I want to quickly search captured sessions by keywords so that I can find relevant conversations and commits.

**As a developer**, I want to filter sessions by date range and project so that I can focus on specific time periods or projects.

**As a developer**, I want the system to maintain an index of all captured data so that queries are fast and efficient.

## Technical Approach

### Components

**1. SQLite Database Schema**
- Sessions table: session metadata (ID, start_time, end_time, project, duration)
- Conversations table: conversation metadata (ID, session_id, file_path, message_count, timestamps)
- Commits table: commit metadata (ID, session_id, hash, message, timestamp, author, branch, files_changed)
- Full-text search indexes for fast keyword searching

**2. Indexing Service**
- Index new conversations as they are captured
- Index new commits as they are captured
- Update indexes when sessions are modified
- Maintain referential integrity between tables

**3. Query Interface**
- Functions for querying by date range
- Functions for querying by project
- Functions for full-text search by keywords
- Functions for retrieving session details

**4. Database Initialization**
- Create database schema on first run
- Run migrations if schema changes
- Handle database corruption gracefully

### Database Schema

```sql
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP,
    project TEXT,
    duration_minutes INTEGER,
    conversation_count INTEGER,
    commit_count INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE conversations (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    message_count INTEGER,
    first_message_time TIMESTAMP,
    last_message_time TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE TABLE commits (
    id TEXT PRIMARY KEY,
    session_id TEXT,
    hash TEXT NOT NULL,
    message TEXT,
    timestamp TIMESTAMP NOT NULL,
    author TEXT,
    branch TEXT,
    files_changed INTEGER,
    lines_added INTEGER,
    lines_removed INTEGER,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

-- Full-text search indexes
CREATE VIRTUAL TABLE conversations_fts USING fts5(
    session_id,
    content,
    content=conversations
);

CREATE INDEX idx_sessions_project ON sessions(project);
CREATE INDEX idx_sessions_start_time ON sessions(start_time);
CREATE INDEX idx_commits_session ON commits(session_id);
CREATE INDEX idx_commits_timestamp ON commits(timestamp);
```

### Data Flow

```
Markdown Files Created (PBI-2, PBI-3)
    ↓
Indexing Service Detects New Files
    ↓
Extract Metadata from Markdown
    ↓
Insert/Update SQLite Database
    ↓
Full-text Index Updated
    ↓
Query Interface Available
```

## UX/UI Considerations

### Performance

- Indexing should not block capture operations
- Queries should return results in <100ms for typical searches
- Database should handle thousands of sessions efficiently

### Reliability

- Database corruption should be detected and handled
- Indexing failures should not prevent data capture
- Regular database backups or recovery mechanisms

## Acceptance Criteria

### Must Have

1. SQLite database is created in configured location (`~/.insightd/insightd.db`)
2. Database schema includes sessions, conversations, and commits tables
3. Full-text search indexes are created for keyword searching
4. New conversations are automatically indexed as they are captured
5. New commits are automatically indexed as they are captured
6. Sessions can be queried by date range
7. Sessions can be queried by project name
8. Full-text search returns relevant sessions by keywords
9. Query performance is acceptable (<100ms for typical queries)
10. Database handles thousands of sessions without performance degradation
11. Indexing failures are logged but don't prevent data capture
12. Database schema can be migrated if changes are needed

## Dependencies

### External Dependencies

- **PBI-1**: Foundation & CLI Framework (for configuration)
- **PBI-2**: Cursor Conversation Capture (for conversation data to index)
- **PBI-3**: Git Activity Capture (for commit data to index)

### Go Libraries

- `github.com/mattn/go-sqlite3` - SQLite database driver

### System Requirements

- Write access to database location
- SQLite3 support

## Open Questions

1. **Indexing Strategy**: Should we index markdown content directly or extract structured data?
2. **Full-text Search**: What content should be indexed? Just messages? Commit messages? Code diffs?
3. **Database Size**: Should we implement data retention/archival policies?
4. **Migration Strategy**: How should we handle schema changes in production?
5. **Backup Strategy**: Should we implement automatic database backups?
6. **Performance**: Should we implement caching layer for frequently accessed data?

## Related Tasks

Tasks will be created in the tasks.md file following the project policy. Initial task breakdown will include:

- Design SQLite database schema
- Implement database initialization and migration
- Create indexing service for conversations
- Create indexing service for commits
- Implement full-text search indexes
- Create query interface functions
- Add performance optimizations (indexes, query optimization)
- Implement error handling and recovery
- Add database backup/recovery mechanisms

