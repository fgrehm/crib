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

### Support `extends` in devcontainer.json

Allow devcontainer.json to inherit from another configuration file, enabling teams to maintain shared base configurations with per-project overrides. This is an active proposal in the devcontainer spec ([devcontainers/spec#22](https://github.com/devcontainers/spec/issues/22)), with ongoing implementation work in the DevContainers CLI ([PR #311](https://github.com/devcontainers/cli/pull/311)). Once a decision is made on the spec side, we'll add support. If you have a strong use case or want to contribute, PRs are welcome.

### Snapshot create-time hook effects into the image

**Problem:** `crib restart` with a safe config change (ports, env, mounts) recreates the container but only runs resume-flow hooks (`postStartCommand`, `postAttachCommand`). The create-time hooks (`onCreateCommand`, `updateContentCommand`, `postCreateCommand`) are skipped even though the container is brand new and their filesystem effects (installed packages, tool setup, etc.) are gone.

**Preferred approach:** after `crib up` or `crib rebuild` finishes all create-time hooks, `docker commit` the container to a local image (e.g. `crib-<workspace-id>:snapshot`). On subsequent recreations, use the snapshot image so hook effects are already in the filesystem and resume-only hooks are sufficient. `crib rebuild` discards the snapshot and starts fresh.

This is spec-compatible: the `devcontainer.metadata` label is designed for baking config into images, and the merge rules (local devcontainer.json wins, hooks are appended) handle the layering. Codespaces prebuilds use a similar pattern. The `devcontainers/cli` has `build --push` for CI prebuilds, but this would be a local-only variant.

**Things to figure out:**

- Compose workspaces: the snapshot image needs to replace the service image via the compose override.
- Staleness detection: if devcontainer.json hooks change, the snapshot is outdated. Could hash the hook definitions and invalidate on mismatch.
- Storage: committed images can be large. May need `crib prune` or automatic cleanup.
- Hook markers become less relevant since the image itself proves hooks ran. But markers still useful for the initial run before the first snapshot.

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
