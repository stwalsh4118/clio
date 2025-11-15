# Cursor API

Last Updated: 2025-11-15

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

## Notes

- All conversation data stored in global `state.vscdb`
- Workspace databases contain composer ID references only
- May be lag between UI and database writes for new conversations
- Open database in read-only mode to avoid locking issues
- User must configure the Cursor log path in config - no automatic detection

