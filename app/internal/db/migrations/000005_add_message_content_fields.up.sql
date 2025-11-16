-- Add new content fields to messages table for better insight analysis
-- These fields extract structured content from agent messages (type 2)

-- Add new columns
ALTER TABLE messages ADD COLUMN thinking_text TEXT;
ALTER TABLE messages ADD COLUMN code_blocks TEXT;
ALTER TABLE messages ADD COLUMN tool_calls TEXT;

-- Add derived/analytical fields
ALTER TABLE messages ADD COLUMN has_code INTEGER DEFAULT 0;
ALTER TABLE messages ADD COLUMN has_thinking INTEGER DEFAULT 0;
ALTER TABLE messages ADD COLUMN has_tool_calls INTEGER DEFAULT 0;
ALTER TABLE messages ADD COLUMN content_source TEXT;

-- Create partial indexes for agent messages only (type 2)
-- These indexes only index agent messages where these fields actually vary
CREATE INDEX IF NOT EXISTS idx_messages_agent_has_code 
    ON messages(has_code) WHERE type = 2;
CREATE INDEX IF NOT EXISTS idx_messages_agent_has_thinking 
    ON messages(has_thinking) WHERE type = 2;
CREATE INDEX IF NOT EXISTS idx_messages_agent_content_source 
    ON messages(content_source) WHERE type = 2;

-- Composite partial index for common query pattern
CREATE INDEX IF NOT EXISTS idx_messages_agent_code_thinking 
    ON messages(has_code, has_thinking) WHERE type = 2;

