# Cursor API

Last Updated: 2025-01-27

## Storage Locations

Paths are relative to the configured `cursor.log_path` (defaults shown below):

### Linux (Default)
- Base: `~/.config/Cursor/User/` (configured via `cursor.log_path`)
- Global: `{log_path}/globalStorage/state.vscdb`
- Workspace: `{log_path}/workspaceStorage/{workspace-hash}/state.vscdb`

### macOS (Default)
- Base: `~/Library/Application Support/Cursor/User/` (configured via `cursor.log_path`)
- Global: `{log_path}/globalStorage/state.vscdb`
- Workspace: `{log_path}/workspaceStorage/{workspace-hash}/state.vscdb`

## Database Structure

SQLite 3.x database with key-value storage:
- Tables: `ItemTable`, `cursorDiskKV`
- Schema: `CREATE TABLE table_name (key TEXT UNIQUE ON CONFLICT REPLACE, value BLOB)`
- Values: JSON-encoded BLOBs

## Conversation Data Access

### Workspace Association

**Query workspace database for composer IDs**:
```sql
SELECT value FROM ItemTable WHERE key = 'composer.composerData';
```
Returns JSON with `allComposers` array containing `composerId`, `name`, `createdAt`, `lastUpdatedAt`.

### Conversation Metadata

**Get conversation data**:
```sql
SELECT value FROM cursorDiskKV WHERE key = 'composerData:{composerId}';
```
Returns JSON with:
- `composerId`: UUID
- `name`: Conversation title
- `status`: "completed", "active", "none"
- `createdAt`: Unix timestamp (milliseconds)
- `fullConversationHeadersOnly`: Array of bubble headers `[{bubbleId, type}, ...]`

### Message Content

**Get message bubble**:
```sql
SELECT value FROM cursorDiskKV WHERE key = 'bubbleId:{composerId}:{bubbleId}';
```
Returns JSON with:
- `type`: 1 (user) or 2 (agent)
- `text`: Message content
- `createdAt`: ISO 8601 timestamp
- `bubbleId`: UUID

### Message Context (Optional)

**Get message request context**:
```sql
SELECT value FROM cursorDiskKV WHERE key = 'messageRequestContext:{composerId}:{bubbleId}';
```
Returns workspace/file context for that message.

## Workspace Mapping

**Build workspace hash → project path mapping**:
- Read `workspaceStorage/{hash}/workspace.json`
- Extract `folder` field (file URI)
- Example: `e2f3424ea92b4bb040c697925bc03b0d` → `/home/user/project`

## Update Detection

- Database uses `UNIQUE ON CONFLICT REPLACE` - updates overwrite entries
- Monitor `state.vscdb` file modification time
- Compare `fullConversationHeadersOnly` array length to detect new messages

## Configuration

**Cursor Log Path**: User-configured via `config.Cursor.LogPath` in the application configuration file (`~/.clio/config.yaml`).

**Validation**: The path is validated using `config.ValidateCursorPath()` which ensures:
- Path is not empty (required)
- Path exists
- Path is a directory
- Directory is readable

**Example Configuration**:
```yaml
cursor:
  log_path: ~/.config/Cursor/User  # Linux (contains globalStorage/ and workspaceStorage/)
  # or
  log_path: ~/Library/Application Support/Cursor/User  # macOS (contains globalStorage/ and workspaceStorage/)
```

**Directory Structure**:
- `{log_path}/globalStorage/state.vscdb` - Global conversation database
- `{log_path}/workspaceStorage/{workspace-hash}/` - Workspace-specific data
  - `workspace.json` - Maps workspace hash to project path
  - `state.vscdb` - Workspace composer ID references

## File System Watcher

**Package**: `github.com/stwalsh4118/clio/internal/cursor`

### WatcherService Interface

```go
type WatcherService interface {
    Start() error
    Stop() error
    Watch() (<-chan FileEvent, error)
}
```

### FileEvent Type

```go
type FileEvent struct {
    Path      string    // Full path to the file
    EventType string    // Type of event ("WRITE", "CREATE")
    Timestamp time.Time // When the event occurred
}
```

### Usage Pattern

1. Create watcher: `watcher, err := cursor.NewWatcher(cfg)`
2. Start watching: `watcher.Start()`
3. Get events channel: `events, err := watcher.Watch()`
4. Process events: `for event := range events { ... }`
5. Stop watching: `watcher.Stop()`

