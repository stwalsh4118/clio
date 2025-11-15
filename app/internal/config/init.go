package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	configFilePerm = 0600 // Read/write for user only
	configDirPerm  = 0755 // Read/write/execute for user, read/execute for group/others
)

// EnsureConfigFile ensures that the configuration file exists.
// If it doesn't exist, creates it with default values.
// This should be called before loading configuration to ensure the file exists.
// Security: Resolves symlinks and validates paths are within home directory to prevent symlink attacks.
func EnsureConfigFile() error {
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

	configPath := filepath.Join(resolvedConfigDir, configFileName+"."+configFileType)

	// Check if config file already exists (using resolved path)
	if _, err := os.Stat(configPath); err == nil {
		// Config file exists - ensure storage directories exist for validation
		// This handles the case where config exists but directories were deleted
		if err := ensureStorageDirectories(); err != nil {
			return fmt.Errorf("failed to ensure storage directories: %w", err)
		}
		return nil
	} else if !os.IsNotExist(err) {
		// Some other error checking file
		return fmt.Errorf("failed to check config file: %w", err)
	}

	// Config file doesn't exist - create it with defaults
	// Use resolved config directory to prevent symlink attacks
	if err := ensureConfigDirectoryWithPath(resolvedConfigDir, homeDir); err != nil {
		return fmt.Errorf("failed to ensure config directory: %w", err)
	}

	if err := CreateDefaultConfig(); err != nil {
		return fmt.Errorf("failed to create default config: %w", err)
	}

	return nil
}

// ensureStorageDirectories ensures that storage directories exist.
// This is called when config file exists to ensure directories are present for validation.
// Security: Resolves symlinks and validates paths are within home directory.
func ensureStorageDirectories() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	// Ensure home directory exists (parent of storage base path)
	// This should always exist, but check to be safe
	if _, err := os.Stat(homeDir); err != nil {
		return fmt.Errorf("home directory does not exist: %w", err)
	}

	// Ensure ~/.clio/ exists (storage base path)
	storageBasePath := filepath.Join(homeDir, configDirName)

	// Resolve symlinks to prevent symlink attacks
	resolvedPath, err := filepath.EvalSymlinks(storageBasePath)
	if err != nil {
		// Path doesn't exist yet - verify it's safe before creating
		if !isPathWithinHome(storageBasePath, homeDir) {
			return fmt.Errorf("storage base path is outside home directory")
		}
		resolvedPath = storageBasePath
	} else {
		// Verify resolved path is within home directory
		if !isPathWithinHome(resolvedPath, homeDir) {
			return fmt.Errorf("storage base path resolves to path outside home directory")
		}
	}

	// Create directory using resolved path
	if err := os.MkdirAll(resolvedPath, configDirPerm); err != nil {
		return fmt.Errorf("failed to create storage base path: %w", err)
	}

	// Re-resolve after creation to ensure it's still safe
	finalResolved, err := filepath.EvalSymlinks(resolvedPath)
	if err == nil && !isPathWithinHome(finalResolved, homeDir) {
		return fmt.Errorf("storage base path is outside home directory after creation")
	}

	return nil
}

// EnsureConfigDirectory ensures that the ~/.clio/ directory exists.
// Creates it with appropriate permissions if it doesn't exist.
// Security: Resolves symlinks and validates paths are within home directory.
func EnsureConfigDirectory() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, configDirName)
	return ensureConfigDirectoryWithPath(configDir, homeDir)
}

// ensureConfigDirectoryWithPath ensures the config directory exists at the given path.
// Security: Resolves symlinks and validates path is within home directory.
func ensureConfigDirectoryWithPath(configDir, homeDir string) error {
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

	// Check if directory already exists (using resolved path)
	if info, err := os.Stat(resolvedConfigDir); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("config path exists but is not a directory: %s", resolvedConfigDir)
		}
		// Directory exists, verify it's still safe
		// Re-resolve to catch any symlink changes
		finalResolved, err := filepath.EvalSymlinks(resolvedConfigDir)
		if err == nil && !isPathWithinHome(finalResolved, homeDir) {
			return fmt.Errorf("config directory is outside home directory")
		}
		// Note: We don't change permissions if directory already exists to avoid
		// breaking user's existing setup
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check config directory: %w", err)
	}

	// Directory doesn't exist - create it using resolved path
	if err := os.MkdirAll(resolvedConfigDir, configDirPerm); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Re-resolve after creation to ensure it's still safe
	finalResolved, err := filepath.EvalSymlinks(resolvedConfigDir)
	if err == nil && !isPathWithinHome(finalResolved, homeDir) {
		return fmt.Errorf("config directory is outside home directory after creation")
	}

	return nil
}

