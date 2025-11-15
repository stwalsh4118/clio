package cli

import (
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
	rootCmd.AddCommand(newDaemonCmd())

	return rootCmd
}

// newStartCmd creates the start command
func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the monitoring daemon",
		Long:  "Start the background monitoring daemon that captures development insights",
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleStart()
		},
	}
}

// newStopCmd creates the stop command
func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the monitoring daemon",
		Long:  "Stop the background monitoring daemon gracefully",
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleStop()
		},
	}
}

// newStatusCmd creates the status command
func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check daemon status",
		Long:  "Check if the monitoring daemon is running",
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleStatus()
		},
	}
}

// newDaemonCmd creates the daemon command (hidden, used internally)
func newDaemonCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "daemon",
		Hidden: true, // Hide from help/usage
		Short:  "Run as daemon (internal use only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleDaemon()
		},
	}
}
