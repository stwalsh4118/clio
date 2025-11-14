# PRD: Insight Capture System for Development Workflow (Cursor-Agent Powered)

## Overview

A Go-based data capture and storage system that monitors Cursor AI conversations and git activity, storing them in a queryable format. The system integrates with cursor-agent mode to intelligently extract insights, connect them to previous blog posts, and draft new content with cross-references. By leveraging cursor-agent's ability to understand both code context and blog content, the system creates richer, more connected technical writing.

## Problem Statement

During development, especially when working with AI coding agents like Cursor, valuable insights emerge during the debugging and iteration phase. These insights represent real problem-solving experiences that would make excellent blog content, but they're lost because:

1. **Cognitive Load**: Taking notes manually during debugging breaks flow state
2. **Context Loss**: Insights are scattered across conversations, commits, and code changes
3. **Timing**: The best insights happen when you're most focused on fixing problems, not documenting them
4. **Disconnected Content**: Even when blog posts are written, they don't reference or build on previous posts because that context isn't easily accessible
5. **Compilation Effort**: Organizing scattered insights into coherent, interconnected blog posts is time-consuming

The result is that valuable technical content never gets written, and when it is written, it exists in isolation rather than building a connected knowledge base.

## User Stories

### Primary User Stories

**As a developer**, I want the system to automatically capture my Cursor conversations and git activity so that I have a complete record of my problem-solving process without manual intervention.

**As a developer**, I want to ask cursor-agent "what insights did I generate today?" and have it analyze captured data in context of my code and previous blog posts.

**As a blogger**, I want cursor-agent to draft blog posts that naturally reference my previous writings so that my content builds on itself and creates a cohesive narrative.

**As a developer**, I want cursor-agent to identify connections between current work and past blog posts so that I can update or extend existing content rather than always writing from scratch.

**As a developer**, I want the system to make my captured data easily accessible to cursor-agent (via files or simple queries) so that the agent has full context when drafting.

### Secondary User Stories

**As a blogger**, I want cursor-agent to suggest which past blog posts to link to when drafting new content so that readers can explore related topics.

**As a developer**, I want to review captured insights in a structured format (markdown files by date/project) so that I can see patterns over time.

**As a blogger**, I want cursor-agent to identify when I'm repeating solutions I've already written about so that I can decide whether to reference the old post or approach the topic differently.

**As a developer**, I want to configure which projects/directories get monitored so that personal projects don't pollute my professional blog content.

## Technical Approach

### Architecture Components

**1. Cursor Monitor Service (Go)**
- File system watcher monitoring `~/.cursor/` for conversation logs
- Parse conversation JSON/log format to extract messages
- Identify user messages vs agent responses
- Track conversation sessions with timestamps
- Export conversations to structured markdown files organized by date/project

**2. Git Activity Tracker (Go)**
- Monitor configured git repositories for new commits
- Extract commit metadata (hash, message, timestamp, author, branch)
- Capture diffs for commits
- Track files with multiple edits in short time windows (debugging signals)
- Export git activity to structured markdown files correlating with conversation timestamps

**3. Data Storage Layer (File-based + SQLite)**
- **Markdown Files**: Raw conversations and git activity organized by date/project
  - `~/.insightd/sessions/2025-11-13/stream-tv/conversation-001.md`
  - `~/.insightd/sessions/2025-11-13/stream-tv/commits.md`
- **SQLite Index**: Metadata for fast querying
  - Session timestamps, projects, file paths, commit hashes
  - Full-text search index for quick filtering
- **Blog Repository**: User's existing blog post repository (external)

**4. Cursor-Agent Integration Layer**
- CLI commands that prepare context for cursor-agent
- Generate "insight prompts" that tell cursor-agent what data to analyze
- Create workspace views combining captured data + blog repository
- Output structured data files cursor-agent can easily parse

**5. CLI Interface**
- Start/stop monitoring
- Query captured data (by date, project, keywords)
- Generate cursor-agent prompts for insight extraction
- Open cursor-agent with pre-loaded context

### Data Flow

```
Cursor Logs → Monitor → Parse → Markdown Files + SQLite Index
                                         ↓
Git Commits → Tracker → Extract Diffs → Markdown Files + SQLite Index
                                         ↓
                                    User triggers analysis
                                         ↓
                            CLI generates context package:
                            - Recent conversations (markdown)
                            - Related commits (markdown)  
                            - Relevant blog posts (from blog repo)
                                         ↓
                            Cursor-Agent analyzes with prompt:
                            "Extract insights, find connections,
                             draft blog post with cross-references"
                                         ↓
                            Cursor-Agent outputs:
                            - Extracted insights (structured)
                            - Blog post draft (markdown)
                            - Suggested updates to existing posts
```

### Key Technical Decisions

