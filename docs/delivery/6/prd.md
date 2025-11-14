# PBI-6: Analysis Workspace Generation

[View in Backlog](../backlog.md#user-content-6)

## Overview

Implement the `clio analyze` command that creates a structured workspace containing captured conversations, commits, and a prompt template for cursor-agent. This workspace enables cursor-agent to analyze development sessions and generate blog content.

## Problem Statement

To enable cursor-agent to analyze captured development sessions and generate blog posts, we need to prepare a structured workspace with all relevant context. This workspace must include conversations, commits, and clear instructions for cursor-agent on what to analyze and how to structure the output.

## User Stories

**As a developer**, I want to generate an analysis workspace for recent sessions so that cursor-agent can analyze my work and extract insights.

**As a developer**, I want the workspace to include all relevant conversations and commits so that cursor-agent has complete context.

**As a developer**, I want the workspace to include a prompt template so that I know how to guide cursor-agent's analysis.

**As a developer**, I want to filter analysis by time range or project so that I can focus on specific work periods.

## Technical Approach

### Components

**1. Analyze Command**
- `clio analyze` - Create analysis workspace
- Options: `--hours N`, `--days N`, `--session <id>`, `--project <name>`
- Query SQLite for relevant sessions
- Copy markdown files to workspace
- Generate prompt template

**2. Workspace Structure**
- Create workspace directory: `~/.clio/analysis/YYYY-MM-DD-<project>/`
- Copy relevant conversation markdown files to `sessions/` subdirectory
- Copy relevant commit markdown files to `sessions/` subdirectory
- Create `README.md` with workspace usage instructions
- Create `prompt.md` with suggested cursor-agent prompt

**3. Session Selection**
- Query database for sessions matching criteria
- Include all conversations and commits for selected sessions
- Handle overlapping time ranges intelligently

**4. Prompt Template Generation**
- Generate context-aware prompt based on selected sessions
- Include project information
- Include session metadata
- Provide clear instructions for cursor-agent

### Workspace Structure

```
~/.clio/analysis/2025-01-27-stream-tv/
├── README.md                    # How to use this workspace
├── prompt.md                    # Suggested cursor-agent prompt
├── sessions/
│   ├── conversation-001.md      # First conversation
│   ├── conversation-002.md      # Second conversation
│   └── commits.md               # Git activity
└── output/                      # Where cursor-agent writes results
    ├── insights.md              # (created by cursor-agent)
    ├── blog-draft.md            # (created by cursor-agent)
    └── suggested-updates.md     # (created by cursor-agent)
```

### Prompt Template Content

The `prompt.md` file should include:
- Task description (analyze sessions, extract insights)
- Available context (sessions directory, project info)
- Instructions for insight extraction
- Instructions for blog post drafting
- Output format requirements

### Command Examples

```bash
# Analyze last 8 hours
clio analyze --hours 8

# Analyze specific session
clio analyze --session abc123

# Analyze last week of specific project
clio analyze --project stream-tv --days 7
```

## UX/UI Considerations

### Workspace Creation

- Clear output message indicating workspace location
- Instructions on next steps (opening cursor in workspace)
- Validation that workspace was created successfully

### Prompt Template

- Should be comprehensive but not overwhelming
- Include examples where helpful
- Make it easy to customize for specific needs

## Acceptance Criteria

### Must Have

1. `clio analyze --hours N` creates workspace with sessions from last N hours
2. `clio analyze --session <id>` creates workspace for specific session
3. `clio analyze --project <name> --days N` creates workspace filtered by project and time
4. Workspace includes all relevant conversation markdown files
5. Workspace includes all relevant commit markdown files
6. Workspace includes `README.md` with usage instructions
7. Workspace includes `prompt.md` with suggested cursor-agent prompt
8. Prompt template includes project context and session metadata
9. Workspace directory structure matches specified format
10. Command outputs clear message with workspace location
11. Workspace creation handles missing or corrupted data gracefully
12. Multiple analysis workspaces can coexist (different dates/projects)

## Dependencies

### External Dependencies

- **PBI-1**: Foundation & CLI Framework (for CLI structure)
- **PBI-2**: Cursor Conversation Capture (for conversation markdown files)
- **PBI-3**: Git Activity Capture (for commit markdown files)
- **PBI-4**: Data Indexing & Storage (for querying sessions)
- **PBI-5**: Data Query Interface (for session selection logic)

### Go Libraries

- (Already included in previous PBIs)

### System Requirements

- Markdown files must exist from PBI-2 and PBI-3
- SQLite database must be populated

## Open Questions

1. **Workspace Cleanup**: Should we implement automatic cleanup of old workspaces?
2. **Workspace Naming**: How should we name workspaces? Date-project? Include session IDs?
3. **Prompt Customization**: Should users be able to customize prompt templates?
4. **Workspace Updates**: What happens if user runs analyze again for same time period? Overwrite or create new?
5. **Size Limits**: Should we limit workspace size or warn if very large?
6. **Symlinks vs Copies**: Should we copy markdown files or use symlinks to save space?

## Related Tasks

Tasks will be created in the tasks.md file following the project policy. Initial task breakdown will include:

- Implement analyze command with filtering options
- Create workspace directory structure
- Copy conversation markdown files to workspace
- Copy commit markdown files to workspace
- Generate README.md template
- Generate prompt.md template with context
- Implement session selection logic
- Add workspace validation
- Handle edge cases (no sessions found, etc.)