// CreateDefaultConfig creates the default configuration file with sensible defaults.
// The default configuration matches the PRD schema and should pass validation.
// Security: Uses Save() which has proper symlink protection, but we also validate
// the directory path here for defense in depth.
func CreateDefaultConfig() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, configDirName)

	// Resolve symlinks to prevent symlink attacks (defense in depth)
	// Save() also does this, but we check here too
	resolvedConfigDir, err := filepath.EvalSymlinks(configDir)
	if err != nil {
		// Directory might not exist yet - Save() will create it safely
		if !isPathWithinHome(configDir, homeDir) {
			return fmt.Errorf("config directory path is outside home directory")
		}
		resolvedConfigDir = configDir
	} else {
		if !isPathWithinHome(resolvedConfigDir, homeDir) {
			return fmt.Errorf("config directory resolves to path outside home directory")
		}
	}

	configPath := filepath.Join(resolvedConfigDir, configFileName+"."+configFileType)

	// Create default config struct matching PRD schema
	// Use ~ notation for paths (will be expanded when loaded)
	defaultCfg := &Config{
		WatchedDirectories: []string{}, // Empty list
		BlogRepository:     "",         // Empty string
		Storage: StorageConfig{
			BasePath:     "~/" + configDirName,
			SessionsPath: "~/" + configDirName + "/sessions",
			DatabasePath: "~/" + configDirName + "/clio.db",
		},
		Cursor: CursorConfig{
			LogPath: "", // User must configure this explicitly
		},
		Session: SessionConfig{
			InactivityTimeoutMinutes: 30,
		},
	}

	// Ensure storage base path directory exists (we created ~/.clio/ but validation
	// might check the exact path from config)
	expandedCfg := *defaultCfg
	expandConfigPaths(&expandedCfg)

	// Create storage base path directory if it doesn't exist
	if expandedCfg.Storage.BasePath != "" {
		if err := os.MkdirAll(expandedCfg.Storage.BasePath, configDirPerm); err != nil {
			return fmt.Errorf("failed to create storage base path: %w", err)
		}
	}

	// Validate default config before saving
	// Note: We validate with paths expanded, but we need to be lenient for initial setup
	// Some paths (like cursor log path) might not exist yet, which is okay for defaults
	// For default config, we skip validation of optional paths that don't exist yet
	// The config will be validated again when actually used, at which point
	// the user can set these paths if needed
	// We only validate that the structure is correct and required paths are valid
	if err := validateDefaultConfig(&expandedCfg); err != nil {
		return fmt.Errorf("default configuration validation failed: %w", err)
	}

	// Save the default configuration
	if err := Save(defaultCfg); err != nil {
		return fmt.Errorf("failed to save default config: %w", err)
	}

	// Set file permissions explicitly (Save uses 0644, we want 0600)
	if err := os.Chmod(configPath, configFilePerm); err != nil {
		return fmt.Errorf("failed to set config file permissions: %w", err)
	}

	return nil
}

// validateDefaultConfig validates the default configuration with lenient rules
// for initial setup. It ensures the config structure is valid and required paths
// are accessible, but allows optional paths (like cursor log path) to not exist yet.
func validateDefaultConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}

	// Validate session config (required and should always be valid)
	if err := ValidateSessionConfig(cfg.Session); err != nil {
		return fmt.Errorf("session: %v", err)
	}

	// Validate storage paths - base path parent should exist and be writable
	// The base path itself might not exist yet, which is okay
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	// Check that home directory exists and is writable (required for storage base path)
	if err := checkWritable(homeDir); err != nil {
		return fmt.Errorf("home directory is not writable: %w", err)
	}

	// Validate storage base path structure (parent must exist and be writable)
	expandedBasePath := expandHomeDir(cfg.Storage.BasePath)
	parentDir := filepath.Dir(expandedBasePath)
	parentInfo, err := os.Stat(parentDir)
	if err != nil {
		return fmt.Errorf("storage base path parent directory does not exist: %w", err)
	}
	if !parentInfo.IsDir() {
		return fmt.Errorf("storage base path parent is not a directory")
	}
	if err := checkWritable(parentDir); err != nil {
		return fmt.Errorf("storage base path parent is not writable: %w", err)
	}

	// Validate database path parent exists and is writable
	if cfg.Storage.DatabasePath != "" {
		expandedDatabasePath := expandHomeDir(cfg.Storage.DatabasePath)
		dbParentDir := filepath.Dir(expandedDatabasePath)
		dbParentInfo, err := os.Stat(dbParentDir)
		if err != nil {
			return fmt.Errorf("storage database path parent directory does not exist: %w", err)
		}
		if !dbParentInfo.IsDir() {
			return fmt.Errorf("storage database path parent is not a directory")
		}
		if err := checkWritable(dbParentDir); err != nil {
			return fmt.Errorf("storage database path parent is not writable: %w", err)
		}
	}

	// Cursor log path is optional - don't validate if it doesn't exist yet
	// Watched directories can be empty - that's valid
	// Blog repository can be empty - that's valid

	return nil
}
