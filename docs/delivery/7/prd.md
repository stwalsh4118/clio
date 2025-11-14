# PBI-7: Blog Repository Integration

[View in Backlog](../backlog.md#user-content-7)

## Overview

Integrate blog repository into analysis workspaces, enabling cursor-agent to access previous blog posts for connection-finding and cross-referencing. This component completes the workflow by providing cursor-agent with the full context needed to create interconnected blog content.

## Problem Statement

To enable cursor-agent to create blog posts that reference and build on previous writings, it needs access to the user's existing blog repository. Without this integration, cursor-agent cannot identify connections between current work and past blog posts, limiting the value of the generated content.

## User Stories

**As a blogger**, I want cursor-agent to have access to my previous blog posts so that it can identify connections to current work.

**As a blogger**, I want cursor-agent to suggest which past blog posts to link to when drafting new content.

**As a developer**, I want the blog repository included in analysis workspaces so that cursor-agent has full context for analysis.

## Technical Approach

### Components

**1. Blog Repository Configuration**
- Extend configuration to include blog repository path
- Validate that path exists and is a git repository
- Support multiple blog repositories (future enhancement)

**2. Blog Repository Integration in Workspace**
- Clone or symlink blog repository into analysis workspace
- Place in `blog-repo/` subdirectory
- Include in workspace structure

**3. Blog Repository Discovery**
- Detect blog post files (markdown files)
- Extract metadata if available (frontmatter, tags, categories)
- Make structure available to cursor-agent

**4. Workspace Prompt Enhancement**
- Update prompt template to reference blog repository
- Include instructions for finding connections
- Guide cursor-agent on cross-referencing

### Workspace Structure (Updated)

```
~/.clio/analysis/2025-01-27-stream-tv/
├── README.md                    # How to use this workspace
├── prompt.md                    # Suggested cursor-agent prompt
├── sessions/
│   ├── conversation-001.md
│   ├── conversation-002.md
│   └── commits.md
├── blog-repo/                   # Blog repository (clone or symlink)
│   ├── posts/
│   │   ├── 2025-10-15-hls-streaming.md
│   │   └── 2025-11-01-websocket-debugging.md
│   └── index.md
└── output/                      # Where cursor-agent writes results
    ├── insights.md
    ├── blog-draft.md
    └── suggested-updates.md
```

### Integration Strategy

**Option 1: Clone Repository**
- Clone blog repo into workspace
- Pros: Isolated, safe, can modify without affecting original
- Cons: Uses more disk space, requires git operations

**Option 2: Symlink**
- Create symlink to blog repo
- Pros: No duplication, always up-to-date
- Cons: Requires write access, might affect original repo

**Option 3: Copy Selected Files**
- Copy only relevant blog posts (based on keywords/topics)
- Pros: Smaller workspace, focused content
- Cons: Might miss relevant posts, requires analysis

**Recommended**: Start with Option 1 (clone) for safety and isolation, can optimize later.

### Prompt Template Enhancement

The `prompt.md` should be updated to include:
- Reference to `blog-repo/` directory
- Instructions to search for related topics
- Guidance on identifying which posts to reference
- Instructions for suggesting updates to existing posts

## UX/UI Considerations

### Configuration

- Clear instructions for setting blog repository path
- Validation that repository exists and contains blog posts
- Helpful error messages if repository is invalid

### Workspace Size

- Blog repositories can be large
- Consider options to limit what's included (e.g., only posts directory)
- Warn user if workspace will be very large

## Acceptance Criteria

### Must Have

1. `clio config --set-blog-repo <path>` successfully configures blog repository path
2. System validates that blog repository path exists and is a git repository
3. Analysis workspace includes blog repository in `blog-repo/` subdirectory
4. Blog repository is cloned (or symlinked) into workspace when `clio analyze` runs
5. Prompt template references blog repository and includes instructions for finding connections
6. Cursor-agent can successfully read blog posts from workspace
7. System handles missing or invalid blog repository configuration gracefully
8. Multiple analysis workspaces can include the same blog repository
9. Blog repository is included in workspace README.md instructions

## Dependencies

### External Dependencies

- **PBI-1**: Foundation & CLI Framework (for configuration)
- **PBI-6**: Analysis Workspace Generation (for workspace creation)

### Go Libraries

- `github.com/go-git/go-git/v5` - For cloning blog repository (if using clone strategy)

### System Requirements

- Blog repository accessible locally
- Git installed (for cloning if using clone strategy)

## Open Questions

1. **Repository Structure**: What structure should we assume for blog repositories?
   - All markdown files in one directory?
   - Organized by date/category?
   - Support for multiple blog repos?
2. **Clone vs Symlink**: Which strategy should we use? Clone is safer but uses more space.
3. **Selective Inclusion**: Should we copy only relevant posts or entire repository?
4. **Repository Updates**: Should we update blog repo in workspace if original changes?
5. **Metadata Extraction**: Should we extract and index blog post metadata (tags, categories)?
6. **Size Limits**: Should we warn or limit workspace size for very large blog repositories?

## Related Tasks

Tasks will be created in the tasks.md file following the project policy. Initial task breakdown will include:

- Extend configuration schema for blog repository
- Implement blog repository validation
- Add blog repository cloning/symlinking to workspace creation
- Update prompt template to reference blog repository
- Update workspace README with blog repository instructions
- Handle blog repository errors gracefully
- Add blog repository to workspace structure documentation

