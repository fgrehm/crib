---
title: Workspaces
description: How crib tracks and manages your devcontainer projects.
---

A workspace is crib's way of tracking a devcontainer project. When you run `crib up` in a project directory, crib creates a workspace that maps your project to a container and stores the state needed to manage it.

## Resolution

crib finds your project automatically. From your current directory, it walks up the directory tree looking for:

- `.devcontainer/devcontainer.json`, or
- `.devcontainer.json` in the project root

No workspace names to type. Just `cd` into your project (or any subdirectory) and run commands.

```bash
cd ~/projects/myapp/src/components
crib up    # finds ~/projects/myapp/.devcontainer/devcontainer.json
crib exec ls
```

You can override this with `--dir` (start the search from a different directory) or `--config` (point directly at a config directory, skipping the walk-up).

## Naming

The workspace ID is derived from the project directory name:

- Lowercased
- Non-alphanumeric characters replaced with hyphens
- Very long names truncated with a hash suffix for uniqueness

The container is named `crib-{workspace-id}` and labeled `crib.workspace={workspace-id}`, so `docker ps` output stays readable:

```
CONTAINER ID   IMAGE          NAMES
a1b2c3d4e5f6   node:20        crib-myapp
f6e5d4c3b2a1   python:3.12    crib-data-pipeline
```

## State

Each workspace stores its state in `~/.crib/workspaces/{id}/`:

| File | Purpose |
|---|---|
| `workspace.json` | Project metadata: source path, config location, timestamps |
| `result.json` | Last run result: container ID, merged config, remote user, env |
| `hooks/*.done` | Markers for lifecycle hooks that have already run |
| `plugins/` | Plugin state (shell history, credentials, etc.) |

`result.json` is saved early during `crib up`, before lifecycle hooks finish. This lets you run `crib exec` or `crib shell` in another terminal while hooks are still executing.

## Lifecycle

| Command | Effect on workspace |
|---|---|
| `crib up` | Creates the workspace (if new) and starts the container. Updates `result.json`. |
| `crib down` | Stops the container. Clears hook markers so all hooks re-run on next `up`. |
| `crib restart` | Restarts or recreates the container depending on what changed. |
| `crib rebuild` | Full rebuild: tears down the container, clears hooks, rebuilds the image, starts fresh. |
| `crib remove` | Stops the container and deletes the entire workspace state directory. |

`crib down` preserves workspace state. The container is gone, but `crib up` will recreate it and re-run all lifecycle hooks. `crib remove` is a full cleanup, deleting both the container and all stored state.

## Listing workspaces

`crib list` shows all tracked workspaces:

```bash
$ crib list
WORKSPACE         SOURCE
myapp             /home/user/projects/myapp
data-pipeline     /home/user/projects/data-pipeline
```

## Per-project configuration

A `.cribrc` file in the project root can set defaults for that workspace:

```
config=.devcontainer/python
```

This is equivalent to always passing `--config .devcontainer/python` when running crib commands from that project. Useful for projects with multiple devcontainer configs where you want a default.