**File-Based Storage for Cursor-Agent Consumption**:
- Markdown files are easily read by cursor-agent
- Organized directory structure provides natural context boundaries
- SQLite index enables fast querying without overwhelming agent context
- Human-readable for debugging and manual review

**Cursor-Agent as Analysis Engine**:
- Already has sophisticated context management
- Can read entire blog repository for connection-finding
- User can iteratively refine insights through conversation
- No need to manage Claude API calls separately
- Better at understanding code context than standalone API calls

**Go for Data Capture Only**:
- Fast, reliable file system watching
- Efficient git operations
- Single binary deployment
- Analysis happens in cursor-agent, not Go

**Blog Repository Integration**:
- System references user's existing blog post git repository
- Cursor-agent can clone/read blog repo to understand past content
- Enables intelligent cross-referencing and connection-making

### Cursor-Agent Workflow

**Example Usage Session:**

```bash
# 1. Start monitoring (runs in background)
insightd start

# 2. Work normally for several hours...

# 3. End of day - prepare analysis context
insightd analyze --hours 8 --project stream-tv

# This command:
# - Queries SQLite for relevant sessions
# - Copies related markdown files to analysis workspace
# - Finds related blog posts from blog repo
# - Generates a prompt file for cursor-agent
# - Outputs: "Analysis workspace ready at ~/.insightd/analysis/2025-11-13/"

# 4. Open cursor in the analysis workspace
cd ~/.insightd/analysis/2025-11-13/
cursor .

# 5. In cursor-agent mode, user asks:
# "Review the captured sessions and commits in the 'sessions/' directory.
#  Analyze my debugging process and extract key insights.
#  Check my blog posts in 'blog-repo/' for related topics.
#  Draft a blog post about today's learnings with references to past posts."

# 6. Cursor-agent produces:
# - insights.md (structured insights with metadata)
# - blog-draft.md (draft post with cross-references)
# - suggested-updates.md (existing posts to update/link)
```

## UX/UI Considerations

### CLI Commands

```bash
# Background monitoring
insightd start                    # Start monitoring daemon
insightd stop                     # Stop monitoring
insightd status                   # Check if running

# Configuration
insightd config --add-watch ~/projects/stream-tv
insightd config --set-blog-repo ~/repos/blog
insightd config --show

# Querying captured data
insightd list --hours 24          # List recent sessions
insightd list --project stream-tv # List by project
insightd search "websocket error" # Full-text search
insightd show session <id>        # View specific session

# Analysis preparation
insightd analyze --hours 8                    # Last 8 hours
insightd analyze --session <id>               # Specific session
insightd analyze --project stream-tv --days 7 # Last week of project

# Opens analysis workspace with:
# - sessions/: Captured conversations and commits (markdown)
# - blog-repo/: Clone of blog repository  
# - prompt.md: Suggested prompt for cursor-agent
# - README.md: Guide for using workspace
```

### Analysis Workspace Structure

```
~/.insightd/analysis/2025-11-13-stream-tv/
├── README.md                    # How to use this workspace
├── prompt.md                    # Suggested cursor-agent prompt
├── sessions/
│   ├── conversation-001.md      # First conversation of session
│   ├── conversation-002.md      # Second conversation
│   └── commits.md               # Git activity during session
├── blog-repo/                   # Clone/symlink to blog posts
│   ├── posts/
│   │   ├── 2025-10-15-hls-streaming.md
│   │   └── 2025-11-01-websocket-debugging.md
│   └── index.md
└── output/                      # Where cursor-agent writes results
    ├── insights.md              # (created by cursor-agent)
    ├── blog-draft.md            # (created by cursor-agent)
    └── suggested-updates.md     # (created by cursor-agent)
```

### Example Prompt.md Content

```markdown
# Insight Analysis Prompt

## Task
Analyze the captured development sessions and draft a blog post about today's learnings.

## Context Available
- **Sessions**: See `sessions/` directory for conversations and git activity
- **Blog Posts**: See `blog-repo/posts/` for previous writings
- **Project**: stream-tv (HLS video streaming application)

## Instructions

1. **Extract Insights**: 
   - Review conversations in `sessions/`
   - Identify key debugging moments, decisions, and learnings
   - Focus on: problem → attempted solution → actual solution
   - Extract reusable patterns or techniques

2. **Find Connections**:
   - Search `blog-repo/posts/` for related topics
   - Identify which past posts to reference
   - Note if this extends or contradicts previous writings

3. **Draft Blog Post**:
   - Write in `output/blog-draft.md`
   - Use conversational, technical tone
   - Include code snippets from today's work
   - Add cross-references to relevant past posts
   - Structure: Problem → Journey → Solution → Lessons

4. **Suggest Updates**:
   - Write in `output/suggested-updates.md`
   - List existing posts that should link to this new post
   - Suggest updates to existing posts if this provides new context

## Output Format

Create three files in `output/`:
1. `insights.md` - Structured insights (use YAML frontmatter for metadata)
2. `blog-draft.md` - Full blog post draft
3. `suggested-updates.md` - List of posts to update with suggested edits
```

