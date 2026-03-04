---
title: Commands
description: All available crib CLI commands and global flags.
---

## Quick reference

| Command | Aliases | Description |
|---------|---------|-------------|
| `up` | | Create or start the workspace container |
| `down` | `stop` | Stop and remove the workspace container |
| `remove` | `rm`, `delete` | Remove the workspace container and state |
| `shell` | `sh` | Open an interactive shell (detects zsh/bash/sh) |
| `exec` | | Execute a command in the workspace container |
| `restart` | | Restart the workspace container (picks up safe config changes) |
| `rebuild` | | Rebuild the workspace (down + up) |
| `logs` | | Show container logs |
| `doctor` | | Check workspace health and diagnose issues |
| `cache list` | | List package cache volumes |
| `cache clean` | | Remove package cache volumes |
| `list` | `ls` | List all workspaces |
| `status` | `ps` | Show workspace container status |
| `version` | | Show version information |

## Global flags

| Flag | Description |
|------|-------------|
| `--config`, `-C` | Path to the devcontainer config directory |
| `--debug` | Enable debug logging |
| `--verbose`, `-V` | Show full compose output (suppressed by default) |

## Commands

### `crib up`

Build the container image (if needed) and start the workspace. On first run, this builds the image, creates the container, syncs UID/GID, probes the user environment, and runs all [lifecycle hooks](/crib/guides/lifecycle-hooks/). On subsequent runs, it starts the existing container and runs only the resume hooks (`postStartCommand`, `postAttachCommand`).

### `crib down`

Stop and remove the workspace container. This clears lifecycle hook markers, so the next `crib up` runs all hooks from scratch. Use this when you want a clean restart.

### `crib remove`

Remove the workspace container and all stored state from `~/.crib/workspaces/`. Use this to fully clean up a workspace.

### `crib shell`

Open an interactive shell inside the container. crib detects the user's shell (zsh, bash, or sh) and uses the environment captured during `crib up` (including tools installed by version managers like mise, nvm, rbenv).

### `crib exec`

Run a command inside the container:

```bash
crib exec -- npm test
crib exec -- bash -c "echo hello"
```

Like `shell`, inherits the probed environment from `crib up`.

### `crib restart`

Restart the workspace, detecting what changed since the last `crib up`. See [Smart Restart](/crib/guides/smart-restart/) for details on how change detection works.

### `crib logs`

Show container logs. Supports `--follow`/`-f` for live streaming and `--tail N` for limiting output. For compose workspaces, shows logs from all services.

```bash
crib logs                # show all logs
crib logs -f             # follow (stream) logs
crib logs --tail 50      # last 50 lines
```

### `crib doctor`

Check workspace health and diagnose issues. Detects orphaned workspaces (source directory deleted), dangling containers (crib label but no workspace state), and stale plugin data. Use `--fix` to auto-clean.

```bash
crib doctor              # check for issues
crib doctor --fix        # auto-fix found issues
```

### `crib cache`

Manage package cache volumes created by the [package cache plugin](/crib/guides/plugins/#package-cache).

#### `crib cache list`

List cache volumes for the current workspace. Shows volume name, provider, and disk usage.

```bash
crib cache list              # current workspace only
crib cache list --all        # all workspaces
```

#### `crib cache clean`

Remove cache volumes. Without arguments, removes all cache volumes for the current workspace. Pass provider names to remove specific ones.

```bash
crib cache clean             # remove all for current workspace
crib cache clean npm go      # remove specific providers only
crib cache clean --all       # remove all crib cache volumes
```


### `crib rebuild`

Full rebuild: runs `down` followed by `up`. Use this when the image needs to be rebuilt (changed Dockerfile, base image, or features). Clears any snapshot image so the build starts from scratch.

### `crib list`

List all known workspaces and their container status.

### `crib status`

Show the status of the current workspace's container, including published ports. For compose workspaces, shows all service statuses with their ports.
