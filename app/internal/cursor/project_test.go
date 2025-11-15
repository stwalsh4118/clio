package cursor

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stwalsh4118/clio/internal/config"
	_ "modernc.org/sqlite"
)

// createTestWorkspace creates a test workspace directory structure
func createTestWorkspace(t *testing.T, baseDir, workspaceHash, projectPath string, composerIDs []string) {
	workspaceDir := filepath.Join(baseDir, workspaceHash)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("Failed to create workspace directory: %v", err)
	}

	// Create workspace.json
	workspaceJSON := map[string]interface{}{
		"folder": projectPath,
	}
	workspaceJSONData, err := json.Marshal(workspaceJSON)
	if err != nil {
		t.Fatalf("Failed to marshal workspace.json: %v", err)
	}
	workspaceJSONPath := filepath.Join(workspaceDir, "workspace.json")
	if err := os.WriteFile(workspaceJSONPath, workspaceJSONData, 0644); err != nil {
		t.Fatalf("Failed to write workspace.json: %v", err)
	}

	// Create workspace database
	dbPath := filepath.Join(workspaceDir, "state.vscdb")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	// Create ItemTable
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS ItemTable (
		key TEXT UNIQUE ON CONFLICT REPLACE,
		value BLOB
	);`
	if _, err := db.Exec(createTableSQL); err != nil {
		t.Fatalf("Failed to create ItemTable: %v", err)
	}

	// Create composer.composerData entry
	allComposers := make([]map[string]interface{}, 0, len(composerIDs))
	for _, composerID := range composerIDs {
		allComposers = append(allComposers, map[string]interface{}{
			"composerId": composerID,
			"name":       "Test Conversation",
			"createdAt": 1704067200000,
		})
	}

	composerData := map[string]interface{}{
		"allComposers": allComposers,
	}
	composerDataJSON, err := json.Marshal(composerData)
	if err != nil {
		t.Fatalf("Failed to marshal composer data: %v", err)
	}

	key := "composer.composerData"
	if _, err := db.Exec("INSERT INTO ItemTable (key, value) VALUES (?, ?)", key, composerDataJSON); err != nil {
		t.Fatalf("Failed to insert composer data: %v", err)
	}
}

func TestNewProjectDetector(t *testing.T) {
	// Create temporary directory for workspace storage
	tmpDir := t.TempDir()
	workspaceStoragePath := filepath.Join(tmpDir, "workspaceStorage")
	if err := os.MkdirAll(workspaceStoragePath, 0755); err != nil {
		t.Fatalf("Failed to create workspace storage directory: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	detector, err := NewProjectDetector(cfg)
	if err != nil {
		t.Fatalf("Failed to create project detector: %v", err)
	}

	if detector == nil {
		t.Fatal("Project detector is nil")
	}
}

func TestDetectProject_Success(t *testing.T) {
	// Create temporary directory for workspace storage
	tmpDir := t.TempDir()
	workspaceStoragePath := filepath.Join(tmpDir, "workspaceStorage")
	if err := os.MkdirAll(workspaceStoragePath, 0755); err != nil {
		t.Fatalf("Failed to create workspace storage directory: %v", err)
	}

	// Create test workspace
	workspaceHash := "test-workspace-hash-123"
	projectPath := "file:///home/user/my-project"
	composerID := "test-composer-id-456"
	createTestWorkspace(t, workspaceStoragePath, workspaceHash, projectPath, []string{composerID})

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	detector, err := NewProjectDetector(cfg)
	if err != nil {
		t.Fatalf("Failed to create project detector: %v", err)
	}

	// Create test conversation
	conv := &Conversation{
		ComposerID: composerID,
		Name:       "Test Conversation",
		Status:     "completed",
		CreatedAt:  time.Now(),
		Messages:   []Message{},
	}

	project, err := detector.DetectProject(conv)
	if err != nil {
		t.Fatalf("Failed to detect project: %v", err)
	}

	expected := "my-project"
	if project != expected {
		t.Errorf("Expected project name %q, got %q", expected, project)
	}
}

func TestDetectProject_Unknown(t *testing.T) {
	// Create temporary directory for workspace storage
	tmpDir := t.TempDir()
	workspaceStoragePath := filepath.Join(tmpDir, "workspaceStorage")
	if err := os.MkdirAll(workspaceStoragePath, 0755); err != nil {
		t.Fatalf("Failed to create workspace storage directory: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	detector, err := NewProjectDetector(cfg)
	if err != nil {
		t.Fatalf("Failed to create project detector: %v", err)
	}

	// Create test conversation with unknown composer ID
	conv := &Conversation{
		ComposerID: "unknown-composer-id",
		Name:       "Test Conversation",
		Status:     "completed",
		CreatedAt:  time.Now(),
		Messages:   []Message{},
	}

	project, err := detector.DetectProject(conv)
	if err != nil {
		t.Fatalf("Failed to detect project: %v", err)
	}

	expected := "unknown"
	if project != expected {
		t.Errorf("Expected project name %q, got %q", expected, project)
	}
}

func TestDetectProject_NilConversation(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	detector, err := NewProjectDetector(cfg)
	if err != nil {
		t.Fatalf("Failed to create project detector: %v", err)
	}

	project, err := detector.DetectProject(nil)
	if err == nil {
		t.Error("Expected error for nil conversation")
	}

	if project != "unknown" {
		t.Errorf("Expected project name %q, got %q", "unknown", project)
	}
}

func TestDetectProject_EmptyComposerID(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	detector, err := NewProjectDetector(cfg)
	if err != nil {
		t.Fatalf("Failed to create project detector: %v", err)
	}

	conv := &Conversation{
		ComposerID: "",
		Name:       "Test Conversation",
		Status:     "completed",
		CreatedAt:  time.Now(),
		Messages:   []Message{},
	}

	project, err := detector.DetectProject(conv)
	if err == nil {
		t.Error("Expected error for empty composer ID")
	}

	if project != "unknown" {
		t.Errorf("Expected project name %q, got %q", "unknown", project)
	}
}

func TestNormalizeProjectName(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	detector, err := NewProjectDetector(cfg)
	if err != nil {
		t.Fatalf("Failed to create project detector: %v", err)
	}

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "file:// URI",
			input:    "file:///home/user/my-project",
			expected: "my-project",
		},
		{
			name:     "absolute path",
			input:    "/home/user/my-project",
			expected: "my-project",
		},
		{
			name:     "relative path",
			input:    "./my-project",
			expected: "my-project",
		},
		{
			name:     "project with spaces",
			input:    "/home/user/my project",
			expected: "my-project",
		},
		{
			name:     "project with special chars",
			input:    "/home/user/my@project#123",
			expected: "my-project-123",
		},
		{
			name:     "project with uppercase",
			input:    "/home/user/MyProject",
			expected: "myproject",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "unknown",
		},
		{
			name:     "only special chars",
			input:    "@#$%",
			expected: "unknown",
		},
		{
			name:     "path with multiple slashes",
			input:    "/home/user/projects/my-project",
			expected: "my-project",
		},
		{
			name:     "file:// URI with encoded characters",
			input:    "file:///home/user/my%20project",
			expected: "my-project", // URL decoding happens, %20 becomes space, then normalized to dash
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := detector.NormalizeProjectName(tc.input)
			if result != tc.expected {
				t.Errorf("Input %q: expected %q, got %q", tc.input, tc.expected, result)
			}
		})
	}
}

func TestRefreshWorkspaceCache(t *testing.T) {
	// Create temporary directory for workspace storage
	tmpDir := t.TempDir()
	workspaceStoragePath := filepath.Join(tmpDir, "workspaceStorage")
	if err := os.MkdirAll(workspaceStoragePath, 0755); err != nil {
		t.Fatalf("Failed to create workspace storage directory: %v", err)
	}

	// Create multiple test workspaces
	workspace1Hash := "workspace-1"
	workspace1Path := "file:///home/user/project-1"
	composer1ID := "composer-1"
	createTestWorkspace(t, workspaceStoragePath, workspace1Hash, workspace1Path, []string{composer1ID})

	workspace2Hash := "workspace-2"
	workspace2Path := "file:///home/user/project-2"
	composer2ID := "composer-2"
	createTestWorkspace(t, workspaceStoragePath, workspace2Hash, workspace2Path, []string{composer2ID})

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	detector, err := NewProjectDetector(cfg)
	if err != nil {
		t.Fatalf("Failed to create project detector: %v", err)
	}

	// Refresh cache
	if err := detector.RefreshWorkspaceCache(); err != nil {
		t.Fatalf("Failed to refresh workspace cache: %v", err)
	}

	// Verify both composers are detected
	conv1 := &Conversation{ComposerID: composer1ID}
	project1, err := detector.DetectProject(conv1)
	if err != nil {
		t.Fatalf("Failed to detect project for composer 1: %v", err)
	}
	if project1 != "project-1" {
		t.Errorf("Expected project-1, got %q", project1)
	}

	conv2 := &Conversation{ComposerID: composer2ID}
	project2, err := detector.DetectProject(conv2)
	if err != nil {
		t.Fatalf("Failed to detect project for composer 2: %v", err)
	}
	if project2 != "project-2" {
		t.Errorf("Expected project-2, got %q", project2)
	}
}

func TestRefreshWorkspaceCache_MissingWorkspaceJSON(t *testing.T) {
	// Create temporary directory for workspace storage
	tmpDir := t.TempDir()
	workspaceStoragePath := filepath.Join(tmpDir, "workspaceStorage")
	if err := os.MkdirAll(workspaceStoragePath, 0755); err != nil {
		t.Fatalf("Failed to create workspace storage directory: %v", err)
	}

	// Create workspace directory without workspace.json
	workspaceHash := "workspace-no-json"
	workspaceDir := filepath.Join(workspaceStoragePath, workspaceHash)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("Failed to create workspace directory: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	detector, err := NewProjectDetector(cfg)
	if err != nil {
		t.Fatalf("Failed to create project detector: %v", err)
	}

	// Refresh should not fail even if workspace.json is missing
	if err := detector.RefreshWorkspaceCache(); err != nil {
		t.Fatalf("Refresh should handle missing workspace.json gracefully: %v", err)
	}
}

func TestRefreshWorkspaceCache_MissingDatabase(t *testing.T) {
	// Create temporary directory for workspace storage
	tmpDir := t.TempDir()
	workspaceStoragePath := filepath.Join(tmpDir, "workspaceStorage")
	if err := os.MkdirAll(workspaceStoragePath, 0755); err != nil {
		t.Fatalf("Failed to create workspace storage directory: %v", err)
	}

	// Create workspace directory with workspace.json but no database
	workspaceHash := "workspace-no-db"
	workspaceDir := filepath.Join(workspaceStoragePath, workspaceHash)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("Failed to create workspace directory: %v", err)
	}

	workspaceJSON := map[string]interface{}{
		"folder": "file:///home/user/test-project",
	}
	workspaceJSONData, err := json.Marshal(workspaceJSON)
	if err != nil {
		t.Fatalf("Failed to marshal workspace.json: %v", err)
	}
	workspaceJSONPath := filepath.Join(workspaceDir, "workspace.json")
	if err := os.WriteFile(workspaceJSONPath, workspaceJSONData, 0644); err != nil {
		t.Fatalf("Failed to write workspace.json: %v", err)
	}

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	detector, err := NewProjectDetector(cfg)
	if err != nil {
		t.Fatalf("Failed to create project detector: %v", err)
	}

	// Refresh should not fail even if database is missing
	if err := detector.RefreshWorkspaceCache(); err != nil {
		t.Fatalf("Refresh should handle missing database gracefully: %v", err)
	}
}

func TestRefreshWorkspaceCache_EmptyWorkspaceStorage(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	detector, err := NewProjectDetector(cfg)
	if err != nil {
		t.Fatalf("Failed to create project detector: %v", err)
	}

	// Refresh should not fail even if workspace storage doesn't exist
	if err := detector.RefreshWorkspaceCache(); err != nil {
		t.Fatalf("Refresh should handle missing workspace storage gracefully: %v", err)
	}
}

func TestDetectProject_MultipleComposersSameWorkspace(t *testing.T) {
	// Create temporary directory for workspace storage
	tmpDir := t.TempDir()
	workspaceStoragePath := filepath.Join(tmpDir, "workspaceStorage")
	if err := os.MkdirAll(workspaceStoragePath, 0755); err != nil {
		t.Fatalf("Failed to create workspace storage directory: %v", err)
	}

	// Create workspace with multiple composers
	workspaceHash := "workspace-multi"
	projectPath := "file:///home/user/multi-project"
	composerIDs := []string{"composer-1", "composer-2", "composer-3"}
	createTestWorkspace(t, workspaceStoragePath, workspaceHash, projectPath, composerIDs)

	cfg := &config.Config{
		Cursor: config.CursorConfig{
			LogPath: tmpDir,
		},
	}

	detector, err := NewProjectDetector(cfg)
	if err != nil {
		t.Fatalf("Failed to create project detector: %v", err)
	}

	// Verify all composers map to the same project
	for _, composerID := range composerIDs {
		conv := &Conversation{ComposerID: composerID}
		project, err := detector.DetectProject(conv)
		if err != nil {
			t.Fatalf("Failed to detect project for composer %s: %v", composerID, err)
		}
		if project != "multi-project" {
			t.Errorf("Composer %s: expected multi-project, got %q", composerID, project)
		}
	}
}

