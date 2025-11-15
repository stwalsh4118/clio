CREATE TABLE IF NOT EXISTS processed_conversations (
    composer_id TEXT PRIMARY KEY,
    message_count INTEGER NOT NULL,
    last_processed_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_processed_conversations_composer_id ON processed_conversations(composer_id);
CREATE INDEX IF NOT EXISTS idx_processed_conversations_last_processed_at ON processed_conversations(last_processed_at);

