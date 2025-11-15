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

### Database Access Helper

**Package**: `github.com/stwalsh4118/clio/internal/cursor`

**Function**:
```go
func OpenCursorDatabase(cfg *config.Config) (*sql.DB, error)
```

Opens the Cursor global `state.vscdb` database in read-only mode (`?mode=ro`) to avoid locking issues. This shared helper function is used by parser, updater, and other components that need to access Cursor's conversation database.

**Usage**:
```go
db, err := cursor.OpenCursorDatabase(cfg)
if err != nil {
    return err
}
defer db.Close()
```

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

- **Watcher creation failures**: Returns error, does not start watcher
- **Watch path determination failures**: Returns error with context (file path, parent directory)
- **File system errors**: Logs warning, continues monitoring, attempts recovery
- **Watch recovery**: Automatically attempts to re-establish watch on errors
- **Channel full**: Logs warning, drops event (non-blocking)
- **Graceful shutdown**: Closes watcher cleanly, logs shutdown events

### Logging

**Component Tag**: `component=watcher`

**Log Levels**:
- **Error**: Failed to create watcher, failed to add watch, failed to close watcher, recovery failures
- **Warn**: File watcher errors, channel full events, failed watch recovery attempts
- **Info**: Watcher started, watcher stopped, watch path determined, switched to file watch, watch recovered
- **Debug**: Event received, event filtered, path normalization, watch path determination details

**Structured Fields**:
- `watch_path`: Path being watched
- `db_path`: Database file path
- `event_type`: Type of file system event
- `file_path`: Path of file in event
- `error`: Error details

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

- **Database locked**: Returns error with clear message, logs error
- **Database open failures**: Returns wrapped error with context (database path), logs error
- **Missing composer data**: Returns error with composer ID, logs warning
- **Missing message bubbles**: Logs warning, skips bubble, continues parsing (allows partial conversation extraction)
- **Corrupted JSON**: Logs warning, skips entry, continues with remaining messages
- **Invalid timestamps**: Logs warning, uses zero time, continues parsing
- **Query failures**: Returns wrapped error with context (composer ID, bubble ID), logs error
- **Partial conversation extraction**: Returns conversation with available messages, logs warning about missing data

**Graceful Degradation**:
- Continues parsing other conversations when one fails
- Returns partial conversations when some messages fail to parse
- Logs all failures but does not crash the system

### Logging

**Component Tag**: `component=parser`

**Log Levels**:
- **Error**: Database open failures, query failures, JSON parse failures (with context)
- **Warn**: Missing bubbles, corrupted JSON, invalid timestamps, missing composer data, parsing failures for individual conversations
- **Info**: Database opened, conversation parsed, all conversations parsed, composer IDs retrieved
- **Debug**: Composer ID queried, message bubble queried, timestamp parsed, parsing details

**Structured Fields**:
- `composer_id`: Conversation identifier
- `bubble_id`: Message bubble identifier
- `db_path`: Database file path
- `message_count`: Number of messages
- `name`: Conversation name
- `status`: Conversation status
- `error`: Error details
- `count`: Number of items (composers, messages, etc.)
- `total_composers`: Total number of composers
- `successful`: Number of successful operations
- `failed`: Number of failed operations
- `missing`: Number of missing items
- `corrupted`: Number of corrupted items
- `invalid_timestamps`: Number of invalid timestamps

### Incremental Parsing Support

- Supports querying specific composer IDs
- Supports querying all composers
- Designed for integration with file watcher (task 2-3) and update handling (task 2-8)

## Session Tracking

**Package**: `github.com/stwalsh4118/clio/internal/cursor`

### SessionManager Interface

```go
type SessionManager interface {
    GetOrCreateSession(project string, conversation *Conversation) (*Session, error)
    AddConversation(sessionID string, conversation *Conversation) error
    EndSession(sessionID string) error
    GetActiveSessions() ([]*Session, error)
    GetSession(sessionID string) (*Session, error)
    LoadSessions() error
    SaveSessions() error
    StartInactivityMonitor(ctx context.Context) error
    Stop() error
}
```

### Session Type

