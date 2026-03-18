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

### ~~Fully qualified workspace paths~~

~~Workspace IDs are currently derived from the project directory name (e.g. `ruby-project`), which can collide when different orgs or parent directories have projects with the same name. Use a hash or full path to guarantee uniqueness. This is a breaking change (existing workspaces would need migration), so it should land before v1.0.~~ Done. Workspace IDs now use `{slug}-{7-char-sha256}` of the absolute project path.

### ~~`--force` flag for destructive commands~~

~~Add a `--force` / `-f` flag to commands like `remove`, `rebuild`, and `restart` to skip confirmation prompts. Useful for scripting and CI.~~ Done. `crib remove` and `crib prune` have `--force` / `-f`.

### XDG-based cache provider

The current cache plugin has per-tool providers (`apt`, `pip`, `npm`, `go`, etc.) that each mount a volume to a specific path. The `downloads` provider adds a general-purpose cache at `~/.cache/crib` with `$CRIB_CACHE`. Consider a single `xdg-cache` provider that mounts a volume at `$XDG_CACHE_HOME` (`~/.cache`), which would cover all tools that follow the XDG Base Directory Spec without per-tool configuration. Recipes and scripts could cache downloads to standard paths like `~/.cache/neovim/` without knowing about crib. Tradeoff: less granular control (can't cache apt but not pip), but simpler and more portable. Could coexist with per-tool providers for cases like `apt` where the path isn't under `~/.cache`.

### XDG-compliant cache location

The feature cache lives at `~/.crib/feature-cache/` but should be under `$XDG_CACHE_HOME/crib/` (defaults to `~/.cache/crib/`). This aligns with the existing decision to use `~/.config/crib/` for config (XDG standard) and lets system cleanup tools (`bleachbit`, distro scripts) discover purgeable data. Needs a migration path for existing installs.

### Colored log output

Color-code `crib logs` lines by service name when the terminal supports it, similar to `docker compose logs`. Useful for compose workspaces with multiple services where logs are interleaved.

### Container health checks

Detect when a container is unhealthy or stuck and surface it in `crib status` / `crib ps`.

### Build progress indicator

Show a spinner or progress bar during `docker commit` and other long-running operations when a TTY is detected. Investigate if `podman`/`docker commit` has machine-readable output we can use.

### Workspace-scoped `crib status` and `crib delete`

Accept an explicit workspace name argument so these commands work outside the project directory. Also useful for managing multiple workspaces from a central location.

### ~~`crib prune`~~

~~Clean all crib state, containers, volumes, and images in one shot. Useful for full reset or uninstall.~~ Done. `crib prune` removes stale and orphan workspace images, with dry-run preview and `--all` for global scope.

### Compose-built image cleanup

`crib remove` and `crib prune` don't clean up images produced by `docker compose build` for services that have a `build:` section but no DevContainer Features. These images are unnamed by crib (compose names them `{project}-{service}` on Docker, `{project}_{service}` on Podman) and carry no `crib.workspace` label. Cleaning them up requires either tracking the compose image name in `result.json` at build time, or deriving it from the stored config at remove time (accounting for runtime differences and explicit `image:` overrides in the compose file).

### Dangling feature build stage cleanup

When crib installs DevContainer Features, it generates a multi-stage Dockerfile with an intermediate named stage (`dev_containers_base_stage`). BuildKit materializes this stage as a separate image layer that never gets a tag or a `crib.workspace` label. After each rebuild, the old intermediate layer becomes a dangling `<none>:<none>` image. These accumulate silently -- `crib prune` is blind to them because it filters by `crib.workspace` label, and none of the dangling stages carry it. They do carry `devcontainer.metadata` (set by the feature install scripts), which could serve as a fingerprint. Options: apply the `crib.workspace` label to the build call so intermediate stages inherit it (Docker supports `--label` on `buildx build`), or include a `docker image prune` pass in `crib prune` scoped to images that carry `devcontainer.metadata` but no `crib.workspace`.

### Enhanced `crib list`

Accept arguments to filter/show state details (container status, services, ports, etc.).

### `crib build` command

Build the image / container without starting it. Useful for CI or pre-warming caches.

### Debug mode env var

Pass `CRIB_DEBUG=1` (or `DEVCONTAINER_BUILD_DEBUG`) into containers during builds and hook execution for easier troubleshooting. Pair with build log capture for a complete debugging story.

### Build log capture

Write all build output to `~/.crib/workspaces/{id}/logs/{timestamp}-build.txt` for post-mortem debugging.

### ~~Version tracking in workspace state~~

~~Record the crib version (semver, commit SHA, build timestamp) in workspace state so we can detect version mismatches and provide upgrade guidance. Surface in `crib doctor` when the workspace was created by a different version.~~ Done. `CribVersion` field in `workspace.json`, refreshed on every workspace access.

### Log output scrubbing

~~Redact sensitive env var values (tokens, keys, passwords) in `--debug` exec logs.~~ Done in v0.5.1.

Remaining: scrub PII patterns from subprocess output and verbose logs (not just env var names). Examples: email addresses, phone numbers, IP addresses, filesystem paths containing usernames. These can leak through lifecycle hook output, build logs, and error messages.

### ~~Env var filtering for exec/run~~

~~Skip injecting noisy env vars (e.g. `LS_COLORS`, `LSCOLORS`) that don't serve a purpose inside the container. Could be a configurable allowlist/denylist.~~ Done in v0.6.0. Implemented as a hardcoded skip list in `filterProbedEnv` (`internal/engine/env.go`), covering terminal colors, session-specific vars, version manager internals, and security-sensitive vars. A configurable allowlist/denylist can be added later if needed.

### Remote access plugin (SSH into containers)

SSH server inside containers via the plugin system, enabling native filesystem performance on macOS and editor-agnostic remote development. See the [RFC](https://github.com/fgrehm/crib/blob/main/docs/rfcs/remote-access.md) for the full design.

### Revisit save-path abstraction (ADR 001)

[ADR 001](decisions/001-no-save-path-abstraction.md) decided against abstracting the 6+ save sites across single/compose/restart paths. Since then, each new feature that touches container state (feature entrypoints, feature metadata, `${containerEnv:*}` resolution) has needed manual wiring into every path. The `resolveConfigEnvFromStored` fix is the latest example of a bug class where restart paths miss critical resolution steps that `setupContainer` handles automatically. Consider a `RestartStateResolver` or similar that encapsulates the restore-from-stored + resolve + plugin-merge sequence so new paths can't silently drop state.

### Project rename

The name "crib" is close to [cribl.io](https://cribl.io/), which muddies search results and makes it harder to find crib-specific troubleshooting material. Candidates: `devcrib`, `cribcontainers`, or something else entirely. This is a breaking change (binary name, Go module path, container labels, state directory) so it needs a migration plan.

### Agent sandboxing plugin

Restrict coding agents' filesystem and network access inside dev containers.
Built and shipped as part of v0.7.x, then reverted: the implementation used
[`bubblewrap`](https://github.com/containers/bubblewrap) for filesystem isolation
and `iptables` for network blocking, but both require capabilities unavailable in
rootless Podman (unprivileged user namespaces and `NET_ADMIN`). See
[ADR 002](decisions/002-sandbox-plugin.md) for the full design history.

The viable path forward is [Linux Landlock LSM](https://landlock.io/): works without
root or capabilities (kernel 5.13+), Go library available
([go-landlock](https://github.com/landlock-lsm/go-landlock)), supports filesystem
access control and TCP port restrictions (kernel 6.7+). Tradeoff: network restriction
is port-based only — IP/CIDR blocking (RFC 1918 ranges, cloud metadata endpoints)
is not possible, so `blockLocalNetwork` as designed cannot be reimplemented with
Landlock alone. Revisit if there's demand or if a solution for IP-level network
restriction without `NET_ADMIN` emerges.

### Reconsider `stop` / `down` semantics

`crib stop` is an alias for `crib down`, which removes the container rather than pausing it. This is surprising: users expect `stop` to be non-destructive (like `docker stop`), but it tears down the container and clears hook markers, causing all lifecycle hooks to re-run on the next `crib up`. Named compose volumes survive, but anything in the container's writable layer is gone, and setup commands like `db:seed` re-run unconditionally.

Options to consider: split `stop` (pause, preserve container) from `down` (remove), make hook-marker clearing opt-in, or at minimum update the command description and docs to set the right expectations. The driver already has `StopContainer`/`StartContainer` methods, so a non-destructive stop is feasible.

### Reduce cyclomatic complexity hotspots

CI gates at gocyclo > 40 (the ratchet that prevents things from getting worse). Several engine functions exceed 15, the practical threshold for maintainability: `upCompose` (38), `syncRemoteUserUID` (29), `generateComposeOverride` (26), `Doctor` (23), `detectConfigChange` (22), `extractTar` (19), `upSingle` (18), `restartRecreateSingle` (18). These should be broken into focused helpers before they become harder to change.

## Not Planned

These are explicitly out of scope for `crib`'s design philosophy.

- **Remote/cloud providers**: `crib` is local-only by design.
- **IDE integration**: No VS Code extension, no JetBrains plugin. CLI only.
- **Agent injection**: All setup happens via `docker exec` from the host.
- **Kubernetes / cloud backends**: Local container runtimes only (Docker, Podman).
