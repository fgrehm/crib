---
title: Command Reference
description: Detailed usage, flags, and examples for every crib command.
---

## `crib up`

Build the container image (if needed) and start the workspace. On first run, this builds the image, creates the container, syncs UID/GID, probes the user environment, and runs all [lifecycle hooks](/crib/guides/lifecycle-hooks/). On subsequent runs, it starts the existing container and runs only the resume hooks (`postStartCommand`, `postAttachCommand`).

```bash
crib up                                    # standard run
crib up --disable-plugin ssh               # skip a bundled plugin for this run
crib up --disable-plugin ssh,dotfiles      # repeatable or comma-separated
```

See [Disabling plugins](/crib/guides/plugins/#disabling-plugins) for per-project and global alternatives.

## `crib down`

Stop and remove the workspace container. This clears lifecycle hook markers, so the next `crib up` runs all hooks from scratch. Use this when you want a clean restart.

## `crib remove`

Remove the workspace container, all associated images, and stored state. Shows a preview of what will be deleted and prompts for confirmation before proceeding.

```bash
crib remove              # preview + confirm
crib remove --force      # skip confirmation (useful in scripts)
crib remove -f           # shorthand
```

## `crib shell`

Open an interactive shell inside the container. crib detects the user's shell (zsh, bash, or sh) and uses the environment captured during `crib up` (including tools installed by version managers like mise, nvm, rbenv).

## `crib run`

Run a command inside the container through a login shell. This sources shell init files (`.zshrc`, `.bashrc`, `.profile`) before running your command, making tools installed by version managers (mise, asdf, nvm, rbenv) available on PATH.

```bash
crib run -- ruby -v
crib run -- bundle install
crib run -- npm test
```

## `crib exec`

Run a command directly inside the container (raw `docker exec`). Does not source shell init files, so tools installed by version managers may not be on PATH. Use `crib run` instead if the command depends on shell init.

```bash
crib exec -- /usr/bin/env
crib exec -- bash -c "echo hello"
```

Both `run` and `exec` inherit the probed environment (`remoteEnv`) from `crib up`.

## `crib restart`

Restart the workspace, detecting what changed since the last `crib up`. See [Smart Restart](/crib/guides/smart-restart/) for details on how change detection works. Accepts `--disable-plugin` like `crib up`.

## `crib rebuild`

Full rebuild: runs `down` followed by `up`. Use this when the image needs to be rebuilt (changed Dockerfile, base image, or features). Clears any snapshot image so the build starts from scratch. Accepts `--disable-plugin` like `crib up`.

## `crib logs`

Show container logs. Defaults to the last 50 lines. For compose workspaces, shows logs from all services.

```bash
crib logs                # last 50 lines
crib logs -f             # follow (stream) all logs
crib logs --tail 100     # last 100 lines
crib logs -a             # show all logs (no tail limit)
```

## `crib doctor`

Check workspace health and diagnose issues. Detects orphaned workspaces (source directory deleted), dangling containers (crib label but no workspace state), and stale plugin data. Use `--fix` to auto-clean.

```bash
crib doctor              # check for issues
crib doctor --fix        # auto-fix found issues
```

## `crib cache`

Manage package cache volumes created by the [package cache plugin](/crib/guides/plugins/#package-cache).

### `crib cache list`

List cache volumes for the current workspace. Shows volume name, provider, and disk usage.

```bash
crib cache list              # current workspace only
crib cache list --all        # all workspaces
```

### `crib cache clean`

Remove cache volumes. Without arguments, removes all cache volumes for the current workspace. Pass provider names to remove specific ones.

```bash
crib cache clean             # remove all for current workspace
crib cache clean npm go      # remove specific providers only
crib cache clean --all       # remove all crib cache volumes
```

## `crib prune`

Remove stale and orphan workspace images. Shows a dry-run preview with sizes before prompting for confirmation.

- **Stale**: labeled images for an active workspace that are no longer the active build image or snapshot.
- **Orphan**: labeled images for a workspace that no longer exists in `~/.crib/workspaces/`.

```bash
crib prune               # stale images for current workspace + confirm
crib prune --all         # all workspaces including orphans
crib prune --force       # skip confirmation
```

## `crib list`

List all known workspaces and their container status.

## `crib status`

Show the status of the current workspace's container, including published ports. For compose workspaces, shows all service statuses with their ports.
