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
func Load() (*Config, error) {
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

	// Expand home directory paths in the loaded config
	expandConfigPaths(&cfg)

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

	// Cursor log path
	viper.SetDefault("cursor.log_path", filepath.Join(homeDir, ".cursor"))

	// Session configuration
	viper.SetDefault("session.inactivity_timeout_minutes", 30)
}

// loadConfig performs any additional loading logic after Viper is initialized
// Currently this is a placeholder for future validation or transformation logic
func loadConfig() error {
	// Additional loading logic can be added here if needed
	return nil
}

// expandHomeDir expands ~ in a path to the user's home directory
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
			return filepath.Join(homeDir, path[2:])
		}
	}
	return path
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

	// Expand watched directories paths
	for i, dir := range cfg.WatchedDirectories {
		cfg.WatchedDirectories[i] = expandHomeDir(dir)
	}
}