### Path Construction

The watcher monitors: `{cursor.log_path}/globalStorage/state.vscdb`

- If file exists: Watches the file directly
- If file doesn't exist: Watches parent directory (`globalStorage/`) and switches to file watch when created

### Event Filtering

- Only processes `WRITE` and `CREATE` events for `state.vscdb`
- Filters out events for other files
- Ignores `CHMOD`, `REMOVE`, `RENAME` events

### Error Handling

- Logs errors but continues monitoring
- Attempts to re-establish watch if it fails
- Graceful shutdown on `Stop()`

## Conversation Parser

**Package**: `github.com/stwalsh4118/clio/internal/cursor`

### ParserService Interface

```go
type ParserService interface {
    ParseConversation(composerID string) (*Conversation, error)
    ParseAllConversations() ([]*Conversation, error)
    GetComposerIDs() ([]string, error)
    Close() error
}
```

### Data Types

```go
type Conversation struct {
    ComposerID string    // Unique identifier for the conversation
    Name       string    // Conversation title/name
    Status     string    // Conversation status (e.g., "completed", "active", "none")
    CreatedAt  time.Time // When the conversation was created
    Messages   []Message // All messages in chronological order
}

type Message struct {
    BubbleID  string                 // Unique identifier for this message bubble
    Type      int                    // Message type: 1 = user, 2 = agent
    Role      string                 // Human-readable role: "user" or "agent"
    Text      string                 // Message content
    CreatedAt time.Time              // When the message was created
    Metadata  map[string]interface{} // Additional metadata for future extensibility
}
```

### Usage Pattern

1. Create parser: `parser, err := cursor.NewParser(cfg)`
2. Parse single conversation: `conv, err := parser.ParseConversation(composerID)`
3. Parse all conversations: `conversations, err := parser.ParseAllConversations()`
4. Get composer IDs: `ids, err := parser.GetComposerIDs()`
5. Close parser: `parser.Close()`

### Database Access

- Opens database in read-only mode (`?mode=ro`) to avoid locking issues with Cursor
- Constructs path: `{cursor.log_path}/globalStorage/state.vscdb`
- Queries `cursorDiskKV` table for composer data and message bubbles
- Parses JSON-encoded BLOB values

### Query Methods

**Get Composer Data**:
- Query: `SELECT value FROM cursorDiskKV WHERE key = 'composerData:{composerId}'`
- Extracts: `composerId`, `name`, `status`, `createdAt` (Unix milliseconds), `fullConversationHeadersOnly` array

**Get Message Bubbles**:
- Iterates through `fullConversationHeadersOnly` array
- For each bubble: `SELECT value FROM cursorDiskKV WHERE key = 'bubbleId:{composerId}:{bubbleId}'`
- Extracts: `type` (1=user, 2=agent), `text`, `createdAt` (ISO 8601), `bubbleId`

### Timestamp Parsing

- **Composer timestamps**: Unix milliseconds → `time.Time`
- **Message timestamps**: ISO 8601 string → `time.Time`
- Supports multiple ISO 8601 formats (RFC3339, RFC3339Nano, custom formats)

### Role Identification

- Type `1` → Role `"user"`
- Type `2` → Role `"agent"`
- Other types → Role `"unknown"`

### Error Handling

- **Database locked**: Returns error with clear message
- **Missing entries**: Logs warning, continues parsing (allows partial conversation extraction)
- **Corrupted JSON**: Skips entry, continues with remaining messages
- **Database file not found**: Returns clear error message
- **Missing bubbles**: Skips missing bubbles, returns partial conversation

### Incremental Parsing Support

- Supports querying specific composer IDs
- Supports querying all composers
- Designed for integration with file watcher (task 2-3) and update handling (task 2-8)

## Notes

- All conversation data stored in global `state.vscdb`
- Workspace databases contain composer ID references only
- May be lag between UI and database writes for new conversations
- Open database in read-only mode to avoid locking issues
- User must configure the Cursor log path in config - no automatic detection
- File watcher detects changes but does not track state - state tracking happens in parser/updater tasks
- Parser focuses on extraction only - session tracking and markdown export are separate tasks

