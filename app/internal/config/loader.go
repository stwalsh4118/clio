package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

const (
	configDirName  = ".clio"
	configFileName = "config"
	configFileType = "yaml"
	envPrefix      = "CLIO"
)

// Load loads the configuration from file, environment variables, and defaults.
// It returns a Config struct populated with values from these sources in order of precedence:
// 1. Environment variables (CLIO_ prefix)
// 2. Configuration file (~/.clio/config.yaml)
// 3. Default values
// If the configuration file doesn't exist, it will be created automatically with default values.
func Load() (*Config, error) {
	// Ensure config file exists before loading (creates it with defaults if missing)
	if err := EnsureConfigFile(); err != nil {
		return nil, fmt.Errorf("failed to ensure config file: %w", err)
	}

	if err := initViper(); err != nil {
		return nil, fmt.Errorf("failed to initialize viper: %w", err)
	}

	setDefaults()

	if err := loadConfig(); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Apply defaults for empty string values (Viper treats empty strings as set values)
	applyDefaultsForEmptyValues(&cfg)

	// Expand home directory paths in the loaded config
	expandConfigPaths(&cfg)

	// Ensure storage base path exists before validation
	// This handles the case where config file exists but directories were deleted
	if cfg.Storage.BasePath != "" {
		if err := os.MkdirAll(cfg.Storage.BasePath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create storage base path: %w", err)
		}
	}

	// Validate configuration after loading and expanding paths
	if err := ValidateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &cfg, nil
}

// initViper initializes Viper with configuration file path, environment variable prefix, and settings
func initViper() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, configDirName)
	configPath := filepath.Join(configDir, configFileName+"."+configFileType)

	// Set config file path
	viper.SetConfigFile(configPath)

	// Set environment variable prefix
	viper.SetEnvPrefix(envPrefix)

	// Enable automatic environment variable reading
	viper.AutomaticEnv()

	// Replace dots and dashes with underscores in env var names
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// Try to read the config file (it's okay if it doesn't exist)
	if err := viper.ReadInConfig(); err != nil {
		// Ignore file not found errors - we'll use defaults
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) && !os.IsNotExist(err) {
			return fmt.Errorf("error reading config file: %w", err)
		}
	}

	return nil
}

// setDefaults sets default configuration values matching the PRD schema
func setDefaults() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// If we can't get home dir, use literal ~ which will be expanded later
		homeDir = "~"
	}

	// Watched directories - empty slice by default
	viper.SetDefault("watched_directories", []string{})

	// Blog repository - empty string by default
	viper.SetDefault("blog_repository", "")

	// Storage paths
	viper.SetDefault("storage.base_path", filepath.Join(homeDir, configDirName))
	viper.SetDefault("storage.sessions_path", filepath.Join(homeDir, configDirName, "sessions"))
	viper.SetDefault("storage.database_path", filepath.Join(homeDir, configDirName, "clio.db"))

	// Cursor log path - user must configure this explicitly
	viper.SetDefault("cursor.log_path", "")
	// Cursor polling interval - default 7 seconds
	viper.SetDefault("cursor.poll_interval_seconds", 7)

	// Session configuration
	viper.SetDefault("session.inactivity_timeout_minutes", 30)

	// Logging configuration
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.file_path", filepath.Join(homeDir, configDirName, "clio.log"))
	viper.SetDefault("logging.console", false) // Default to false (daemon mode), CLI commands can override
	viper.SetDefault("logging.max_size", 10)   // 10 MB
	viper.SetDefault("logging.max_backups", 3) // Keep 3 rotated files
}

// loadConfig performs any additional loading logic after Viper is initialized
// Currently this is a placeholder for future validation or transformation logic
func loadConfig() error {
	// Additional loading logic can be added here if needed
	return nil
}

