package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// ValidatePath validates that a path exists and is a directory.
// It expands home directory paths (~) before validation and checks for security issues.
// Returns an error with a helpful message if validation fails.
func ValidatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	// Validate input doesn't contain dangerous characters
	if err := validatePathInput(path); err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Expand home directory path
	expandedPath := expandHomeDir(path)

	// Resolve symlinks to prevent symlink attacks
	resolvedPath, err := filepath.EvalSymlinks(expandedPath)
	if err != nil {
		// If symlink resolution fails, use expanded path
		// This could be a real error or the path doesn't exist yet
		resolvedPath = expandedPath
	}

	// Check if path exists
	info, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist")
		}
		return fmt.Errorf("failed to check path: %w", err)
	}

	// Check if path is a directory
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory")
	}

	return nil
}

// validatePathInput checks for dangerous characters in path input
func validatePathInput(path string) error {
	// Check for null bytes
	if strings.ContainsRune(path, '\x00') {
		return fmt.Errorf("path contains null byte")
	}

	// Check for control characters (except newline/tab which might be valid in some contexts)
	for _, r := range path {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return fmt.Errorf("path contains control character")
		}
	}

	return nil
}

// IsDuplicate checks if a path (after expansion) already exists in the given slice of paths.
// It compares paths after expanding home directory notation and normalizing.
// Returns true if the path is a duplicate, false otherwise.
func IsDuplicate(path string, paths []string) bool {
	if path == "" {
		return false
	}

	expandedPath := expandHomeDir(path)
	expandedPathAbs, err := filepath.Abs(expandedPath)
	if err != nil {
		// If we can't get absolute path, do simple string comparison
		expandedPathAbs = expandedPath
	}

	for _, existingPath := range paths {
		if existingPath == "" {
			continue
		}

		expandedExisting := expandHomeDir(existingPath)
		expandedExistingAbs, err := filepath.Abs(expandedExisting)
		if err != nil {
			expandedExistingAbs = expandedExisting
		}

		// Compare normalized paths
		if expandedPathAbs == expandedExistingAbs {
			return true
		}

		// Also check if paths are the same after resolving symlinks
		// This handles cases where paths might be different representations
		// of the same directory
		resolvedPath, err1 := filepath.EvalSymlinks(expandedPathAbs)
		resolvedExisting, err2 := filepath.EvalSymlinks(expandedExistingAbs)

		if err1 == nil && err2 == nil && resolvedPath == resolvedExisting {
			return true
		}
	}

	return false
}

// ValidateWatchedDirectories validates that all watched directories exist and are readable.
// Returns an error with details about any invalid directories.
// Security: Restricts watched directories to be within the user's home directory to prevent
// watching sensitive system directories or other users' directories.
func ValidateWatchedDirectories(dirs []string) error {
	if len(dirs) == 0 {
		// Empty list is valid - user may not have any watched directories yet
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	var errors []string
	for i, dir := range dirs {
		if dir == "" {
			errors = append(errors, fmt.Sprintf("watched directory %d is empty", i+1))
			continue
		}

		// Expand and resolve path
		expandedPath := expandHomeDir(dir)
		resolvedPath, err := filepath.EvalSymlinks(expandedPath)
		if err != nil {
			resolvedPath = expandedPath
		}

		// Security: Restrict watched directories to be within home directory
		// This prevents watching sensitive system directories like /etc, /root, /usr, etc.
		if !isPathWithinHome(resolvedPath, homeDir) {
			errors = append(errors, fmt.Sprintf("watched directory %d: path must be within your home directory for security", i+1))
			continue
		}

		// Check if path is a sensitive system directory (even if within home)
		if isSensitiveSystemDirectory(resolvedPath) {
			errors = append(errors, fmt.Sprintf("watched directory %d: cannot watch sensitive system directory", i+1))
			continue
		}

		if err := ValidatePath(dir); err != nil {
			errors = append(errors, fmt.Sprintf("watched directory %d: %v", i+1, err))
			continue
		}

		// Check if directory is readable
		file, err := os.Open(resolvedPath)
		if err != nil {
			errors = append(errors, fmt.Sprintf("watched directory %d: cannot read directory: %v", i+1, err))
			continue
		}
		file.Close()
	}

	if len(errors) > 0 {
		return fmt.Errorf("invalid watched directories:\n  %s", strings.Join(errors, "\n  "))
	}

	return nil
}

// ValidateBlogRepository validates that the blog repository path exists, is a directory,
// and optionally checks if it's a git repository.
// Returns an error with a helpful message if validation fails.
func ValidateBlogRepository(path string) error {
	if path == "" {
		// Blog repository is optional, empty is valid
		return nil
	}

	// Validate path exists and is a directory
	if err := ValidatePath(path); err != nil {
		return fmt.Errorf("blog repository path is invalid: %w", err)
	}

	// Optionally check if it's a git repository
	expandedPath := expandHomeDir(path)
	resolvedPath, err := filepath.EvalSymlinks(expandedPath)
	if err != nil {
		resolvedPath = expandedPath
	}

	gitDir := filepath.Join(resolvedPath, ".git")
	gitInfo, err := os.Stat(gitDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Not a git repository, but that's okay - it's optional
			// We'll just warn but not fail
			return nil
		}
		// Some other error checking .git - ignore it
		return nil
	}

	// Check if .git is a directory (normal git repo) or file (git worktree)
	if !gitInfo.IsDir() {
		// Could be a git worktree, which uses a file instead of directory
		// This is still valid
		return nil
	}

	// It's a git repository - validation passed
	return nil
}

