# Cursor Conversation Logs Research

**Task**: 2-1  
**Date**: 2025-11-15  
**Status**: Complete

## Executive Summary

Cursor stores conversation logs in a SQLite database located at platform-specific paths. Conversations are organized using a composer-based system where each conversation (composer) contains multiple message bubbles. The database uses a key-value storage pattern with JSON-encoded values.

## Storage Locations

### Linux
- **Base Directory**: `~/.config/Cursor/User/`
- **Database Path**: `~/.config/Cursor/User/globalStorage/state.vscdb`
- **Workspace Storage**: `~/.config/Cursor/User/workspaceStorage/{workspace-hash}/`

### macOS (Inferred)
- **Base Directory**: `~/Library/Application Support/Cursor/User/`
- **Database Path**: `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb`
- **Workspace Storage**: `~/Library/Application Support/Cursor/User/workspaceStorage/{workspace-hash}/`

**Note**: macOS path needs verification on actual macOS system, but follows standard macOS application support directory conventions.

## Database Format

### Database Type
- **Format**: SQLite 3.x database
- **File Extension**: `.vscdb` (VSCode/Cursor database format)
- **Encoding**: UTF-8
- **Size**: Can grow large (observed: 287MB with 147 conversations in global storage)

### Database Locations

1. **Global Storage Database** (`globalStorage/state.vscdb`):
   - Contains all conversation data (composerData, bubbleId entries)
   - Shared across all workspaces
   - Tables: `ItemTable`, `cursorDiskKV`

2. **Workspace Storage Databases** (`workspaceStorage/{hash}/state.vscdb`):
   - Contains workspace-specific composer ID references
   - One database per workspace
   - Tables: `ItemTable`, `cursorDiskKV`
   - Key: `composer.composerData` in `ItemTable` contains composer metadata for that workspace

### Database Schema

Both global and workspace databases contain the same table structure:

1. **`ItemTable`**
   ```sql
   CREATE TABLE ItemTable (key TEXT UNIQUE ON CONFLICT REPLACE, value BLOB);
   ```

2. **`cursorDiskKV`**
   ```sql
   CREATE TABLE cursorDiskKV (key TEXT UNIQUE ON CONFLICT REPLACE, value BLOB);
   ```

Both tables use the same structure: key-value pairs where values are stored as BLOB (binary large objects) containing JSON-encoded data.

**Key Difference**:
- **Global storage**: Contains full conversation data (`composerData:{id}`, `bubbleId:{composerId}:{bubbleId}`)
- **Workspace storage**: Contains composer ID references (`composer.composerData` with `allComposers` array)

## Conversation Data Structure

### Composer Data (Conversation Metadata)

**Key Format**: `composerData:{composerId}`

**Example Key**: `composerData:ff976b0a-1677-4488-a449-f14b72b08c22`

**Value Structure** (JSON):
```json
{
  "_v": 10,
  "composerId": "ff976b0a-1677-4488-a449-f14b72b08c22",
  "richText": "",
  "hasLoaded": true,
  "text": "",
  "fullConversationHeadersOnly": [
    {
      "bubbleId": "2797413d-c33b-49d9-a577-60b2fe08160d",
      "type": 1
    },
    {
      "bubbleId": "another-bubble-id",
      "type": 2
    }
  ],
  "conversationMap": {},
  "status": "completed",  // or "none", "active", etc.
  "context": {
    "notepads": [],
    "composers": [],
    "quotes": [],
    "selectedCommits": [],
    "fileSelections": [],
    "selections": [],
    // ... many more context fields
  },
  "createdAt": 1761827086955,  // Unix timestamp in milliseconds
  "hasChangedContext": false,
  // ... additional metadata fields
}
```

**Key Fields**:
- `composerId`: Unique identifier for the conversation
- `fullConversationHeadersOnly`: Array of message headers in chronological order
- `status`: Conversation status (e.g., "completed", "none", "active")
- `createdAt`: Unix timestamp in milliseconds when conversation was created
- `context`: Rich context data including file selections, code chunks, etc.

### Message Bubbles (Individual Messages)

**Key Format**: `bubbleId:{composerId}:{bubbleId}`

**Example Key**: `bubbleId:008b5a40-0ad7-442d-aa7c-23b725514efa:0262dc22-8c31-4833-baae-946a666a1177`

