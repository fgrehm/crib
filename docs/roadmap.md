---
title: Roadmap
description: What's planned, what's being considered, and what's not happening.
---

Items move between sections as priorities shift.

## Planned

### Shared host configuration

Auto-inject common host config (SSH keys, gitconfig, package caches) into containers without boilerplate in every devcontainer.json.

### Zero-config project bootstrapping

Detect project type (Ruby, Node, Go, etc.) from conventions and generate a working devcontainer config without the user writing one.

### Transparent command dispatch

Run `crib` commands from inside or outside the container, with automatic delegation.

## Considering

### Package cache sharing

Share host-level package caches (apt, pip, npm, Go modules, etc.) across all workspaces via bind mounts. Like [vagrant-cachier](https://github.com/fgrehm/vagrant-cachier) for containers. Avoids re-downloading the world on every rebuild. Would be implemented as a bundled plugin using `pre-container-run` mounts.

### Machine-readable output (`--json`)

Add `--json` or `--format json` flag to commands like `status`, `list` for scripting and tooling integration. The internal data structures already support this.

### Container health checks

Detect when a container is unhealthy or stuck and surface it in `crib status` / `crib ps`.

### `crib logs`

Tail container logs, especially useful for compose workspaces with multiple services.

## Not Planned

These are explicitly out of scope for `crib`'s design philosophy.

- **SSH / remote providers**: `crib` is local-only by design.
- **IDE integration**: No VS Code extension, no JetBrains plugin. CLI only.
- **Agent injection**: All setup happens via `docker exec` from the host.
- **Kubernetes / cloud backends**: Local container runtimes only (Docker, Podman).
