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

1. Create storage: `storage, err := cursor.NewConversationStorage(database)`
2. Store conversation: `storage.StoreConversation(conversation, sessionID)`
3. Store message: `storage.StoreMessage(message, conversationID)`
4. Update conversation: `storage.UpdateConversation(conversationID, newMessages)`
5. Retrieve conversation: `conv, err := storage.GetConversationByComposerID(composerID)`
6. Retrieve by session: `conversations, err := storage.GetConversationsBySession(sessionID)`

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

- **Nil conversation/message**: Returns error
- **Nonexistent session**: Returns error when storing conversation
- **Nonexistent conversation**: Returns error when storing/updating messages
- **Database errors**: Returns wrapped errors with context
- **Transaction rollback**: Automatic on error

### Integration with SessionManager

- SessionManager automatically uses ConversationStorage for all conversation persistence
- Conversations are stored immediately when added to sessions
- LoadSessions migrates existing JSON conversations to normalized tables
- `conversations_json` column is kept for backward compatibility but set to NULL

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

