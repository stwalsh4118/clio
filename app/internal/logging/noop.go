package logging

import "github.com/rs/zerolog"

// NewNoopLogger creates a no-op logger that discards all messages using zerolog's built-in Nop()
func NewNoopLogger() Logger {
	// Use zerolog's built-in no-op logger
	return &logger{zl: zerolog.Nop()}
}
