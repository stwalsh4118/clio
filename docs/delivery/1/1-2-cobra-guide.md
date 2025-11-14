# Cobra CLI Framework Guide

**Date**: 2025-01-14  
**Task**: 1-2  
**Package**: `github.com/spf13/cobra`  
**Original Documentation**: https://cobra.dev/

## Overview

Cobra is a CLI framework for Go applications that provides:
- Command hierarchy with subcommands
- Automatic help text generation
- Flag parsing (persistent and local flags)
- Command aliases
- Version support
- Shell completion support

## Core Concepts

### Command Structure

A Cobra command is defined using `cobra.Command`:

```go
var rootCmd = &cobra.Command{
    Use:   "clio",                    // Command name and usage
    Short: "Short description",        // Short description shown in help
    Long:  "Long detailed description", // Long description shown in help --help
    Run:   func(cmd *cobra.Command, args []string) {
        // Command execution logic
    },
    RunE: func(cmd *cobra.Command, args []string) error {
        // Command execution with error return
        return nil
    },
}
```

### Key Fields

- **Use**: Command name and usage string (e.g., "clio [command]")
- **Short**: Brief description (shown in command list)
- **Long**: Detailed description (shown with --help)
- **Run**: Function executed when command runs (no error return)
- **RunE**: Function executed when command runs (returns error)
- **Version**: Version string (can be set via `SetVersion()`)

### Creating and Executing Commands

```go
package main

import (
    "github.com/spf13/cobra"
    "os"
)

func main() {
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}
```

### Adding Subcommands

Subcommands are added using `AddCommand()`:

```go
rootCmd.AddCommand(startCmd)
rootCmd.AddCommand(stopCmd)
rootCmd.AddCommand(statusCmd)
rootCmd.AddCommand(configCmd)
```

### Creating Subcommands

```go
var startCmd = &cobra.Command{
    Use:   "start",
    Short: "Start the monitoring daemon",
    Long:  "Start the background monitoring daemon that captures development insights",
    RunE: func(cmd *cobra.Command, args []string) error {
        // Implementation
        return nil
    },
}
```

### Flags

**Persistent Flags** (available to command and all subcommands):
```go
rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file path")
```

**Local Flags** (only available to specific command):
```go
startCmd.Flags().BoolVar(&daemon, "daemon", false, "run as daemon")
```

**String Flags**:
```go
configCmd.Flags().StringVar(&watchPath, "add-watch", "", "add directory to watch list")
```

### Version Support

```go
rootCmd.SetVersion("1.0.0")
rootCmd.Flags().Bool("version", false, "display version")
```

Or use built-in version flag:
```go
rootCmd.Version = "1.0.0"
```

### Error Handling

Cobra commands can return errors:
```go
RunE: func(cmd *cobra.Command, args []string) error {
    if err := doSomething(); err != nil {
        return fmt.Errorf("failed to do something: %w", err)
    }
    return nil
}
```

Errors are automatically displayed to the user.

### Help Text

Cobra automatically generates help text:
- `clio --help` or `clio -h` shows help for root command
- `clio start --help` shows help for start command
- Help includes command description, flags, and subcommands

## Project-Specific Patterns

### Root Command Pattern

For this project, we'll use a factory function pattern:

```go
// internal/cli/root.go
package cli

import "github.com/spf13/cobra"

func NewRootCmd() *cobra.Command {
    rootCmd := &cobra.Command{
        Use:   "clio",
        Short: "Capture and analyze development insights",
        Long: `Clio is a CLI tool that automatically captures and analyzes
development insights from Cursor conversations and git activity.

It monitors your development workflow and stores captured data in a
queryable format for analysis and blog content generation.`,
        Version: "0.1.0",
    }
    
    // Add subcommands
    rootCmd.AddCommand(newStartCmd())
    rootCmd.AddCommand(newStopCmd())
    rootCmd.AddCommand(newStatusCmd())
    rootCmd.AddCommand(newConfigCmd())
    
    return rootCmd
}
```

### Main Entry Point

```go
// cmd/clio/main.go
package main

import (
    "os"
    "github.com/stwalsh4118/clio/internal/cli"
)

func main() {
    rootCmd := cli.NewRootCmd()
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}
```

### Placeholder Subcommands

For commands not yet implemented:

```go
func newStartCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "start",
        Short: "Start the monitoring daemon",
        Long:  "Start the background monitoring daemon (not yet implemented)",
        RunE: func(cmd *cobra.Command, args []string) error {
            return fmt.Errorf("start command not yet implemented")
        },
    }
}
```

## Integration with Viper

Cobra integrates well with Viper (to be added in task 1-3):

```go
import (
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

func init() {
    // Bind flags to viper
    viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
}
```

## Best Practices

1. **Use RunE instead of Run** when you need error handling
2. **Export factory functions** (e.g., `NewRootCmd()`) for testability
3. **Keep command definitions separate** from business logic
4. **Use persistent flags** for settings that apply to all commands
5. **Provide clear help text** in Short and Long fields
6. **Handle errors gracefully** and provide actionable error messages

## References

- Official Documentation: https://cobra.dev/
- GitHub Repository: https://github.com/spf13/cobra
- Getting Started Guide: https://cobra.dev/docs/tutorials/getting-started/

