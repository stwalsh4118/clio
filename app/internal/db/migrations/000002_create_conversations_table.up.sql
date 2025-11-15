CREATE TABLE IF NOT EXISTS conversations (
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

CREATE INDEX IF NOT EXISTS idx_conversations_session_id ON conversations(session_id);
CREATE INDEX IF NOT EXISTS idx_conversations_composer_id ON conversations(composer_id);
CREATE INDEX IF NOT EXISTS idx_conversations_created_at ON conversations(created_at);


