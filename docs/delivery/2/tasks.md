# Tasks for PBI 2: Cursor Conversation Capture

This document lists all tasks associated with PBI 2.

**Parent PBI**: [PBI 2: Cursor Conversation Capture](./prd.md)

## Task Summary

| Task ID | Name | Status | Description |
| :------ | :--------------------------------------- | :------- | :--------------------------------- |
| 2-1 | [Research Cursor log format and storage location](./2-1.md) | Done | Investigate Cursor's log format, storage locations (macOS/Linux), and answer open questions from PRD |
| 2-2 | [Implement Cursor log discovery service](./2-2.md) | Done | Enhanced validation for user-configured Cursor log path pointing to User directory |
| 2-3 | [Implement file system watcher for Cursor log directory](./2-3.md) | Done | File system watcher using fsnotify to monitor Cursor log directory for new and modified files |
| 2-4 | [Design and implement conversation parser](./2-4.md) | Done | Parser to extract messages, identify user vs agent responses, and extract metadata from Cursor logs |
| 2-5 | [Implement session tracking logic](./2-5.md) | Proposed | Logic to group conversations into sessions with proper boundary detection |
| 2-6 | [Implement markdown export functionality](./2-6.md) | Proposed | Export functionality to write markdown files in date/project directory structure |
| 2-7 | [Implement project detection mechanism](./2-7.md) | Proposed | Mechanism to detect which project a conversation belongs to |
| 2-8 | [Handle conversation updates and modifications](./2-8.md) | Proposed | Handle updates to existing conversations that are modified after initial capture |
| 2-9 | [Add error handling and logging](./2-9.md) | Proposed | Comprehensive error handling and logging throughout the capture system |
| 2-10 | [E2E CoS Test](./2-10.md) | Proposed | End-to-end test verifying all 11 acceptance criteria from PBI 2 |

