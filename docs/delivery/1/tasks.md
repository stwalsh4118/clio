# Tasks for PBI 1: Foundation & CLI Framework

This document lists all tasks associated with PBI 1.

**Parent PBI**: [PBI 1: Foundation & CLI Framework](./prd.md)

## Task Summary

| Task ID | Name | Status | Description |
| :------ | :--------------------------------------- | :------- | :--------------------------------- |
| 1-1 | [Initialize Go module and project structure](./1-1.md) | Proposed | Set up Go module, directory structure (cmd/, internal/, pkg/), and basic build configuration |
| 1-2 | [Set up Cobra CLI framework with root command](./1-2.md) | Done | Install Cobra, create root command structure, set up command scaffolding |
| 1-3 | [Implement configuration management with Viper](./1-3.md) | Done | Set up Viper for config file management, environment variables, and default values |
| 1-4 | [Create config command with subcommands](./1-4.md) | Done | Implement config command with --show, --add-watch, --set-blog-repo flags |
| 1-5 | [Implement start/stop/status commands](./1-5.md) | Proposed | Basic daemon process management (background process, PID file, graceful shutdown) |
| 1-6 | [Add configuration validation](./1-6.md) | Proposed | Validate config values, paths, and settings with helpful error messages |
| 1-7 | [Create default configuration file on first run](./1-7.md) | Proposed | Auto-create ~/.clio/config.yaml with defaults when missing |
| 1-8 | [E2E CoS Test](./1-8.md) | Proposed | End-to-end testing to verify all 12 acceptance criteria from the PBI |


