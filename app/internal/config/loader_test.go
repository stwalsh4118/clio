package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func resetViper() {
	viper.Reset()
}

func TestLoad_WithDefaults(t *testing.T) {
	resetViper()
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}

	// Verify default values
	if cfg.WatchedDirectories == nil {
		t.Error("WatchedDirectories should not be nil")
	}
	if len(cfg.WatchedDirectories) != 0 {
		t.Errorf("Expected empty WatchedDirectories, got %v", cfg.WatchedDirectories)
	}

	if cfg.BlogRepository != "" {
		t.Errorf("Expected empty BlogRepository, got %q", cfg.BlogRepository)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	expectedBasePath := filepath.Join(homeDir, ".clio")
	if cfg.Storage.BasePath != expectedBasePath {
		t.Errorf("Expected Storage.BasePath %q, got %q", expectedBasePath, cfg.Storage.BasePath)
	}

	expectedSessionsPath := filepath.Join(homeDir, ".clio", "sessions")
	if cfg.Storage.SessionsPath != expectedSessionsPath {
		t.Errorf("Expected Storage.SessionsPath %q, got %q", expectedSessionsPath, cfg.Storage.SessionsPath)
	}

	expectedDatabasePath := filepath.Join(homeDir, ".clio", "clio.db")
	if cfg.Storage.DatabasePath != expectedDatabasePath {
		t.Errorf("Expected Storage.DatabasePath %q, got %q", expectedDatabasePath, cfg.Storage.DatabasePath)
	}

	expectedCursorLogPath := filepath.Join(homeDir, ".cursor")
	if cfg.Cursor.LogPath != expectedCursorLogPath {
		t.Errorf("Expected Cursor.LogPath %q, got %q", expectedCursorLogPath, cfg.Cursor.LogPath)
	}

	if cfg.Session.InactivityTimeoutMinutes != 30 {
		t.Errorf("Expected Session.InactivityTimeoutMinutes 30, got %d", cfg.Session.InactivityTimeoutMinutes)
	}
}

func TestLoad_WithEnvironmentVariables(t *testing.T) {
	resetViper()
	// Set environment variables
	os.Setenv("CLIO_BLOG_REPOSITORY", "/test/blog/repo")
	os.Setenv("CLIO_SESSION_INACTIVITY_TIMEOUT_MINUTES", "60")
	defer func() {
		os.Unsetenv("CLIO_BLOG_REPOSITORY")
		os.Unsetenv("CLIO_SESSION_INACTIVITY_TIMEOUT_MINUTES")
		resetViper()
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.BlogRepository != "/test/blog/repo" {
		t.Errorf("Expected BlogRepository to be overridden by env var, got %q", cfg.BlogRepository)
	}

	if cfg.Session.InactivityTimeoutMinutes != 60 {
		t.Errorf("Expected InactivityTimeoutMinutes to be overridden by env var, got %d", cfg.Session.InactivityTimeoutMinutes)
	}
}

func TestExpandHomeDir(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tilde only",
			input:    "~",
			expected: homeDir,
		},
		{
			name:     "tilde with slash",
			input:    "~/test/path",
			expected: filepath.Join(homeDir, "test", "path"),
		},
		{
			name:     "no tilde",
			input:    "/absolute/path",
			expected: "/absolute/path",
		},
		{
			name:     "relative path",
			input:    "relative/path",
			expected: "relative/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandHomeDir(tt.input)
			if result != tt.expected {
				t.Errorf("expandHomeDir(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