// expandHomeDir expands ~ in a path to the user's home directory.
// It validates that the resulting path structure is within the home directory to prevent path traversal attacks.
// Note: This doesn't validate that the path exists - that's done by ValidatePath.
func expandHomeDir(path string) string {
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			// If we can't get home dir, return path as-is
			return path
		}
		if path == "~" {
			return homeDir
		}
		if strings.HasPrefix(path, "~/") {
			// Join the paths - filepath.Join will clean the path
			expanded := filepath.Join(homeDir, path[2:])

			// Get absolute path to check for traversal
			absExpanded, err := filepath.Abs(expanded)
			if err != nil {
				// If we can't get absolute, return as-is (will fail validation later)
				return path
			}

			absHome, err := filepath.Abs(homeDir)
			if err != nil {
				return path
			}

			// Check if expanded path is within home directory using Rel
			rel, err := filepath.Rel(absHome, absExpanded)
			if err != nil {
				return path
			}

			// If relative path starts with "..", it's a path traversal attempt
			if strings.HasPrefix(rel, "..") || rel == ".." {
				// Path traversal detected - return original path to fail validation later
				return path
			}

			// Path is safe - try to resolve symlinks if path exists
			// If it doesn't exist yet, that's okay - validation will catch it
			resolved, err := filepath.EvalSymlinks(absExpanded)
			if err == nil {
				// Verify resolved path is still within home
				if isPathWithinHome(resolved, absHome) {
					return resolved
				}
				// Resolved path is outside home - return original to fail validation
				return path
			}

			// Path doesn't exist yet or symlink resolution failed - return cleaned path
			// Validation will happen later in ValidatePath
			return absExpanded
		}
	}
	return path
}

// isPathWithinHome checks if a path is within the home directory.
// It resolves symlinks and uses filepath.Rel to detect path traversal attempts.
func isPathWithinHome(path, homeDir string) bool {
	// Get absolute paths
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absHome, err := filepath.Abs(homeDir)
	if err != nil {
		return false
	}

	// Resolve symlinks
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		resolvedPath = absPath
	}
	resolvedHome, err := filepath.EvalSymlinks(absHome)
	if err != nil {
		resolvedHome = absHome
	}

	// Check if path is within home directory
	rel, err := filepath.Rel(resolvedHome, resolvedPath)
	if err != nil {
		return false
	}

	// If relative path starts with "..", it's outside the home directory
	return !strings.HasPrefix(rel, "..") && rel != ".."
}

// applyDefaultsForEmptyValues applies default values for empty string fields
// This handles the case where the config file has empty strings, which Viper
// treats as set values rather than missing values.
func applyDefaultsForEmptyValues(cfg *Config) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// If we can't get home dir, skip defaults that require it
		return
	}

	// Apply logging defaults if empty
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.FilePath == "" {
		cfg.Logging.FilePath = filepath.Join(homeDir, configDirName, "clio.log")
	}
	if cfg.Logging.MaxSize == 0 {
		cfg.Logging.MaxSize = 10
	}
	if cfg.Logging.MaxBackups == 0 {
		cfg.Logging.MaxBackups = 3
	}
	// Console defaults to false, so we don't need to set it

	// Apply cursor defaults if not set
	if cfg.Cursor.PollIntervalSeconds == 0 {
		cfg.Cursor.PollIntervalSeconds = 7
	}
}

// expandConfigPaths expands all ~ paths in the configuration struct
func expandConfigPaths(cfg *Config) {
	// Expand blog repository path
	cfg.BlogRepository = expandHomeDir(cfg.BlogRepository)

	// Expand storage paths
	cfg.Storage.BasePath = expandHomeDir(cfg.Storage.BasePath)
	cfg.Storage.SessionsPath = expandHomeDir(cfg.Storage.SessionsPath)
	cfg.Storage.DatabasePath = expandHomeDir(cfg.Storage.DatabasePath)

	// Expand cursor log path
	cfg.Cursor.LogPath = expandHomeDir(cfg.Cursor.LogPath)

	// Expand logging file path
	cfg.Logging.FilePath = expandHomeDir(cfg.Logging.FilePath)

	// Expand watched directories paths
	for i, dir := range cfg.WatchedDirectories {
		cfg.WatchedDirectories[i] = expandHomeDir(dir)
	}
}
