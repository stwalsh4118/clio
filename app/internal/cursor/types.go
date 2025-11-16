package cursor

import "time"

// Conversation represents a complete conversation from Cursor's database
type Conversation struct {
	ComposerID string    // Unique identifier for the conversation
	Name       string    // Conversation title/name
	Status     string    // Conversation status (e.g., "completed", "active", "none")
	CreatedAt  time.Time // When the conversation was created
	Messages   []Message // All messages in chronological order
}

// CodeBlock represents a code block in a message
type CodeBlock struct {
	Content     string `json:"content"`      // The actual code content
	LanguageID  string `json:"languageId"`   // Language identifier (e.g., "go", "typescript", "shellscript")
	CodeBlockIdx int   `json:"codeBlockIdx"` // Index of the code block in the message
}

// ToolCall represents a tool call made by the agent
type ToolCall struct {
	Name      string `json:"name"`      // Tool name (e.g., "read_file", "write_file")
	Status    string `json:"status"`    // Tool call status (e.g., "completed", "error")
	ToolIndex int    `json:"toolIndex"` // Index of the tool call
}

// Message represents a single message in a conversation
type Message struct {
	BubbleID      string                 // Unique identifier for this message bubble
	Type          int                    // Message type: 1 = user, 2 = agent
	Role          string                 // Human-readable role: "user" or "agent" (derived from Type)
	Text          string                 // Primary message content (from 'text' field)
	ThinkingText  string                 // Agent reasoning/thought process (from 'thinking.text', type 2 only)
	CodeBlocks    []CodeBlock            // Code blocks in the message (type 2 only)
	ToolCalls     []ToolCall             // Tool calls made by the agent (type 2 only)
	ContentSource string                 // Where content came from: "text" | "thinking" | "code" | "tool" | "mixed"
	HasCode       bool                   // Derived: true if code_blocks is not empty
	HasThinking   bool                   // Derived: true if thinking_text is not empty
	HasToolCalls  bool                   // Derived: true if tool_calls is not empty
	CreatedAt     time.Time              // When the message was created
	Metadata      map[string]interface{} // Additional metadata for future extensibility
}
