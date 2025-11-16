# Tasks for PBI 3: Git Activity Capture

This document lists all tasks associated with PBI 3.

**Parent PBI**: [PBI 3: Git Activity Capture](./prd.md)

## Task Summary

| Task ID | Name | Status | Description |
| :------ | :--------------------------------------- | :------- | :--------------------------------- |
| 3-1 | [Research git library and design git commit monitoring strategy](./3-1.md) | Review | Research go-git library API, answer open questions from PRD, and design polling strategy |
| 3-2 | [Implement git repository discovery service](./3-2.md) | Done | Scan watched directories for git repositories and track repository paths and metadata |
| 3-3 | [Implement git commit polling mechanism](./3-3.md) | Done | Implement periodic polling of git repositories to detect new commits with configurable interval |
| 3-4 | [Implement commit metadata extraction](./3-4.md) | Proposed | Extract commit hash, message, timestamp, author, and branch information |
| 3-5 | [Implement diff extraction](./3-5.md) | Proposed | Extract full commit diffs and file-level statistics, handling large diffs efficiently |
| 3-6 | [Implement commit-to-session correlation](./3-6.md) | Proposed | Correlate commits with conversation timestamps and group by development session |
| 3-7 | [Implement database persistence for commits](./3-7.md) | Done | Persist commits and file changes to database following same pattern as conversation storage |
| 3-8 | [Add error handling and logging for git operations](./3-8.md) | Proposed | Handle git repository errors gracefully and add comprehensive logging throughout git capture system |
| 3-9 | [Integrate git capture components into daemon](./3-9.md) | Proposed | Wire together all git capture components and integrate with existing daemon lifecycle |
| 3-10 | [E2E CoS Test](./3-10.md) | Proposed | End-to-end test verifying all 11 acceptance criteria from PBI 3 |

