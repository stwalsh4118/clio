package config

// Config represents the root configuration structure for clio
type Config struct {
	WatchedDirectories []string      `mapstructure:"watched_directories" yaml:"watched_directories"`
	BlogRepository     string        `mapstructure:"blog_repository" yaml:"blog_repository"`
	Storage            StorageConfig `mapstructure:"storage" yaml:"storage"`
	Cursor             CursorConfig  `mapstructure:"cursor" yaml:"cursor"`
	Session            SessionConfig `mapstructure:"session" yaml:"session"`
	Logging            LoggingConfig `mapstructure:"logging" yaml:"logging"`
	Git                GitConfig     `mapstructure:"git" yaml:"git"`
}

// StorageConfig contains storage-related configuration
type StorageConfig struct {
	BasePath     string `mapstructure:"base_path" yaml:"base_path"`
	SessionsPath string `mapstructure:"sessions_path" yaml:"sessions_path"`
	DatabasePath string `mapstructure:"database_path" yaml:"database_path"`
}

// CursorConfig contains Cursor-related configuration
type CursorConfig struct {
	LogPath            string `mapstructure:"log_path" yaml:"log_path"`
	PollIntervalSeconds int  `mapstructure:"poll_interval_seconds" yaml:"poll_interval_seconds"`
}

// SessionConfig contains session-related configuration
type SessionConfig struct {
	InactivityTimeoutMinutes int `mapstructure:"inactivity_timeout_minutes" yaml:"inactivity_timeout_minutes"`
}

// LoggingConfig contains logging-related configuration
type LoggingConfig struct {
	Level      string `mapstructure:"level" yaml:"level"`           // "debug", "info", "warn", "error" (default: "info")
	FilePath   string `mapstructure:"file_path" yaml:"file_path"`   // Path to log file (default: ~/.clio/clio.log)
	Console    bool   `mapstructure:"console" yaml:"console"`       // Also log to console (default: false for daemon, true for CLI)
	MaxSize    int    `mapstructure:"max_size" yaml:"max_size"`     // Max log file size in MB before rotation (default: 10)
	MaxBackups int    `mapstructure:"max_backups" yaml:"max_backups"` // Number of rotated log files to keep (default: 3)
}

// GitConfig contains git-related configuration
type GitConfig struct {
	PollIntervalSeconds int `mapstructure:"poll_interval_seconds" yaml:"poll_interval_seconds"` // Polling interval in seconds (default: 30, minimum: 1)
}
