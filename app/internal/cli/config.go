package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/stwalsh4118/clio/internal/config"
	"gopkg.in/yaml.v3"
)

// newConfigCmd creates the config command with subcommands for viewing and modifying configuration
func newConfigCmd() *cobra.Command {
	var showFlag bool
	var addWatchPath string
	var setBlogRepoPath string

	cmd := &cobra.Command{
		Use:   "config",
		Short: "View and modify configuration",
		Long: `View and modify clio configuration settings.

Use --show to display current configuration, --add-watch to add a directory
to the watch list, or --set-blog-repo to set the blog repository path.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Count how many flags are set
			flagCount := 0
			if showFlag {
				flagCount++
			}
			if addWatchPath != "" {
				flagCount++
			}
			if setBlogRepoPath != "" {
				flagCount++
			}

			// If no flags provided, show help
			if flagCount == 0 {
				return cmd.Help()
			}

			// Ensure only one flag is used at a time
			if flagCount > 1 {
				return fmt.Errorf("only one flag can be used at a time")
			}

			// Load current configuration
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			// Handle --show flag
			if showFlag {
				return handleShow(cfg)
			}

			// Handle --add-watch flag
			if addWatchPath != "" {
				return handleAddWatch(cfg, addWatchPath)
			}

			// Handle --set-blog-repo flag
			if setBlogRepoPath != "" {
				return handleSetBlogRepo(cfg, setBlogRepoPath)
			}

			return nil
		},
	}

	// Add flags
	cmd.Flags().BoolVarP(&showFlag, "show", "s", false, "Display current configuration")
	cmd.Flags().StringVar(&addWatchPath, "add-watch", "", "Add directory to watched directories list")
	cmd.Flags().StringVar(&setBlogRepoPath, "set-blog-repo", "", "Set blog repository path")

	return cmd
}

// handleShow displays the current configuration in YAML format
func handleShow(cfg *config.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// handleAddWatch adds a directory to the watched directories list
func handleAddWatch(cfg *config.Config, path string) error {
	// Validate path
	if err := config.ValidatePath(path); err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check for duplicates
	if config.IsDuplicate(path, cfg.WatchedDirectories) {
		return fmt.Errorf("directory already in watch list: %s", path)
	}

	// Add to watched directories
	cfg.WatchedDirectories = append(cfg.WatchedDirectories, path)

	// Validate entire configuration before saving
	if err := config.ValidateConfig(cfg); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Save configuration
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Added %s to watched directories\n", path)
	return nil
}

// handleSetBlogRepo sets the blog repository path
func handleSetBlogRepo(cfg *config.Config, path string) error {
	// Validate path
	if err := config.ValidatePath(path); err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Set blog repository
	cfg.BlogRepository = path

	// Validate entire configuration before saving
	if err := config.ValidateConfig(cfg); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Save configuration
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Set blog repository to %s\n", path)
	return nil
}
