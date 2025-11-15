package cursor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/stwalsh4118/clio/internal/config"
	"github.com/stwalsh4118/clio/internal/logging"
	_ "modernc.org/sqlite" // SQLite driver
)

const (
	// defaultProjectName is returned when a composer ID cannot be found in any workspace
	defaultProjectName = "unknown"
	// maxProjectNameLength limits the length of normalized project names
	maxProjectNameLength = 255
)

// ProjectDetector defines the interface for detecting which project a conversation belongs to
type ProjectDetector interface {
	DetectProject(conv *Conversation) (string, error)
	NormalizeProjectName(name string) string
	RefreshWorkspaceCache() error
}

// projectDetector implements ProjectDetector using workspace database lookup
type projectDetector struct {
	config                    *config.Config
	logger                    logging.Logger
	workspaceStoragePath      string
	mu                        sync.RWMutex
	workspaceHashToProjectPath map[string]string // workspaceHash → projectPath
	composerIDToWorkspaceHash map[string]string // composerID → workspaceHash
}

// NewProjectDetector creates a new project detector instance
func NewProjectDetector(cfg *config.Config) (ProjectDetector, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Create logger (use component-specific logger)
	logger, err := logging.NewLogger(cfg)
	if err != nil {
		// If logger creation fails, use no-op logger (don't fail detector creation)
		logger = logging.NewNoopLogger()
	}
	logger = logger.With("component", "project_detector")

	workspaceStoragePath := filepath.Join(cfg.Cursor.LogPath, "workspaceStorage")

	detector := &projectDetector{
		config:                    cfg,
		logger:                    logger,
		workspaceStoragePath:      workspaceStoragePath,
		workspaceHashToProjectPath: make(map[string]string),
		composerIDToWorkspaceHash:  make(map[string]string),
	}

	// Perform initial workspace scan
	if err := detector.RefreshWorkspaceCache(); err != nil {
		// Log error but don't fail creation - cache can be refreshed later
		logger.Warn("failed to perform initial workspace scan", "error", err)
	}

	return detector, nil
}

// DetectProject detects which project a conversation belongs to
func (pd *projectDetector) DetectProject(conv *Conversation) (string, error) {
	if conv == nil {
		return pd.NormalizeProjectName(defaultProjectName), fmt.Errorf("conversation cannot be nil")
	}

	if conv.ComposerID == "" {
		return pd.NormalizeProjectName(defaultProjectName), fmt.Errorf("conversation composer ID is empty")
	}

	pd.mu.RLock()
	defer pd.mu.RUnlock()

	// Look up composer ID in cache
	workspaceHash, found := pd.composerIDToWorkspaceHash[conv.ComposerID]
	if !found {
		pd.logger.Debug("composer ID not found in any workspace", "composer_id", conv.ComposerID)
		return pd.NormalizeProjectName(defaultProjectName), nil
	}

	// Look up workspace hash to get project path
	projectPath, found := pd.workspaceHashToProjectPath[workspaceHash]
	if !found {
		pd.logger.Debug("workspace hash not found in cache", "workspace_hash", workspaceHash, "composer_id", conv.ComposerID)
		return pd.NormalizeProjectName(defaultProjectName), nil
	}

	// Normalize and return project name
	projectName := pd.NormalizeProjectName(projectPath)
	pd.logger.Debug("detected project for conversation", "composer_id", conv.ComposerID, "project", projectName)
	return projectName, nil
}

// NormalizeProjectName normalizes a project path or name to a filesystem-safe project name
func (pd *projectDetector) NormalizeProjectName(name string) string {
	if name == "" {
		return defaultProjectName
	}

	// Handle file:// URIs
	if strings.HasPrefix(name, "file://") {
		parsedURL, err := url.Parse(name)
		if err == nil {
			name = parsedURL.Path
		} else {
			// If parsing fails, try to extract path manually
			if idx := strings.Index(name, "://"); idx != -1 {
				if pathIdx := strings.Index(name[idx+3:], "/"); pathIdx != -1 {
					name = name[idx+3+pathIdx:]
				}
			}
		}
	}

	// Extract directory name from full path
	name = filepath.Base(name)

	// Remove special characters that aren't filesystem-safe
	// Keep alphanumeric, dash, underscore, and dot
	reg := regexp.MustCompile(`[^a-zA-Z0-9._-]`)
	name = reg.ReplaceAllString(name, "-")

	// Convert to lowercase for consistency
	name = strings.ToLower(name)

	// Remove consecutive dashes
	reg = regexp.MustCompile(`-+`)
	name = reg.ReplaceAllString(name, "-")

	// Remove leading/trailing dashes
	name = strings.Trim(name, "-")

	// Limit length
	if len(name) > maxProjectNameLength {
		name = name[:maxProjectNameLength]
	}

	// If result is empty after normalization, return default
	if name == "" {
		return defaultProjectName
	}

	return name
}

