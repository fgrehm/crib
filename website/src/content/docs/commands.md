---
title: Commands
description: All available crib CLI commands.
---

| Command | Aliases | Description |
|---------|---------|-------------|
| `crib up` | | Create or start the workspace container |
| `crib down` | `stop` | Stop and remove the workspace container |
| `crib remove` | `rm`, `delete` | Remove the workspace container and state |
| `crib shell` | `sh` | Open an interactive shell (detects zsh/bash/sh) |
| `crib exec` | | Execute a command in the workspace container |
| `crib restart` | | Restart the workspace container (picks up safe config changes) |
| `crib rebuild` | | Rebuild the workspace (down + up) |
| `crib list` | `ls` | List all workspaces |
| `crib status` | `ps` | Show workspace container status |
| `crib version` | | Show version information |