**Value Structure** (JSON):
```json
{
  "_v": 3,
  "type": 1,  // 1 = user message, 2 = agent response
  "bubbleId": "0262dc22-8c31-4833-baae-946a666a1177",
  "text": "User's message text here...",
  "createdAt": "2025-10-30T12:25:54.186Z",  // ISO 8601 timestamp
  "usageUuid": "b2e4d3a3-9c95-4cb6-a562-5c70709459ae",
  "attachedFileCodeChunksMetadataOnly": [],
  "codebaseContextChunks": [],
  "commits": [],
  "gitDiffs": [],
  "images": [],
  "toolResults": [],
  // ... extensive metadata fields
}
```

**Message Types**:
- **Type 1**: User message
- **Type 2**: Agent/Assistant response

**Key Fields**:
- `type`: Message type (1 = user, 2 = agent)
- `text`: The actual message content
- `createdAt`: ISO 8601 timestamp string
- `bubbleId`: Unique identifier for this message bubble
- `usageUuid`: Usage tracking UUID

### Message Request Context

**Key Format**: `messageRequestContext:{composerId}:{bubbleId}`

Contains context about the workspace and files when the message was sent:

```json
{
  "currentFileLocationData": "{\"relativeWorkspacePath\":\"/path/to/file\",\"lineNumber\":16,\"text\":\"code snippet\"}",
  "attachedFileCodeChunksMetadataOnly": [],
  "terminalFiles": [],
  "cursorRules": [],
  // ... more context
}
```

## Conversation Organization

### How Conversations Are Stored

1. **Conversation Identification**: Each conversation has a unique `composerId` (UUID format)
2. **Message Ordering**: Messages are ordered via `fullConversationHeadersOnly` array in composerData
3. **Message Content**: Actual message content is stored separately in `bubbleId` entries
4. **Conversation Status**: Tracked via `status` field in composerData

### Statistics Observed

- **Total Conversations**: 147 composers found in database
- **Total Messages**: ~14,879 bubble entries
- **Average Messages per Conversation**: ~100 messages
- **Largest Conversation**: 438KB (composerData entry size)

## Update Mechanism

### How Updates Work

The database uses `UNIQUE ON CONFLICT REPLACE` constraint, which means:

1. **In-Place Updates**: When a conversation is updated, the existing `composerData` entry is replaced with the new version
2. **New Messages**: New messages are added to `fullConversationHeadersOnly` array and new `bubbleId` entries are created
3. **No Versioning**: There is no version history - updates overwrite previous data
4. **Detection Strategy**: To detect updates vs new conversations:
   - Check if `composerData:{composerId}` key exists
   - Compare `fullConversationHeadersOnly` array length or content
   - Monitor `createdAt` timestamp changes
   - Track file modification time of `state.vscdb`

### File System Events

- **New Conversation**: New `composerData:{new-id}` entry created
- **Updated Conversation**: Existing `composerData:{existing-id}` entry modified
- **New Message**: New `bubbleId:{composerId}:{new-bubble-id}` entry created
- **Database File**: `state.vscdb` file is modified on every change

## Project/Workspace Detection

### How Conversations Relate to Workspaces

**Key Finding**: All conversations are stored in the **global** `state.vscdb` database. However, workspace-specific databases contain **composer ID references** that provide a direct way to find conversations for each workspace.

**Method for Workspace Association**:

Workspace database contains composer IDs that directly link conversations to workspaces:
- **Location**: `workspaceStorage/{workspace-hash}/state.vscdb`
- **Key**: `composer.composerData` in `ItemTable`
- **Structure**: JSON object with `allComposers` array containing composer metadata
- **Content**: Each entry has `composerId`, `name`, `createdAt`, `lastUpdatedAt`, and other metadata
- **Usage**: Extract composer IDs from workspace database, then query global storage using `composerData:{composerId}`
- **Verified**: This method works and is the most direct way to find workspace conversations

**How Cursor Determines Workspace Association**:
Cursor reads `composer.composerData` from the workspace database to get a list of composer IDs for that workspace, then loads the full conversation data from global storage. This is why you see the "last 4 conversations" for your current workspace - Cursor queries the workspace database first, then loads those specific conversations from global storage.

