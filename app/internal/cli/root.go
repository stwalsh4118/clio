package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

const (
	version = "0.1.0"
)

// NewRootCmd creates and returns the root command for clio
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "clio",
		Short: "Capture and analyze development insights",
		Long: `Clio is a CLI tool that automatically captures and analyzes
development insights from Cursor conversations and git activity.

It monitors your development workflow and stores captured data in a
queryable format for analysis and blog content generation.`,
		Version: version,
	}

	// Add subcommands
	rootCmd.AddCommand(newStartCmd())
	rootCmd.AddCommand(newStopCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newConfigCmd())

	return rootCmd
}

// newStartCmd creates the start command (placeholder for task 1-5)
func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the monitoring daemon",
		Long:  "Start the background monitoring daemon that captures development insights (not yet implemented)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("start command not yet implemented (task 1-5)")
		},
	}
}

// newStopCmd creates the stop command (placeholder for task 1-5)
func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the monitoring daemon",
		Long:  "Stop the background monitoring daemon (not yet implemented)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("stop command not yet implemented (task 1-5)")
		},
	}
}

// newStatusCmd creates the status command (placeholder for task 1-5)
func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check daemon status",
		Long:  "Check if the monitoring daemon is running (not yet implemented)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("status command not yet implemented (task 1-5)")
		},
	}
}

// newConfigCmd creates the config command (placeholder for task 1-4)
func newConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "View and modify configuration",
		Long:  "View and modify clio configuration settings (not yet implemented)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("config command not yet implemented (task 1-4)")
		},
	}
}
