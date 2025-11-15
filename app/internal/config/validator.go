package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
			return fmt.Errorf("path does not exist: %s", path)
		}
		return fmt.Errorf("failed to check path: %w", err)
	}

	// Check if path is a directory
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
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
