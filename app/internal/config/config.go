package config

// Config represents the root configuration structure for clio
type Config struct {
	WatchedDirectories []string      `mapstructure:"watched_directories" yaml:"watched_directories"`
	BlogRepository     string        `mapstructure:"blog_repository" yaml:"blog_repository"`
	Storage           StorageConfig  `mapstructure:"storage" yaml:"storage"`
	Cursor            CursorConfig   `mapstructure:"cursor" yaml:"cursor"`
	Session           SessionConfig  `mapstructure:"session" yaml:"session"`
}

// StorageConfig contains storage-related configuration
type StorageConfig struct {
	BasePath     string `mapstructure:"base_path" yaml:"base_path"`
	SessionsPath string `mapstructure:"sessions_path" yaml:"sessions_path"`
	DatabasePath string `mapstructure:"database_path" yaml:"database_path"`
}

// CursorConfig contains Cursor-related configuration
type CursorConfig struct {
	LogPath string `mapstructure:"log_path" yaml:"log_path"`
}

// SessionConfig contains session-related configuration
type SessionConfig struct {
	InactivityTimeoutMinutes int `mapstructure:"inactivity_timeout_minutes" yaml:"inactivity_timeout_minutes"`
}

