package logging

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
	"github.com/stwalsh4118/clio/internal/config"
)

// Logger defines the interface for structured logging
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	With(fields ...interface{}) Logger
	WithContext(ctx context.Context) Logger
}

// logger implements Logger using zerolog
type logger struct {
	zl zerolog.Logger
}

// NewLogger creates a new logger instance based on configuration
func NewLogger(cfg *config.Config) (Logger, error) {
	if cfg == nil {
		return nil, os.ErrInvalid
	}

	logCfg := cfg.Logging

	// Parse log level
	level, err := parseLogLevel(logCfg.Level)
	if err != nil {
		level = zerolog.InfoLevel // Default to info if invalid
	}

	// Set global log level
	zerolog.SetGlobalLevel(level)

	// Create writers for output
	var writers []io.Writer

	// File output (always enabled for daemon, optional for CLI)
	if logCfg.FilePath != "" {
		fileWriter, err := createLogFile(logCfg.FilePath)
		if err != nil {
			return nil, err
		}
		writers = append(writers, fileWriter)
	}

	// Console output (if enabled)
	if logCfg.Console {
		// Use colorized console output if terminal supports it
		writers = append(writers, zerolog.ConsoleWriter{Out: os.Stderr})
	}

	// If no writers, default to stderr (shouldn't happen, but be safe)
	if len(writers) == 0 {
		writers = append(writers, os.Stderr)
	}

	// Create multi-writer if we have multiple outputs
	var output io.Writer
	if len(writers) == 1 {
		output = writers[0]
	} else {
		output = zerolog.MultiLevelWriter(writers...)
	}

	// Create zerolog logger
	zl := zerolog.New(output).With().
		Timestamp().
		Logger()

	return &logger{zl: zl}, nil
}

// parseLogLevel converts a string log level to zerolog.Level
func parseLogLevel(level string) (zerolog.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return zerolog.DebugLevel, nil
	case "info":
		return zerolog.InfoLevel, nil
	case "warn", "warning":
		return zerolog.WarnLevel, nil
	case "error":
		return zerolog.ErrorLevel, nil
	default:
		return zerolog.InfoLevel, os.ErrInvalid
	}
}

// Debug logs a debug message with optional fields
func (l *logger) Debug(msg string, fields ...interface{}) {
	l.zl.Debug().Fields(fields).Msg(msg)
}

// Info logs an info message with optional fields
func (l *logger) Info(msg string, fields ...interface{}) {
	l.zl.Info().Fields(fields).Msg(msg)
}

// Warn logs a warning message with optional fields
func (l *logger) Warn(msg string, fields ...interface{}) {
	l.zl.Warn().Fields(fields).Msg(msg)
}

// Error logs an error message with optional fields
func (l *logger) Error(msg string, fields ...interface{}) {
	l.zl.Error().Fields(fields).Msg(msg)
}

// With creates a new logger with additional fields
func (l *logger) With(fields ...interface{}) Logger {
	return &logger{
		zl: l.zl.With().Fields(fields).Logger(),
	}
}

// WithContext creates a new logger with context
func (l *logger) WithContext(ctx context.Context) Logger {
	return &logger{
		zl: l.zl.With().Ctx(ctx).Logger(),
	}
}

// createLogFile creates or opens a log file with proper permissions
func createLogFile(filePath string) (io.Writer, error) {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	// Open or create log file with append mode
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}

	// Set restrictive permissions (0600) for security
	if err := os.Chmod(filePath, 0600); err != nil {
		file.Close()
		return nil, err
	}

	return file, nil
}