// RefreshWorkspaceCache scans workspace directories and rebuilds the cache
func (pd *projectDetector) RefreshWorkspaceCache() error {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	// Clear existing cache
	pd.workspaceHashToProjectPath = make(map[string]string)
	pd.composerIDToWorkspaceHash = make(map[string]string)

	// Check if workspace storage directory exists
	_, err := os.Stat(pd.workspaceStoragePath)
	if err != nil {
		if os.IsNotExist(err) {
			pd.logger.Debug("workspace storage directory does not exist", "path", pd.workspaceStoragePath)
			return nil // Not an error - just no workspaces yet
		}
		return fmt.Errorf("failed to check workspace storage directory: %w", err)
	}

	// Read workspace directories
	entries, err := os.ReadDir(pd.workspaceStoragePath)
	if err != nil {
		return fmt.Errorf("failed to read workspace storage directory: %w", err)
	}

	workspaceCount := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		workspaceHash := entry.Name()
		workspaceDir := filepath.Join(pd.workspaceStoragePath, workspaceHash)

		// Read workspace.json to get project path
		projectPath, err := pd.readWorkspaceJSON(workspaceDir, workspaceHash)
		if err != nil {
			pd.logger.Debug("failed to read workspace.json", "workspace_hash", workspaceHash, "error", err)
			continue // Skip this workspace but continue with others
		}

		if projectPath != "" {
			pd.workspaceHashToProjectPath[workspaceHash] = projectPath
		}

		// Query workspace database for composer IDs
		if err := pd.scanWorkspaceDatabase(workspaceDir, workspaceHash); err != nil {
			pd.logger.Debug("failed to scan workspace database", "workspace_hash", workspaceHash, "error", err)
			continue // Skip this workspace but continue with others
		}

		workspaceCount++
	}

	pd.logger.Info("refreshed workspace cache", "workspace_count", workspaceCount, "composer_count", len(pd.composerIDToWorkspaceHash))
	return nil
}

// readWorkspaceJSON reads workspace.json and extracts the project path
func (pd *projectDetector) readWorkspaceJSON(workspaceDir, workspaceHash string) (string, error) {
	workspaceJSONPath := filepath.Join(workspaceDir, "workspace.json")

	// Check if file exists
	_, err := os.Stat(workspaceJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // Not an error - workspace.json is optional
		}
		return "", fmt.Errorf("failed to check workspace.json: %w", err)
	}

	// Read file
	data, err := os.ReadFile(workspaceJSONPath)
	if err != nil {
		return "", fmt.Errorf("failed to read workspace.json: %w", err)
	}

	// Parse JSON
	var workspaceData struct {
		Folder string `json:"folder"`
	}
	if err := json.Unmarshal(data, &workspaceData); err != nil {
		return "", fmt.Errorf("failed to parse workspace.json: %w", err)
	}

	return workspaceData.Folder, nil
}

// scanWorkspaceDatabase queries a workspace database for composer IDs
func (pd *projectDetector) scanWorkspaceDatabase(workspaceDir, workspaceHash string) error {
	dbPath := filepath.Join(workspaceDir, "state.vscdb")

	// Check if database exists
	_, err := os.Stat(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Not an error - database may not exist yet
		}
		return fmt.Errorf("failed to check database: %w", err)
	}

	// Open database in read-only mode
	dsn := fmt.Sprintf("file:%s?mode=ro", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("failed to open workspace database: %w", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping workspace database: %w", err)
	}

	// Query for composer.composerData
	query := "SELECT value FROM ItemTable WHERE key = 'composer.composerData'"
	var valueBlob []byte
	err = db.QueryRow(query).Scan(&valueBlob)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // Not an error - no composers in this workspace yet
		}
		return fmt.Errorf("failed to query composer data: %w", err)
	}

	// Parse JSON
	var composerData struct {
		AllComposers []struct {
			ComposerID string `json:"composerId"`
		} `json:"allComposers"`
	}
	if err := json.Unmarshal(valueBlob, &composerData); err != nil {
		return fmt.Errorf("failed to parse composer data JSON: %w", err)
	}

	// Build composer ID → workspace hash mapping
	for _, composer := range composerData.AllComposers {
		if composer.ComposerID != "" {
			pd.composerIDToWorkspaceHash[composer.ComposerID] = workspaceHash
		}
	}

	return nil
}