### Workspace Storage

Each workspace has a unique hash directory:
- **Location**: `~/.config/Cursor/User/workspaceStorage/{workspace-hash}/`
- **Workspace Metadata**: `workspace.json` contains:
  ```json
  {
    "folder": "file:///home/user/path/to/project"
  }
  ```
- **Database**: Each workspace has its own `state.vscdb` with **composer ID references**
  - **Key Table**: `ItemTable` contains workspace-specific data
  - **Key**: `composer.composerData` contains composer metadata for this workspace
  - **Structure**: JSON object with `allComposers` array
  - **Content**: Array of composer objects, each with `composerId`, `name`, `createdAt`, `lastUpdatedAt`, etc.
- **Other Data**: Workspace directories may contain:
  - `anysphere.cursor-retrieval/` - Retrieval/checkpoint data
  - `images/` - Workspace-specific images
  - Other workspace-specific storage

**Important**: 
- **Full conversation data** is stored in `globalStorage/state.vscdb`
- **Composer IDs** for each workspace are stored in `workspaceStorage/{hash}/state.vscdb`
- Workspace.json files provide the mapping: workspace hash → project path
- **To find conversations for a workspace**: Query `composer.composerData` in workspace database, extract composer IDs, then query global storage

### Conversation-to-Workspace Association

**Method - Direct Composer ID Lookup**:
1. **Query workspace database** for composer IDs:
   ```sql
   SELECT value FROM ItemTable WHERE key = 'composer.composerData';
   ```
2. **Parse JSON** to extract `allComposers` array
3. **Extract composer IDs** from each composer object (`composerId` field)
4. **Query global storage** for each composer:
   ```sql
   SELECT value FROM cursorDiskKV WHERE key = 'composerData:{composerId}';
   ```
5. **Verified**: This method works and found 13 composer IDs in clio workspace, all verified in global storage

**Note**: There may be a lag between UI and database writes, so newly created conversations might not appear immediately in workspace database.

## Session Boundaries

### Determining Session Boundaries

Based on the data structure, session boundaries can be determined by:

1. **Time-Based**:
   - Group conversations by `createdAt` timestamp
   - Use inactivity timeout (e.g., 30 minutes between last message and new conversation)
   - Default: `session.inactivity_timeout_minutes` from config (30 minutes)

2. **Project-Based**:
   - Group conversations by detected project/workspace
   - New project = new session

3. **Manual Boundaries**:
   - Allow explicit session markers (future enhancement)
   - Detect Cursor restart events

**Recommendation**: Use time-based grouping with project awareness. A new session starts when:
- A new conversation starts in a different project, OR
- A new conversation starts after inactivity timeout period

## Answers to PRD Open Questions

### 1. What is the exact format and location of Cursor conversation logs?

**Answer**:
- **Format**: SQLite database (`.vscdb` file) with key-value storage
- **Location**: 
  - Linux: `~/.config/Cursor/User/globalStorage/state.vscdb`
  - macOS: `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb` (inferred)
- **Structure**: 
  - Conversations stored as `composerData:{composerId}` entries
  - Messages stored as `bubbleId:{composerId}:{bubbleId}` entries
  - Values are JSON-encoded BLOBs

### 2. How do we determine session boundaries?

**Answer**:
- **Primary Strategy**: Time-based grouping with inactivity timeout (30 minutes default)
- **Secondary Strategy**: Project-based grouping (new project = new session)
- **Implementation**: 
  - Group conversations by `createdAt` timestamp
  - Determine project association from workspace database composer ID lookup
  - Start new session when project changes or timeout exceeded

### 3. How do we detect which project a conversation belongs to?

**Answer**:
- **Method**: Query workspace database for composer IDs
  - Query `workspaceStorage/{workspace-hash}/state.vscdb` → `ItemTable` → `composer.composerData`
  - Extract `allComposers` array, which contains composer metadata including `composerId`
  - Each composer in the array belongs to that workspace
  - **Verified**: Found 13 composer IDs in clio workspace database, all verified in global storage
  - This is the most direct and efficient method - Cursor uses this for filtering conversations
- **Important Notes**: 
  - All conversation data is in `globalStorage/state.vscdb`
  - Composer IDs for each workspace are in `workspaceStorage/{hash}/state.vscdb` → `composer.composerData`
  - There may be a lag between UI and database writes for newly created conversations

