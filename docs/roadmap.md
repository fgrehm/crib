---
title: Roadmap
description: What's planned, what's being considered, and what's not happening.
---

Items move between sections as priorities shift.

## Planned

### Zero-config project bootstrapping

Detect project type (Ruby, Node, Go, etc.) from conventions and generate a working devcontainer config without the user writing one.

### Transparent command dispatch

Run `crib` commands from inside or outside the container, with automatic delegation.

## Considering

### Standardize container-side plugin paths

Currently plugin mounts land at ad-hoc paths (`~/.crib_history/`, `/tmp/ssh-agent.sock`). A standard base (XDG `$XDG_DATA_HOME/crib/` for persistent data, `/tmp/crib/` for runtime sockets) would be cleaner, but we need more plugins and real-world usage to understand the right shape before committing to a convention.

### Package cache sharing

Share host-level package caches (apt, pip, npm, Go modules, etc.) across all workspaces via bind mounts. Like [vagrant-cachier](https://github.com/fgrehm/vagrant-cachier) for containers. Avoids re-downloading the world on every rebuild. Would be implemented as a bundled plugin using `pre-container-run` mounts.

### Machine-readable output (`--json`)

Add `--json` or `--format json` flag to commands like `status`, `list` for scripting and tooling integration. The internal data structures already support this.

### `crib doctor`

Diagnose common problems and stale state. Candidate checks:

- **Orphaned workspaces**: workspace state in `~/.crib/workspaces/` whose source directory no longer exists (e.g. project folder was deleted without running `crib remove`). Flag leftover credentials or plugin data.
- **Dangling containers**: containers with `crib.workspace` labels whose workspace state is missing.
- **Runtime availability**: Docker/Podman reachable, compose available.
- **Rootless Podman quirks**: subuid/subgid ranges, missing `newuidmap`/`newgidmap`.
- **Stale plugin data**: plugin directories under a workspace that no longer match the active plugin set.

Could offer `--fix` to clean up automatically (remove orphaned state, prune dangling containers).

### Container health checks

Detect when a container is unhealthy or stuck and surface it in `crib status` / `crib ps`.

### `crib logs`

Tail container logs, especially useful for compose workspaces with multiple services.

### Remote access plugin (SSH into containers)

SSH server inside containers via the plugin system, enabling native filesystem performance on macOS and editor-agnostic remote development. See the [RFC](https://github.com/fgrehm/crib/blob/main/docs/rfcs/remote-access.md) for the full design.

## Not Planned

These are explicitly out of scope for `crib`'s design philosophy.

- **Remote/cloud providers**: `crib` is local-only by design.
- **IDE integration**: No VS Code extension, no JetBrains plugin. CLI only.
- **Agent injection**: All setup happens via `docker exec` from the host.
- **Kubernetes / cloud backends**: Local container runtimes only (Docker, Podman).