// ValidateStoragePaths validates that storage paths are valid and writable.
// Checks that base path exists and is writable, and that sessions/database paths are valid.
func ValidateStoragePaths(storage StorageConfig) error {
	if storage.BasePath == "" {
		return fmt.Errorf("storage base path cannot be empty")
	}

	// Expand and resolve base path
	expandedBasePath := expandHomeDir(storage.BasePath)
	resolvedBasePath, err := filepath.EvalSymlinks(expandedBasePath)
	if err != nil {
		// Path might not exist yet - check if we can create it
		parentDir := filepath.Dir(expandedBasePath)
		parentInfo, err := os.Stat(parentDir)
		if err != nil {
			return fmt.Errorf("storage base path parent directory does not exist")
		}
		if !parentInfo.IsDir() {
			return fmt.Errorf("storage base path parent is not a directory")
		}
		// Check if parent is writable so we can create the base path
		if err := checkWritable(parentDir); err != nil {
			return fmt.Errorf("storage base path parent is not writable: %w", err)
		}
		// Parent exists and is writable, we can create the base path - that's okay
		// No need to validate the non-existent path further
	} else {
		// Path exists - check if it's a directory
		info, err := os.Stat(resolvedBasePath)
		if err != nil {
			return fmt.Errorf("failed to check storage base path: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("storage base path is not a directory")
		}

		// Check if directory is writable
		if err := checkWritable(resolvedBasePath); err != nil {
			return fmt.Errorf("storage base path is not writable: %w", err)
		}
	}

	// Validate sessions path (must be valid if provided)
	if storage.SessionsPath != "" {
		expandedSessionsPath := expandHomeDir(storage.SessionsPath)
		// Check if path is within base path or can be created
		// For now, just validate the path structure is valid
		// The actual directory will be created when needed
		if err := validatePathStructure(expandedSessionsPath); err != nil {
			return fmt.Errorf("storage sessions path is invalid: %w", err)
		}
	}

	// Validate database path (must be valid if provided)
	if storage.DatabasePath != "" {
		expandedDatabasePath := expandHomeDir(storage.DatabasePath)
		// Check if parent directory exists and is writable
		parentDir := filepath.Dir(expandedDatabasePath)
		parentInfo, err := os.Stat(parentDir)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("storage database path parent directory does not exist")
			}
			return fmt.Errorf("failed to check storage database path parent: %w", err)
		}
		if !parentInfo.IsDir() {
			return fmt.Errorf("storage database path parent is not a directory")
		}
		// Check if parent is writable
		if err := checkWritable(parentDir); err != nil {
			return fmt.Errorf("storage database path parent is not writable: %w", err)
		}
	}

	return nil
}

// ValidateCursorPath validates that the cursor log path exists, is a directory, and is readable.
// The path is required for Cursor capture functionality.
func ValidateCursorPath(path string) error {
	if path == "" {
		return fmt.Errorf("cursor log path is required")
	}

	// Expand and resolve path
	expandedPath := expandHomeDir(path)
	resolvedPath, err := filepath.EvalSymlinks(expandedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cursor log path does not exist")
		}
		// Some other error checking the path - return error
		return fmt.Errorf("failed to check cursor log path: %w", err)
	}

	// Check if path exists and is a directory
	info, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cursor log path does not exist")
		}
		return fmt.Errorf("failed to check cursor log path: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("cursor log path is not a directory")
	}

	// Check if directory is readable
	file, err := os.Open(resolvedPath)
	if err != nil {
		return fmt.Errorf("cursor log path is not readable: %w", err)
	}
	file.Close()

	return nil
}

