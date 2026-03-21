---
title: Commands
description: Quick reference for all crib CLI commands.
---

| Command | Aliases | Description |
|---------|---------|-------------|
| `up` | | Create or start the workspace container |
| `down` | `stop` | Stop and remove the workspace container |
| `remove` | `rm`, `delete` | Remove the workspace container and state |
| `shell` | `sh` | Open an interactive shell (detects zsh/bash/sh) |
| `run` | | Run a command through a login shell (picks up mise/nvm/rbenv) |
| `exec` | | Execute a command directly in the workspace container |
| `restart` | | Restart the workspace container (picks up safe config changes) |
| `rebuild` | | Rebuild the workspace (down + up) |
| `logs` | | Show container logs |
| `doctor` | | Check workspace health and diagnose issues |
| `cache list` | | List package cache volumes |
| `cache clean` | | Remove package cache volumes |
| `prune` | | Remove stale and orphan workspace images |
| `list` | `ls` | List all workspaces |
| `status` | `ps` | Show workspace container status |
| `version` | | Show version information |

## Global flags

| Flag | Description |
|------|-------------|
| `--config`, `-C` | Path to the devcontainer config directory |
| `--debug` | Enable debug logging |
| `--verbose` | Show full compose output (suppressed by default) |

## shell vs run vs exec

| | `crib shell` | `crib run` | `crib exec` |
|---|---|---|---|
| Interactive | Yes | No | No |
| Shell init files | Yes | Yes (login shell) | No |
| Version managers (mise/nvm/rbenv) | Available | Available | Not on PATH |
| Use for | Working interactively | Running project commands | System binaries, scripts with absolute paths |

If a command fails with "not found" in `exec`, try `run` instead.

:::tip[Need more detail?]
See the [command reference](/crib/reference/commands/) for detailed usage, flags, and examples for every command.
:::
