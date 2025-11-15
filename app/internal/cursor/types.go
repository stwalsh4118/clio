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

// Message represents a single message in a conversation
type Message struct {
	BubbleID  string                 // Unique identifier for this message bubble
	Type      int                    // Message type: 1 = user, 2 = agent
	Role      string                 // Human-readable role: "user" or "agent" (derived from Type)
	Text      string                 // Message content
	CreatedAt time.Time              // When the message was created
	Metadata  map[string]interface{} // Additional metadata for future extensibility
}
