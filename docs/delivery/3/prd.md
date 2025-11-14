# PBI-3: Git Activity Capture

[View in Backlog](../backlog.md#user-content-3)

## Overview

Implement automatic monitoring and capture of git commit activity in configured repositories, extracting commit metadata and diffs, and exporting them to structured markdown files. This component enables correlation between code changes and Cursor conversations.

## Problem Statement

Git commits represent the actual code changes that result from development sessions and AI-assisted problem solving. To create comprehensive blog content, we need to capture not just the conversations but also the code changes, commit messages, and the evolution of the codebase during debugging sessions.

## User Stories

**As a developer**, I want the system to automatically capture git commits from my projects so that I can correlate code changes with my Cursor conversations.

**As a developer**, I want commit diffs captured so that I can see what code changed during debugging sessions.

**As a developer**, I want commits organized by date and project so that they align with conversation sessions.

## Technical Approach

### Components

**1. Git Repository Discovery**
- Scan configured watched directories for git repositories
- Identify `.git` directories
- Track repository paths and metadata

**2. Git Commit Monitoring**
- Monitor git repositories for new commits
- Detect commits made during active development sessions
- Extract commit metadata: hash, message, timestamp, author, branch

**3. Diff Extraction**
- Extract full diff for each commit
- Optionally extract file-level diffs
- Track files with multiple edits in short time windows (debugging signals)

**4. Commit Correlation**
- Correlate commits with conversation timestamps
- Group commits by development session
- Identify commits that occurred during active Cursor conversations

**5. Markdown Export**
- Export commits to markdown files
- Organize by date: `~/.clio/sessions/YYYY-MM-DD/<project>/commits.md`
- Include commit metadata and diffs in readable format

### Commit Markdown Format

```markdown
# Git Activity: 2025-01-27

## Commit abc123def (14:45:30)
**Branch**: main
**Author**: Developer Name
**Message**: Fix websocket connection handling

### Files Changed
- `internal/websocket/client.go` (+45, -12)
- `internal/websocket/handler.go` (+23, -8)

### Diff
```diff
+ func (c *Client) Connect() error {
+     // New connection logic
+ }
```

[full diff continues]
```

### Git Monitoring Strategy

**Option 1: Git Hooks**
- Install post-commit hooks in watched repositories
- Pros: Immediate notification, no polling
- Cons: Requires write access, might miss commits made outside working hours

**Option 2: Watch .git Directory**
- Monitor `.git/refs/heads/` for changes
- Poll or use file system events
- Pros: No modification of repositories needed
- Cons: More complex, might miss some commits

**Option 3: Periodic Polling**
- Periodically run `git log` to detect new commits
- Compare with previously seen commits
- Pros: Simple, reliable
- Cons: Not real-time, requires polling interval

**Recommended**: Start with Option 3 (polling) for simplicity, can enhance to Option 2 later.

### Data Flow

```
Watched Git Repositories
    ↓
Git Commit Monitor (polling or hooks)
    ↓
New Commit Detected
    ↓
Extract Commit Metadata & Diff
    ↓
Correlate with Active Sessions
    ↓
Markdown Exporter Writes Files
    ↓
~/.clio/sessions/YYYY-MM-DD/<project>/commits.md
```

## UX/UI Considerations

### Background Operation

- Runs automatically when monitoring is active
- No user interaction required
- Handles multiple repositories concurrently

### Performance

- Polling interval should be configurable (default: 30 seconds)
- Efficient diff extraction for large commits
- Handle repositories with thousands of commits gracefully

## Acceptance Criteria

### Must Have

1. System successfully discovers git repositories in configured watched directories
2. Git commit monitor detects new commits in watched repositories
3. System extracts commit metadata: hash, message, timestamp, author, branch
4. System extracts full commit diffs
5. Commits are exported to markdown files in correct directory structure
6. Commit markdown files are human-readable and well-formatted
7. Commits are correlated with conversation timestamps
8. System handles multiple repositories concurrently
9. System continues operating stably for 8+ hour sessions
10. Large commits (>1000 lines changed) are handled efficiently
11. System gracefully handles git repository errors (corrupted repos, etc.)

## Dependencies

### External Dependencies

- **PBI-1**: Foundation & CLI Framework (for configuration and CLI)
- **PBI-2**: Cursor Conversation Capture (for session correlation)
- Git installed and accessible
- Watched directories contain valid git repositories

### Go Libraries

- `github.com/go-git/go-git/v5` - Git operations

### System Requirements

- Git installed
- Read access to watched git repositories

## Open Questions

1. **Git Strategy**: Watch `.git` directories directly or use git hooks?
   - Hooks might miss commits made outside working hours
   - Watching `.git` requires polling or inotify complexity
   - Recommendation: Start with polling, enhance later
2. **Commit Filtering**: Should we capture all commits or filter by branch/author?
3. **Diff Size Limits**: Should we limit diff size or truncate very large diffs?
4. **Merge Commits**: How should we handle merge commits? Include or exclude?
5. **Commit Correlation**: How precise should timestamp correlation be? Within 5 minutes? Same hour?
6. **Repository Changes**: How do we handle repositories that are moved or deleted?

## Related Tasks

Tasks will be created in the tasks.md file following the project policy. Initial task breakdown will include:

- Implement git repository discovery in watched directories
- Design git commit monitoring strategy (polling vs hooks)
- Implement commit detection and extraction
- Extract commit metadata and diffs
- Implement commit-to-session correlation
- Create markdown export for commits
- Handle multiple repositories concurrently
- Add error handling for git operations
- Optimize for large commits and repositories