```go
type Session struct {
    ID            string         // Unique session identifier
    Project       string         // Project name
    StartTime     time.Time      // When session started
    EndTime       *time.Time     // When session ended (nil if active)
    Conversations []*Conversation // Conversations in this session
    LastActivity  time.Time      // Last conversation/message timestamp
    CreatedAt     time.Time      // When session record was created
    UpdatedAt     time.Time      // When session was last updated
}
```

### Session Methods

```go
func (s *Session) IsActive() bool
func (s *Session) Duration() time.Duration
```

### Usage Pattern

1. Open database: `database, err := db.Open(cfg)` (database is automatically migrated)
2. Create session manager: `sm, err := cursor.NewSessionManager(cfg, database)`
3. Load existing sessions: `sm.LoadSessions()`
4. Start inactivity monitor: `sm.StartInactivityMonitor(ctx)`
5. Get or create session: `session, err := sm.GetOrCreateSession(project, conversation)`
6. Add conversations: `sm.AddConversation(sessionID, conversation)`
7. End session manually: `sm.EndSession(sessionID)`
8. Stop and save: `sm.Stop()`

### Session Boundary Detection

**Session Start**:
- New project detected (no active session for project)
- Manual session creation

**Session End**:
- Inactivity timeout: Last activity > `InactivityTimeoutMinutes` ago
- Project change: New conversation belongs to different project
- Manual end: Explicit `EndSession()` call

**Session Continuation**:
- New conversation in same project within inactivity timeout
- Conversation added to existing active session

### Persistence

**Storage Location**: SQLite database at `{storage.database_path}` (default: `~/.clio/clio.db`)

**Database Schema**:
```sql
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    project TEXT,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP,
    last_activity TIMESTAMP NOT NULL,
    conversations_json TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE conversations (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    composer_id TEXT NOT NULL,
    name TEXT,
    status TEXT,
    message_count INTEGER NOT NULL DEFAULT 0,
    first_message_time TIMESTAMP,
    last_message_time TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE TABLE messages (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    bubble_id TEXT NOT NULL,
    type INTEGER NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    metadata TEXT,
    FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
);

CREATE INDEX idx_sessions_project ON sessions(project);
CREATE INDEX idx_sessions_start_time ON sessions(start_time);
CREATE INDEX idx_sessions_active ON sessions(end_time) WHERE end_time IS NULL;
CREATE INDEX idx_conversations_session_id ON conversations(session_id);
CREATE INDEX idx_conversations_composer_id ON conversations(composer_id);
CREATE INDEX idx_conversations_created_at ON conversations(created_at);
CREATE INDEX idx_messages_conversation_id ON messages(conversation_id);
CREATE INDEX idx_messages_created_at ON messages(created_at);
CREATE INDEX idx_messages_type ON messages(type);
```

**Storage Format**:
- Session metadata stored in `sessions` table
- Conversations stored in normalized `conversations` table (replaces JSON storage)
- Messages stored in normalized `messages` table
- `conversations_json` column kept for backward compatibility but set to NULL
- Save on: session creation, conversation addition, session end, shutdown
- Load on: manager initialization (migrates JSON conversations to normalized tables if found)
- Uses WAL mode for better concurrency
- Transactions ensure atomic updates
- Foreign keys ensure referential integrity

**Database Migrations**:
- Migration files stored in `internal/db/migrations/` directory
- File naming: `{version}_{name}.up.sql` and `{version}_{name}.down.sql`
- Migrations run automatically on database initialization (via `db.Open()`)
- Migrations are idempotent (safe to run multiple times)
- Each migration runs in a transaction (all-or-nothing)
- To add new migrations: create `.up.sql` file in migrations directory with version prefix
- Migrations are embedded in the binary using `embed.FS`
- Works with pure Go SQLite driver (no CGO required)
- Tracks migration versions in `schema_migrations` table

### Inactivity Monitor

Background goroutine that:
- Runs every 1 minute
- Checks all active sessions
- Ends sessions where `time.Now().Sub(session.LastActivity) >= InactivityTimeoutMinutes`
- Uses context for graceful shutdown

