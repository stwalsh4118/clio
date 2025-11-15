package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Save saves the configuration to the config file (~/.clio/config.yaml).
// It creates the config directory if it doesn't exist and writes the config
// in YAML format with user-friendly path formatting (using ~ for home directory).
// It validates that the config directory is within the home directory to prevent symlink attacks.
func Save(cfg *Config) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, configDirName)

	// Resolve symlinks to prevent symlink attacks
	resolvedConfigDir, err := filepath.EvalSymlinks(configDir)
	if err != nil {
		// If directory doesn't exist yet, that's okay - we'll create it
		// But verify the path we're about to create is safe
		if !isPathWithinHome(configDir, homeDir) {
			return fmt.Errorf("config directory path is outside home directory")
		}
		resolvedConfigDir = configDir
	} else {
		// Verify resolved path is within home directory
		if !isPathWithinHome(resolvedConfigDir, homeDir) {
			return fmt.Errorf("config directory resolves to path outside home directory")
		}
	}

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(resolvedConfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Re-resolve after creation to ensure it's still safe
	resolvedConfigDir, err = filepath.EvalSymlinks(resolvedConfigDir)
	if err == nil && !isPathWithinHome(resolvedConfigDir, homeDir) {
		return fmt.Errorf("config directory is outside home directory")
	}

	// Use resolved path for config file
	configPath := filepath.Join(resolvedConfigDir, configFileName+"."+configFileType)

	// Create a copy of config with paths converted to ~ format for readability
	saveCfg := convertPathsToTilde(cfg, homeDir)

	// Marshal config to YAML
	data, err := yaml.Marshal(saveCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// convertPathsToTilde creates a copy of the config with absolute paths
// converted to ~ format if they're within the user's home directory
func convertPathsToTilde(cfg *Config, homeDir string) *Config {
	// Create a copy to avoid modifying the original
	result := &Config{
		WatchedDirectories: make([]string, len(cfg.WatchedDirectories)),
		BlogRepository:     convertPathToTilde(cfg.BlogRepository, homeDir),
		Storage: StorageConfig{
			BasePath:     convertPathToTilde(cfg.Storage.BasePath, homeDir),
			SessionsPath: convertPathToTilde(cfg.Storage.SessionsPath, homeDir),
			DatabasePath: convertPathToTilde(cfg.Storage.DatabasePath, homeDir),
		},
		Cursor: CursorConfig{
			LogPath: convertPathToTilde(cfg.Cursor.LogPath, homeDir),
		},
		Session: cfg.Session,
	}

	// Convert watched directories paths
	for i, dir := range cfg.WatchedDirectories {
		result.WatchedDirectories[i] = convertPathToTilde(dir, homeDir)
	}

	return result
}

// convertPathToTilde converts an absolute path to ~ format if it's within
// the user's home directory, otherwise returns the path as-is.
// If the path already starts with ~, it's returned as-is.
func convertPathToTilde(path, homeDir string) string {
	if path == "" {
		return path
	}

	// If path already starts with ~, return it as-is (it's already in ~ notation)
	if strings.HasPrefix(path, "~") {
		return path
	}

	// Normalize paths for comparison
	absPath, err := filepath.Abs(path)
	if err != nil {
		// If we can't get absolute path, return as-is
		return path
	}

	homeDirAbs, err := filepath.Abs(homeDir)
	if err != nil {
		return path
	}

	// Check if path is within home directory
	relPath, err := filepath.Rel(homeDirAbs, absPath)
	if err != nil {
		return path
	}

	// If relative path doesn't start with "..", it's within home directory
	if !strings.HasPrefix(relPath, "..") {
		if relPath == "." {
			return "~"
		}
		return filepath.Join("~", relPath)
	}

	return path
}
