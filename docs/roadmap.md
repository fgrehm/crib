# Roadmap

What's planned, what's being considered, and what's not happening. Items move between sections as priorities shift.

## Planned

### Documentation website (GitHub Pages)

The README on `main` describes unreleased features and the `stable` branch workaround is not great for discoverability. A proper docs site would:

- Separate getting started / installation from reference docs
- Version docs per release so users see what matches their binary
- Host the spec compliance table, troubleshooting, and development guides
- Use something lightweight (mkdocs-material, mdbook, or plain Hugo)

### Spec compliance gaps

From `docs/implementation-notes.md`, these are parsed but not fully implemented:

- **`build.options`**: Extra Docker build CLI flags. Field is parsed but not passed to the build driver.
- **`waitFor`**: Field is parsed but lifecycle hooks always run sequentially to completion. Should block tool attachment until the specified stage finishes.
- **Parallel object hooks**: Spec says object-syntax hook entries run in parallel. crib runs them sequentially.

### Shared host configuration (`customizations.crib.sharedMounts`)

Auto-inject common host config (SSH keys, gitconfig, package caches) into containers without boilerplate in every devcontainer.json. See `docs/specs/2026-02-20-crib-extensions.md` for the full design.

### Zero-config project bootstrapping

Detect project type (Ruby, Node, Go, etc.) from conventions and generate a working devcontainer config without the user writing one. See `docs/specs/2026-02-20-crib-extensions.md`.

### Transparent command dispatch

Run crib commands from inside or outside the container, with automatic delegation. See `docs/specs/2026-02-20-crib-extensions.md`.

## Considering

### Machine-readable output (`--json`)

Add `--json` or `--format json` flag to commands like `status`, `list` for scripting and tooling integration. The internal data structures already support this.

### Container health checks

Detect when a container is unhealthy or stuck and surface it in `crib status` / `crib ps`.

### `crib logs`

Tail container logs, especially useful for compose workspaces with multiple services.

## Not Planned

These are explicitly out of scope for crib's design philosophy.

- **SSH / remote providers**: crib is local-only by design.
- **IDE integration**: No VS Code extension, no JetBrains plugin. CLI only.
- **Agent injection**: All setup happens via `docker exec` from the host.
- **Kubernetes / cloud backends**: Local container runtimes only (Docker, Podman).