### 4. How do we handle conversation updates?

**Answer**:
- **Update Mechanism**: Database uses `UNIQUE ON CONFLICT REPLACE` - updates overwrite existing entries
- **Detection Strategy**:
  1. Monitor `state.vscdb` file modification time
  2. Compare `fullConversationHeadersOnly` array length before/after
  3. Track known `composerId` values - if exists, it's an update; if new, it's a new conversation
- **Implementation**: 
  - Use file system watcher on `state.vscdb`
  - Query database for changed `composerData` entries
  - Compare conversation state to detect new messages vs new conversations

## Workspace Database Composer ID Lookup (Reddit Method - Verified)

**Status**: ✅ **This method still works as of November 2025**

Based on a Reddit thread from 6 months ago, and verified on current Cursor installation:

### Method Overview

1. **Find composer IDs in workspace database**:
   - Location: `workspaceStorage/{workspace-hash}/state.vscdb`
   - Table: `ItemTable`
   - Key: `composer.composerData`
   - Query: `SELECT value FROM ItemTable WHERE key = 'composer.composerData';`

2. **Parse composer data**:
   - Value is a JSON object with `allComposers` array
   - Each entry contains:
     - `composerId`: UUID of the conversation
     - `name`: Conversation title
     - `createdAt`: Creation timestamp
     - `lastUpdatedAt`: Last update timestamp
     - Other metadata (status, file counts, etc.)

3. **Extract composer IDs**:
   - Iterate through `allComposers` array
   - Extract `composerId` from each composer object

4. **Query global storage**:
   - For each composer ID, query: `SELECT value FROM cursorDiskKV WHERE key = 'composerData:{composerId}';`
   - This gives you the full conversation data

5. **Get message content**:
   - From `composerData`, extract `fullConversationHeadersOnly` array
   - For each bubble header, query: `SELECT value FROM cursorDiskKV WHERE key = 'bubbleId:{composerId}:{bubbleId}';`
   - Extract `text` field from bubble data for message content

### Verification Results

- **Tested on**: clio workspace (`e2f3424ea92b4bb040c697925bc03b0d`)
- **Found**: 13 composer IDs in workspace database
- **Verified**: All 13 composers exist in global storage
- **Matches**: All 12 conversations visible in Cursor UI (plus 1 additional)

### Important Notes

- **Lag**: There may be a delay between UI and database writes - newly created conversations might not appear immediately
- **Efficiency**: This is the most efficient method - direct lookup vs. scanning all conversations
- **Reliability**: This is how Cursor itself filters conversations for a workspace

### Example Query Sequence

```sql
-- Step 1: Get composer IDs from workspace
SELECT value FROM ItemTable WHERE key = 'composer.composerData';
-- Returns: {"allComposers": [{"composerId": "...", "name": "..."}, ...]}

-- Step 2: For each composer ID, get full conversation data
SELECT value FROM cursorDiskKV WHERE key = 'composerData:e66399a3-6373-4c63-9090-b068df86ee2e';
-- Returns: Full composer data with fullConversationHeadersOnly array

-- Step 3: For each bubble, get message content
SELECT value FROM cursorDiskKV WHERE key = 'bubbleId:e66399a3-6373-4c63-9090-b068df86ee2e:2797413d-c33b-49d9-a577-60b2fe08160d';
-- Returns: Bubble data with "text" field containing message content
```

## Implementation Recommendations

### 1. Database Access

- Use SQLite Go driver (`github.com/mattn/go-sqlite3` or `modernc.org/sqlite`)
- Open database in read-only mode to avoid locking issues with Cursor
- Use WAL mode if possible for better concurrent access

### 2. File System Watching

- Watch `state.vscdb` file for modifications
- Use `fsnotify` library for cross-platform file watching
- Debounce events to avoid excessive processing

### 3. Parsing Strategy

1. **Initial Load - Workspace-Specific**:
   - Query workspace database for composer IDs (`composer.composerData`)
   - Extract composer IDs from `allComposers` array
   - Query global storage for each composer ID
   - Build conversation index for that workspace
   - **Advantage**: Only loads conversations for the workspace, much faster

