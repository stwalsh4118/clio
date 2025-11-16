-- Remove the new content fields and indexes added in migration 000005

DROP INDEX IF EXISTS idx_messages_agent_code_thinking;
DROP INDEX IF EXISTS idx_messages_agent_content_source;
DROP INDEX IF EXISTS idx_messages_agent_has_thinking;
DROP INDEX IF EXISTS idx_messages_agent_has_code;

-- SQLite doesn't support DROP COLUMN directly, so we need to recreate the table
-- This is a simplified version - in production you'd want to preserve data
CREATE TABLE IF NOT EXISTS messages_backup AS 
SELECT id, conversation_id, bubble_id, type, role, content, created_at, metadata 
FROM messages;

DROP TABLE IF EXISTS messages;

CREATE TABLE IF NOT EXISTS messages (
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

INSERT INTO messages SELECT * FROM messages_backup;
DROP TABLE IF EXISTS messages_backup;

CREATE INDEX IF NOT EXISTS idx_messages_conversation_id ON messages(conversation_id);
CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at);
CREATE INDEX IF NOT EXISTS idx_messages_type ON messages(type);