## Acceptance Criteria

### Must Have (MVP)

1. System successfully monitors Cursor conversation logs and detects new conversations
2. System monitors git commits in configured directories
3. System exports conversations and commits to structured markdown files
4. SQLite index enables fast querying of sessions by date, project, and keywords
5. `insightd analyze` command creates analysis workspace with all relevant context
6. Analysis workspace includes blog repository reference
7. Generated prompt.md provides clear instructions for cursor-agent
8. System runs stably as background service for 8+ hour sessions
9. Configuration persists across restarts
10. Cursor-agent can successfully read and analyze workspace structure

### Should Have (Phase 2)

1. Automatic blog post metadata extraction (tags, categories from existing posts)
2. Template system for different types of blog posts (tutorial, debugging story, architecture)
3. Integration with git hosting APIs for PR context
4. Periodic analysis scheduling (daily summary generation)
5. Rich metadata capture (stack traces, error messages, timings)

### Could Have (Future)

1. Web UI for browsing captured sessions and insights
2. Team sharing (shared insight repository)
3. Direct publishing to blog platforms (dev.to, Medium)
4. Analytics on debugging patterns
5. Slack integration for sharing insights
6. Voice note integration for additional context capture

## Dependencies

### External Dependencies

- **Cursor**: System assumes Cursor AI IDE is being used and stores logs locally
- **Git**: Requires git repositories in watched directories
- **Blog Repository**: User maintains a git repository of blog posts

### Go Libraries (Estimated)

- `github.com/fsnotify/fsnotify` - File system watching
- `github.com/go-git/go-git/v5` - Git operations
- `github.com/mattn/go-sqlite3` - SQLite database
- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration management
- `github.com/yuin/goldmark` - Markdown parsing/manipulation

### System Requirements

- macOS/Linux (Windows support later)
- Go 1.21+
- Write access to home directory for database/config
- Cursor IDE installed
- Git installed
- Blog repository accessible locally

## Open Questions

1. **Cursor Log Format**: What is the exact format and location of Cursor conversation logs? Need to investigate Cursor's storage mechanism.

2. **Session Boundaries**: How do we determine when a development session ends? 
   - Time-based (30 minutes of inactivity)?
   - Manual `insightd end-session`?
   - Per-project tracking?

3. **Blog Repository Structure**: What structure should we assume for blog repositories?
   - All markdown files in one directory?
   - Organized by date/category?
   - Support for multiple blog repos?

4. **Cursor-Agent Context Limits**: How much data can we reasonably include in analysis workspace before overwhelming cursor-agent's context window?
   - Full conversations or summaries?
   - All commits or just meaningful ones?
   - All blog posts or just related ones?

5. **Privacy & Sensitive Data**: 
   - How do we handle API keys, passwords in captured conversations?
   - Should we have auto-redaction patterns?
   - Option to mark sessions as "private" (capture but don't analyze)?

6. **Multi-Project Sessions**: If user works on multiple projects simultaneously, how do we separate insights?
   - Per-project analysis workspaces?
   - Combined with tagging?

7. **Git Strategy**: Watch `.git` directories directly or use git hooks?
   - Hooks might miss commits made outside working hours
   - Watching `.git` requires polling or inotify complexity

8. **Storage Cleanup**: How long to keep captured data?
   - Keep markdown files forever (they're small)?
   - Archive SQLite entries after N days?
   - User-configurable retention?

9. **Cursor-Agent Automation**: Should we support automatic cursor-agent invocation?
   - Schedule analysis runs?
   - Or always require manual `insightd analyze` + cursor usage?

10. **Insight Format**: What structured format should cursor-agent output insights in?
    - YAML frontmatter for metadata?
    - JSON for programmatic use?
    - Pure markdown for readability?

## Related Tasks

Tasks will be created in a separate PBI following the project policy. Initial task breakdown will include:

- Research Cursor log format and storage location
- Design database schema and file structure
- Implement file system watcher for Cursor logs
- Implement git commit monitor  
- Implement markdown export functionality
- Build SQLite indexing system
- Build CLI framework with core commands
- Implement analysis workspace generation
- Create prompt templates for cursor-agent
- Add blog repository integration
- Implement configuration management
- Create installation and setup documentation

---

**Next Steps**: 
1. User reviews and approves this PRD
2. Create PBI in `docs/delivery/backlog.md` with appropriate ID
3. Create PBI detail directory `docs/delivery/<PBI-ID>/prd.md` with this content
4. Break down into tasks in `docs/delivery/<PBI-ID>/tasks.md`