### Configuration

**Session Timeout**: Configured via `config.Session.InactivityTimeoutMinutes` (default: 30 minutes)

**Database Path**: Configured via `config.Storage.DatabasePath` (default: `~/.clio/clio.db`)

**Sessions Path**: Configured via `config.Storage.SessionsPath` (default: `~/.clio/sessions`) - Used for markdown export, not session persistence

### Error Handling

- **Nil conversation**: Returns error
- **Nonexistent session**: Returns error for GetSession, AddConversation, EndSession
- **Ended session**: Returns error when adding conversation to ended session
- **File I/O errors**: Logged, continues operation
- **Corrupted JSON**: Logged, starts fresh
- **Concurrent access**: Protected with mutex

### Thread Safety

All SessionManager methods are thread-safe and protected with mutex locks.

## Conversation Storage

**Package**: `github.com/stwalsh4118/clio/internal/cursor`

### ConversationStorage Interface

```go
type ConversationStorage interface {
    StoreConversation(conversation *Conversation, sessionID string) error
    StoreMessage(message *Message, conversationID string) error
    UpdateConversation(conversationID string, newMessages []*Message) error
    GetConversation(conversationID string) (*Conversation, error)
    GetConversationByComposerID(composerID string) (*Conversation, error)
    GetConversationsBySession(sessionID string) ([]*Conversation, error)
}
```

### Usage Pattern

1. Create logger: `logger, err := logging.NewLogger(cfg)` (or use no-op logger for tests)
2. Create storage: `storage, err := cursor.NewConversationStorage(database, logger)`
3. Store conversation: `storage.StoreConversation(conversation, sessionID)`
4. Store message: `storage.StoreMessage(message, conversationID)`
5. Update conversation: `storage.UpdateConversation(conversationID, newMessages)`
6. Retrieve conversation: `conv, err := storage.GetConversationByComposerID(composerID)`
7. Retrieve by session: `conversations, err := storage.GetConversationsBySession(sessionID)`

### Storage Details

