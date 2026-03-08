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

Workspace IDs are currently derived from the project directory name (e.g. `ruby-project`), which can collide when different orgs or parent directories have projects with the same name. Use a hash or full path to guarantee uniqueness. This is a breaking change (existing workspaces would need migration), so it should land before v1.0.

### `--force` flag for destructive commands

Add a `--force` / `-f` flag to commands like `remove`, `rebuild`, and `restart` to skip confirmation prompts. Useful for scripting and CI.

### Colored log output

Color-code `crib logs` lines by service name when the terminal supports it, similar to `docker compose logs`. Useful for compose workspaces with multiple services where logs are interleaved.

### Container health checks

Detect when a container is unhealthy or stuck and surface it in `crib status` / `crib ps`.

### Build progress indicator

Show a spinner or progress bar during `docker commit` and other long-running operations when a TTY is detected. Investigate if `podman`/`docker commit` has machine-readable output we can use.

### Workspace-scoped `crib status` and `crib delete`

Accept an explicit workspace name argument so these commands work outside the project directory. Also useful for managing multiple workspaces from a central location.

### `crib nuke` / `crib prune`

Clean all crib state, containers, volumes, and images in one shot. Useful for full reset or uninstall.

### Enhanced `crib list`

Accept arguments to filter/show state details (container status, services, ports, etc.).

### `crib build` command

Build the image / container without starting it. Useful for CI or pre-warming caches.

### Debug mode env var

Pass `CRIB_DEBUG=1` (or `DEVCONTAINER_BUILD_DEBUG`) into containers during builds and hook execution for easier troubleshooting. Pair with build log capture for a complete debugging story.

### Build log capture

Write all build output to `~/.crib/workspaces/{id}/logs/{timestamp}-build.txt` for post-mortem debugging.

### Version tracking in workspace state

Record the crib version (semver, commit SHA, build timestamp) in workspace state so we can detect version mismatches and provide upgrade guidance. Surface in `crib doctor` when the workspace was created by a different version.

### Log output scrubbing

~~Redact sensitive env var values (tokens, keys, passwords) in `--debug` exec logs.~~ Done in v0.5.1.

Remaining: scrub PII patterns from subprocess output and verbose logs (not just env var names). Examples: email addresses, phone numbers, IP addresses, filesystem paths containing usernames. These can leak through lifecycle hook output, build logs, and error messages.

### ~~Env var filtering for exec/run~~

~~Skip injecting noisy env vars (e.g. `LS_COLORS`, `LSCOLORS`) that don't serve a purpose inside the container. Could be a configurable allowlist/denylist.~~ Done in v0.6.0. Implemented as a hardcoded skip list in `filterProbedEnv` (`internal/engine/env.go`), covering terminal colors, session-specific vars, version manager internals, and security-sensitive vars. A configurable allowlist/denylist can be added later if needed.

### Remote access plugin (SSH into containers)

SSH server inside containers via the plugin system, enabling native filesystem performance on macOS and editor-agnostic remote development. See the [RFC](https://github.com/fgrehm/crib/blob/main/docs/rfcs/remote-access.md) for the full design.

### Reduce cyclomatic complexity hotspots

CI gates at gocyclo > 40 (the ratchet that prevents things from getting worse). Several engine functions exceed 15, the practical threshold for maintainability: `upCompose` (38), `syncRemoteUserUID` (29), `generateComposeOverride` (26), `Doctor` (23), `detectConfigChange` (22), `extractTar` (19), `upSingle` (18), `restartRecreateSingle` (18). These should be broken into focused helpers before they become harder to change.

## Not Planned

These are explicitly out of scope for `crib`'s design philosophy.

- **Remote/cloud providers**: `crib` is local-only by design.
- **IDE integration**: No VS Code extension, no JetBrains plugin. CLI only.
- **Agent injection**: All setup happens via `docker exec` from the host.
- **Kubernetes / cloud backends**: Local container runtimes only (Docker, Podman).