2. **Incremental Updates**: 
   - Watch both workspace and global `state.vscdb` files for modifications
   - For workspace database: Check for new entries in `composer.composerData`
   - For global database: Query for new/updated `composerData` entries
   - Extract `fullConversationHeadersOnly` to get message list
   - Fetch `bubbleId` entries for message content
   - **Note**: Handle lag - new conversations may appear in global storage before workspace database

### 4. Project Detection

1. **Build Workspace Mapping** (one-time or periodic):
   - Scan all `workspaceStorage/{hash}/workspace.json` files
   - Build mapping: workspace hash → project path (absolute path)
   - Store mapping for efficient lookup
   - Example: `e2f3424ea92b4bb040c697925bc03b0d` → `/home/sean/Work/clio`

2. **For Each Workspace**:
   - **Query workspace database** for composer IDs:
     ```sql
     SELECT value FROM ItemTable WHERE key = 'composer.composerData';
     ```
   - **Parse JSON** to get `allComposers` array
   - **Extract composer IDs** from each composer object (`composerId` field)
   - **Query global storage** for each composer:
     ```sql
     SELECT value FROM cursorDiskKV WHERE key = 'composerData:{composerId}';
     ```
   - **Result**: All conversations for that workspace
   - **Note**: There may be a lag - newly created conversations might not appear immediately in workspace database

### 5. Session Tracking

1. Group conversations by detected project
2. Within each project, group by time (inactivity timeout)
3. Generate session IDs based on project + start timestamp
4. Track session metadata: start time, end time, project, message count

### 6. Configuration Updates

Update default config path:
- **Current**: `~/.cursor` (incorrect)
- **Should be**: `~/.config/Cursor/User/globalStorage` (Linux) or `~/Library/Application Support/Cursor/User/globalStorage` (macOS)

## Sample Data Extraction

### Example: Extracting a Conversation

```sql
-- Get composer data
SELECT value FROM cursorDiskKV WHERE key = 'composerData:ff976b0a-1677-4488-a449-f14b72b08c22';

-- Get all message bubbles for this conversation
SELECT key, value FROM cursorDiskKV 
WHERE key LIKE 'bubbleId:ff976b0a-1677-4488-a449-f14b72b08c22:%';

-- Get message request context
SELECT key, value FROM cursorDiskKV 
WHERE key LIKE 'messageRequestContext:ff976b0a-1677-4488-a449-f14b72b08c22:%';
```

### Example: Message Structure

**User Message (Type 1)**:
```json
{
  "type": 1,
  "text": "How do I debug websocket connection issues in Go?",
  "createdAt": "2025-10-30T12:25:54.186Z",
  "bubbleId": "2797413d-c33b-49d9-a577-60b2fe08160d"
}
```

**Agent Response (Type 2)**:
```json
{
  "type": 2,
  "text": "To debug websocket connection issues, you can...",
  "createdAt": "2025-10-30T12:25:58.943Z",
  "bubbleId": "0262dc22-8c31-4833-baae-946a666a1177"
}
```

## Limitations and Considerations

1. **Database Locking**: Cursor may lock the database during writes - need read-only access or retry logic
2. **Large Database**: Database can grow very large (287MB+ observed) - need efficient querying
3. **No Version History**: Updates overwrite data - cannot track conversation evolution over time
4. **Project Detection**: 
   - Workspace hash → project path mapping requires scanning workspaceStorage directories
   - Some conversations may not have clear project association if workspace database is incomplete
5. **Performance**: Full database scan on startup may be slow - need incremental loading strategy
6. **Workspace Storage**: Workspace-specific databases contain composer ID references - all conversation data is in globalStorage
7. **Workspace Hash Discovery**: Need to correlate workspace identifiers with workspaceStorage directories

## Next Steps

1. **Task 2-2**: Implement discovery service using identified paths
2. **Task 2-3**: Implement file system watcher for `state.vscdb`
3. **Task 2-4**: Design parser based on documented structure
4. **Task 2-5**: Implement session tracking with time-based and project-based grouping
5. **Task 2-7**: Implement project detection using `messageRequestContext` extraction

## References

- SQLite Documentation: https://www.sqlite.org/docs.html
- Cursor IDE: https://cursor.sh/
- VSCode Storage Format: Similar to Cursor (both based on Electron/VSCode architecture)