**Conversation ID**: Uses `composer_id` as the conversation ID (matches Cursor's identifier)

**Message ID**: Uses `bubble_id` as the message ID (matches Cursor's identifier)

**Transaction Handling**: 
- `StoreConversation` wraps conversation + all messages in a single transaction
- `UpdateConversation` wraps all new messages in a single transaction
- Ensures atomicity and referential integrity

**Referential Integrity**:
- Conversations require valid session (checked before storage)
- Messages require valid conversation (enforced by foreign key)
- Foreign keys use `ON DELETE CASCADE` for cleanup

**Message Ordering**: Messages are ordered by `created_at` timestamp when retrieved

**Metadata Storage**: Message metadata stored as JSON in `metadata` column

### Error Handling

- **Nil conversation/message**: Returns error, logs error
- **Nonexistent session**: Returns error when storing conversation, logs error with session ID
- **Nonexistent conversation**: Returns error when storing/updating messages, logs error with conversation ID
- **Database errors**: Returns wrapped errors with context (conversation ID, session ID, message ID), logs error
- **Transaction failures**: Returns wrapped error, logs error, automatic rollback
- **Invalid rows**: Logs warning, skips row, continues processing
- **Metadata parse failures**: Logs warning, uses empty metadata map, continues processing

**Graceful Degradation**:
- Skips invalid rows when retrieving conversations/messages
- Continues processing other items when individual items fail
- Returns partial results when some items fail to retrieve
- Logs all failures but does not crash the system

### Logging

**Component Tag**: `component=conversation_storage`

**Log Levels**:
- **Error**: Database errors, transaction failures, validation failures (with context)
- **Warn**: Invalid rows skipped, metadata parse failures, retrieval failures for individual items
- **Info**: Conversation stored, message stored, conversation updated, conversation retrieved
- **Debug**: Transaction started, transaction committed, message count calculated, storage operations

**Structured Fields**:
- `composer_id`: Conversation identifier
- `session_id`: Session identifier
- `conversation_id`: Conversation identifier
- `bubble_id`: Message bubble identifier
- `message_count`: Number of messages
- `new_message_count`: Number of new messages
- `count`: Number of items
- `successful`: Number of successful operations
- `skipped`: Number of skipped items
- `error`: Error details

### Integration with SessionManager

- SessionManager automatically uses ConversationStorage for all conversation persistence
- Conversations are stored immediately when added to sessions
- `conversations_json` column exists in schema but is not used (set to NULL)

## Project Detection

**Package**: `github.com/stwalsh4118/clio/internal/cursor`

### ProjectDetector Interface

```go
type ProjectDetector interface {
    DetectProject(conv *Conversation) (string, error)
    NormalizeProjectName(name string) string
    RefreshWorkspaceCache() error
}
```

### Usage Pattern

1. Create detector: `detector, err := cursor.NewProjectDetector(cfg)`
2. Detect project: `project, err := detector.DetectProject(conversation)`
3. Normalize name: `normalized := detector.NormalizeProjectName("/path/to/project")`
4. Refresh cache: `detector.RefreshWorkspaceCache()` (when workspaces change)

### Detection Method

**Workspace Database Lookup**:
- Scans `{cursor.log_path}/workspaceStorage/` for workspace hash directories
- For each workspace:
  - Reads `workspace.json` to get project path (from `folder` field)
  - Queries `state.vscdb` → `ItemTable` → `composer.composerData` for composer IDs
  - Builds mapping: `composerID → workspaceHash → projectPath`
- Uses cached mapping to detect project for given composer ID
- Returns normalized "unknown" if composer ID not found in any workspace

### Caching

**In-Memory Cache**:
- `workspaceHash → projectPath` (from workspace.json files)
- `composerID → workspaceHash` (from workspace databases)
- Cache is built on detector creation and can be refreshed via `RefreshWorkspaceCache()`

### Project Name Normalization

**Normalization Rules**:
- Handles `file://` URIs (extracts path, then directory name)
- Extracts directory name from full paths (e.g., `/home/user/project` → `project`)
- Removes special characters that aren't filesystem-safe
- Converts to lowercase for consistency
- Removes consecutive dashes
- Trims leading/trailing dashes
- Limits length to 255 characters
- Returns "unknown" if result is empty after normalization

**Examples**:
- `file:///home/user/my-project` → `my-project`
- `/home/user/My Project` → `my-project`
- `/home/user/my@project#123` → `my-project-123`
- Empty string → `unknown`

### Error Handling

- **Nil conversation**: Returns error and normalized "unknown"
- **Empty composer ID**: Returns error and normalized "unknown"
- **Missing workspace.json**: Skips workspace, continues scanning
- **Locked database**: Logs warning, skips workspace
- **Invalid JSON**: Logs warning, skips entry
- **Missing composer ID**: Returns normalized "unknown" (not an error)
- **Database errors**: Logs warning, continues with other workspaces

### Thread Safety

All ProjectDetector methods are thread-safe and protected with mutex locks.

### Configuration

**Workspace Storage Path**: Constructed from `config.Cursor.LogPath` + `workspaceStorage/`

**Example**:
- Config: `cursor.log_path: ~/.config/Cursor/User`
- Workspace storage: `~/.config/Cursor/User/workspaceStorage/`

## Conversation Update Handler

**Package**: `github.com/stwalsh4118/clio/internal/cursor`

### ConversationUpdater Interface

```go
type ConversationUpdater interface {
    ProcessUpdate(composerID string) error
    HasBeenProcessed(composerID string, messageCount int) bool
    MarkAsProcessed(composerID string, messageCount int) error
    DetectUpdatedComposers() ([]string, error)
    GetProcessedMessageCount(composerID string) (int, error)
}
```

### Usage Pattern

1. Create updater: `updater, err := cursor.NewConversationUpdater(cfg, db, parser, storage, sessionManager)`
2. Detect updates: `updated, err := updater.DetectUpdatedComposers()`
3. Process updates: `for _, composerID := range updated { updater.ProcessUpdate(composerID) }`
4. Check processed state: `processed := updater.HasBeenProcessed(composerID, messageCount)`

### Update Detection Workflow

**DetectUpdatedComposers()**:
- Queries Cursor database for all composer IDs using `ParserService.GetComposerIDs()`
- For each composer ID, queries `composerData:{composerID}` to get `fullConversationHeadersOnly` array length
- Compares current message count with processed count from `processed_conversations` table
- Returns composer IDs where `currentCount > processedCount`

### Incremental Message Parsing

**ProcessUpdate(composerID)**:
- Gets processed message count from `processed_conversations` table
- Parses full conversation using `ParserService.ParseConversation(composerID)`
- Extracts only messages beyond processed count (slices `Messages` array)
- Updates conversation using `ConversationStorage.UpdateConversation()` with new messages only
- Marks conversation as processed with new total message count
- Updates session metadata (last_activity, updated_at) if conversation belongs to a session

### Processed State Tracking

**Database Schema**:
```sql
CREATE TABLE processed_conversations (
    composer_id TEXT PRIMARY KEY,
    message_count INTEGER NOT NULL,
    last_processed_at TIMESTAMP NOT NULL
);
```

**State Management**:
- `GetProcessedMessageCount()`: Returns processed message count (0 if not processed)
- `HasBeenProcessed()`: Checks if composer ID has been processed with given message count
- `MarkAsProcessed()`: Updates processed state atomically (uses `ON CONFLICT REPLACE`)

### Error Handling

- **Conversation not found**: Returns nil (treats as new conversation, not an update)
- **Database errors**: Logs warnings, continues processing other composers
- **Missing messages**: Parser handles missing bubbles gracefully
- **Concurrent updates**: SQLite handles locking, transactions ensure atomicity
- **First-time processing**: If composer ID not in `processed_conversations`, treated as new conversation

### Integration

- **Dependencies**: Requires `ParserService`, `ConversationStorage`, `SessionManager`, and database connection
- **File Watcher Integration**: Called when `state.vscdb` file modification is detected (task 2-10)
- **Session Updates**: Automatically updates session `last_activity` when conversations are updated
- **Atomic Operations**: Uses database transactions for updating messages and processed state together

### Configuration

**Cursor Database Path**: Constructed from `config.Cursor.LogPath + "globalStorage/state.vscdb"`

**Database Access**: Opens Cursor database in read-only mode (`?mode=ro`) to avoid locking issues

## Error Handling and Logging Patterns

### General Principles

All Cursor capture components follow consistent error handling and logging patterns:

**Error Handling**:
- Errors are wrapped with context using `fmt.Errorf("operation failed: %w", err)`
- Context includes relevant identifiers (composer ID, session ID, bubble ID, file paths)
- Critical failures return errors and stop operation
- Non-critical failures log warnings and continue operation (graceful degradation)
- Partial results are returned when possible (e.g., conversation with some messages missing)

**Logging**:
- All components use structured logging with component tags
- Logger initialization uses fallback to no-op logger if creation fails (prevents component creation failures)
- Consistent log levels across components:
  - **Error**: Component failures, corrupted files, database errors, critical system failures
  - **Warn**: Skipped files, format variations, recoverable issues, missing data, retries
  - **Info**: Session start/end, file captures, important events, component lifecycle
  - **Debug**: Detailed operation flow, file paths, parsing details, query details

**Structured Fields**:
- Consistent field names across components (composer_id, session_id, bubble_id, etc.)
- Error details always included in error log entries
- Operation counts and statistics logged for monitoring

**Graceful Degradation**:
- System continues operating despite individual component failures
- Individual item failures don't crash the system
- Partial results returned when full results unavailable
- All failures logged for troubleshooting

## Notes

- All conversation data stored in global `state.vscdb`
- Workspace databases contain composer ID references only
- May be lag between UI and database writes for new conversations
- Open database in read-only mode to avoid locking issues
- User must configure the Cursor log path in config - no automatic detection
- File watcher detects changes but does not track state - state tracking happens in parser/updater tasks
- Parser focuses on extraction only - session tracking and markdown export are separate tasks
- Sessions are grouped by project - project detection (task 2-7) provides project name
- Session manager persists state across application restarts
- All components include comprehensive error handling and logging for stable operation