// ValidateSessionConfig validates that session configuration values are valid.
// Checks that inactivity timeout is a positive number.
func ValidateSessionConfig(session SessionConfig) error {
	if session.InactivityTimeoutMinutes <= 0 {
		return fmt.Errorf("session inactivity timeout must be a positive number, got: %d", session.InactivityTimeoutMinutes)
	}

	return nil
}

// ValidateConfig validates the entire configuration structure.
// It calls all individual validators and returns a comprehensive error if any validation fails.
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}

	var errors []string

	// Validate watched directories
	if err := ValidateWatchedDirectories(cfg.WatchedDirectories); err != nil {
		errors = append(errors, fmt.Sprintf("watched directories: %v", err))
	}

	// Validate blog repository
	if err := ValidateBlogRepository(cfg.BlogRepository); err != nil {
		errors = append(errors, fmt.Sprintf("blog repository: %v", err))
	}

	// Validate storage paths
	if err := ValidateStoragePaths(cfg.Storage); err != nil {
		errors = append(errors, fmt.Sprintf("storage: %v", sanitizeError(err)))
	}

	// Validate cursor path
	if err := ValidateCursorPath(cfg.Cursor.LogPath); err != nil {
		errors = append(errors, fmt.Sprintf("cursor log path: %v", sanitizeError(err)))
	}

	// Validate session config
	if err := ValidateSessionConfig(cfg.Session); err != nil {
		errors = append(errors, fmt.Sprintf("session: %v", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed:\n  %s", strings.Join(errors, "\n  "))
	}

	return nil
}

// checkWritable checks if a directory is writable by attempting to create a test file.
// Security: Uses a unique timestamped filename to prevent collisions and ensures
// we use the resolved path to prevent symlink race conditions.
func checkWritable(dirPath string) error {
	// Resolve symlinks to prevent symlink race condition attacks
	resolvedPath, err := filepath.EvalSymlinks(dirPath)
	if err != nil {
		// If symlink resolution fails, use original path
		resolvedPath = dirPath
	}

	// Use a unique timestamped filename to prevent collisions with existing files
	// and reduce the window for race condition attacks
	timestamp := time.Now().UnixNano()
	testFile := filepath.Join(resolvedPath, fmt.Sprintf(".clio-write-test-%d", timestamp))

	// Create test file
	file, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("cannot write to directory")
	}
	file.Close()

	// Immediately remove the test file
	// Use RemoveAll to handle edge cases, but we know it's a file so Remove would work too
	if err := os.Remove(testFile); err != nil {
		// If removal fails, try to remove it anyway but don't fail validation
		// The file will be cleaned up eventually
		_ = os.Remove(testFile)
	}

	return nil
}

// validatePathStructure validates that a path structure is valid (doesn't check existence).
func validatePathStructure(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	// Validate input doesn't contain dangerous characters
	if err := validatePathInput(path); err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	return nil
}

// isSensitiveSystemDirectory checks if a path is a sensitive system directory
// that should not be watched for security reasons.
func isSensitiveSystemDirectory(path string) bool {
	// Get absolute path for comparison
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// Normalize path separators
	absPath = filepath.Clean(absPath)

	// List of sensitive system directories that should never be watched
	sensitiveDirs := []string{
		"/etc",
		"/usr",
		"/bin",
		"/sbin",
		"/lib",
		"/lib64",
		"/sys",
		"/proc",
		"/dev",
		"/boot",
		"/root",
		"/var/log",
		"/var/run",
		"/tmp",
		"/var/tmp",
	}

	// Check if path is exactly one of these directories or a subdirectory
	for _, sensitiveDir := range sensitiveDirs {
		if absPath == sensitiveDir || strings.HasPrefix(absPath+"/", sensitiveDir+"/") {
			return true
		}
	}

	return false
}

// sanitizeError removes sensitive path information from error messages
// to prevent information disclosure. It preserves the error type and message
// but removes full path details that could leak filesystem structure.
func sanitizeError(err error) error {
	if err == nil {
		return nil
	}

	errStr := err.Error()
	// Remove common path patterns that might leak sensitive information
	// This is a simple sanitization - we preserve the error message structure
	// but remove specific path details
	if strings.Contains(errStr, ": ") {
		// If error contains a colon, it might have a path after it
		// Keep the part before the colon (the error type)
		parts := strings.SplitN(errStr, ": ", 2)
		if len(parts) == 2 {
			// Check if the second part looks like a path
			if strings.HasPrefix(parts[1], "/") || strings.Contains(parts[1], "~") {
				// It's likely a path - return just the error type
				return fmt.Errorf("%s", parts[0])
			}
		}
	}

	// Return error as-is if it doesn't contain obvious path information
	return err
}
