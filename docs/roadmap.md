---
title: Roadmap
description: What's planned, what's being considered, and what's not happening.
---

Items move between sections as priorities shift.

## Planned

### Zero-config project bootstrapping (`crib init`)

Detect project type (Ruby, Node, Go, etc.) from conventions and generate a working devcontainer config without the user writing one. See the [RFC](https://github.com/fgrehm/crib/blob/main/docs/rfcs/init.md) for the full design.

### Transparent command dispatch

Run `crib` commands from inside or outside the container, with automatic delegation.

## Considering

### Support `extends` in devcontainer.json

Allow devcontainer.json to inherit from another configuration file, enabling teams to maintain shared base configurations with per-project overrides. This is an active proposal in the devcontainer spec ([devcontainers/spec#22](https://github.com/devcontainers/spec/issues/22)), with ongoing implementation work in the DevContainers CLI ([PR #311](https://github.com/devcontainers/cli/pull/311)). Once a decision is made on the spec side, we'll add support. If you have a strong use case or want to contribute, PRs are welcome.

### Standardize container-side plugin paths

Currently plugin mounts land at ad-hoc paths (`~/.crib_history/`, `/tmp/ssh-agent.sock`). A standard base (XDG `$XDG_DATA_HOME/crib/` for persistent data, `/tmp/crib/` for runtime sockets) would be cleaner, but we need more plugins and real-world usage to understand the right shape before committing to a convention.

### Machine-readable output (`--json`)

Add `--json` or `--format json` flag to commands like `status`, `list` for scripting and tooling integration. The internal data structures already support this.

### Fully qualified workspace paths

Workspace IDs are currently derived from the project directory name (e.g. `ruby-project`), which can collide when different orgs or parent directories have projects with the same name. Use a hash or full path to guarantee uniqueness.

### `--force` flag for destructive commands

Add a `--force` / `-f` flag to commands like `remove`, `rebuild`, and `restart` to skip confirmation prompts. Useful for scripting and CI.

### Colored log output

Color-code `crib logs` lines by service name when the terminal supports it, similar to `docker compose logs`. Useful for compose workspaces with multiple services where logs are interleaved.

### Container health checks

Detect when a container is unhealthy or stuck and surface it in `crib status` / `crib ps`.

### Remote access plugin (SSH into containers)

SSH server inside containers via the plugin system, enabling native filesystem performance on macOS and editor-agnostic remote development. See the [RFC](https://github.com/fgrehm/crib/blob/main/docs/rfcs/remote-access.md) for the full design.

## Not Planned

These are explicitly out of scope for `crib`'s design philosophy.

- **Remote/cloud providers**: `crib` is local-only by design.
- **IDE integration**: No VS Code extension, no JetBrains plugin. CLI only.
- **Agent injection**: All setup happens via `docker exec` from the host.
- **Kubernetes / cloud backends**: Local container runtimes only (Docker, Podman).
