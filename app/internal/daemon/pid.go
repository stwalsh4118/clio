package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	pidFileName = "clio.pid"
)

// GetPIDFilePath returns the absolute path to the PID file.
// The PID file is stored at ~/.clio/clio.pid
func GetPIDFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, configDirName)
	pidPath := filepath.Join(configDir, pidFileName)

	// Expand and resolve the path
	absPath, err := filepath.Abs(pidPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve PID file path: %w", err)
	}

	return absPath, nil
}

// WritePID writes the PID to the PID file.
// It creates the .clio directory if it doesn't exist.
// Uses restrictive permissions (0600) to prevent other users from reading/writing.
func WritePID(pid int) error {
	pidPath, err := GetPIDFilePath()
	if err != nil {
		return fmt.Errorf("failed to get PID file path: %w", err)
	}

	// Get the directory and ensure it exists
	pidDir := filepath.Dir(pidPath)

	// Resolve symlinks to prevent symlink attacks
	resolvedDir, err := filepath.EvalSymlinks(pidDir)
	if err != nil {
		// Directory doesn't exist yet, that's okay
		// But verify the path we're about to create is safe
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		if !isPathWithinHome(pidDir, homeDir) {
			return fmt.Errorf("PID directory path is outside home directory")
		}
		resolvedDir = pidDir
	} else {
		// Verify resolved path is within home directory
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		if !isPathWithinHome(resolvedDir, homeDir) {
			return fmt.Errorf("PID directory resolves to path outside home directory")
		}
	}

	// Create directory with restrictive permissions (0700 - owner only)
	if err := os.MkdirAll(resolvedDir, 0700); err != nil {
		return fmt.Errorf("failed to create PID file directory: %w", err)
	}

	// Re-resolve after creation to ensure it's still safe
	resolvedDir, err = filepath.EvalSymlinks(resolvedDir)
	if err == nil {
		homeDir, err := os.UserHomeDir()
		if err == nil && !isPathWithinHome(resolvedDir, homeDir) {
			return fmt.Errorf("PID directory is outside home directory")
		}
	}

	// Use resolved path for PID file
	pidPath = filepath.Join(resolvedDir, pidFileName)

	// Write PID to file with restrictive permissions (0600 - owner read/write only)
	pidStr := strconv.Itoa(pid)
	if err := os.WriteFile(pidPath, []byte(pidStr+"\n"), 0600); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	return nil
}

// ReadPID reads the PID from the PID file.
// Returns an error if the file doesn't exist or contains invalid data.
func ReadPID() (int, error) {
	pidPath, err := GetPIDFilePath()
	if err != nil {
		return 0, fmt.Errorf("failed to get PID file path: %w", err)
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("PID file does not exist")
		}
		return 0, fmt.Errorf("failed to read PID file: %w", err)
	}

	// Parse PID from file content
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file: %w", err)
	}

	if pid <= 0 {
		return 0, fmt.Errorf("invalid PID value: %d", pid)
	}

	return pid, nil
}

// RemovePIDFile removes the PID file.
// It handles the case where the file doesn't exist gracefully.
func RemovePIDFile() error {
	pidPath, err := GetPIDFilePath()
	if err != nil {
		return fmt.Errorf("failed to get PID file path: %w", err)
	}

	if err := os.Remove(pidPath); err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, which is fine
			return nil
		}
		return fmt.Errorf("failed to remove PID file: %w", err)
	}

	return nil
}

// PIDFileExists checks if the PID file exists.
func PIDFileExists() (bool, error) {
	pidPath, err := GetPIDFilePath()
	if err != nil {
		return false, fmt.Errorf("failed to get PID file path: %w", err)
	}

	_, err = os.Stat(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check PID file: %w", err)
	}

	return true, nil
}

// configDirName matches the constant from config package
const configDirName = ".clio"

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
