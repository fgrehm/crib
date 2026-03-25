---
title: Roadmap
description: What's planned, what's being considered, and what's not happening.
---

Items move between sections as priorities shift.

## Planned

### Zero-config project bootstrapping (`crib init`)

Detect project type (Ruby, Node, Go, etc.) from conventions and generate a working devcontainer config without the user writing one. See the [RFC](https://github.com/fgrehm/crib/blob/main/docs/rfcs/init.md) for the full design.

### Reconsider `stop` / `down` semantics

`crib stop` is an alias for `crib down`, which removes the container rather than pausing it. This is surprising: users expect `stop` to be non-destructive (like `docker stop`), but it tears down the container and clears hook markers, causing all lifecycle hooks to re-run on the next `crib up`. Named compose volumes survive, but anything in the container's writable layer is gone, and setup commands like `db:seed` re-run unconditionally.

Options to consider: split `stop` (pause, preserve container) from `down` (remove), make hook-marker clearing opt-in, or at minimum update the command description and docs to set the right expectations. The driver already has `StopContainer`/`StartContainer` methods, so a non-destructive stop is feasible.

## Considering

### Features

#### Global user config (mounts, env vars)

A global config file (`~/.config/crib/` or similar, format TBD) that declares mounts and environment variables applied to every `crib up`. This is an escape hatch for simple "bind this path" use cases that don't need plugin logic. Plugins stay for anything requiring detection, file copying, or env var computation.

Motivating examples:

- Mount `~/.mem/` (read-write) so [dotmem](https://github.com/fgrehm/dotmem) memory is available inside containers and across all workspace projects
- Mount the [cartage](https://github.com/fgrehm/cartage) socket so notifications and `xdg-open` forwarding work without per-project config

Open questions: config file format (TOML, extended `.cribrc`, or something else), full scope of supported fields (just mounts? env vars? both?), merge semantics with per-project devcontainer.json.

Prior art: [devstep](https://github.com/fgrehm/devstep) had `~/devstep.yml` (global) merged with per-project `devstep.yml`, supporting `volumes`, `environment`, `links`, and more.

#### Ephemeral environments

Spin up a container for a cloned project that has no `.devcontainer/` config, without generating any files in the project tree. The detected config lives entirely in crib's workspace state (`~/.crib/workspaces/{id}/`).

Depends on the `crib init` detection engine (same project-type detection, different output target). An `ephemeral: true` flag in workspace state makes `crib down` behave like `crib remove`, cleaning up the container and all state automatically.

Build order: `crib init` first, ephemeral mode on top.

#### "Machine mode"

A general-purpose container not attached to any specific project. A scratch environment with your plugins (ssh, dotmem, shell-history, coding-agents) and a base image, for random experiments and one-off tasks. Separate from ephemeral project environments.

#### Support `extends` in devcontainer.json

Allow devcontainer.json to inherit from another configuration file, enabling teams to maintain shared base configurations with per-project overrides. This is an active proposal in the devcontainer spec ([devcontainers/spec#22](https://github.com/devcontainers/spec/issues/22)), with ongoing implementation work in the DevContainers CLI ([PR #311](https://github.com/devcontainers/cli/pull/311)). Once a decision is made on the spec side, we'll add support. If you have a strong use case or want to contribute, PRs are welcome.

#### Machine-readable output (`--json`)

Add `--json` or `--format json` flag to commands like `status`, `list` for scripting and tooling integration. The internal data structures already support this.

#### Workspace-scoped `crib status` and `crib delete`

Accept an explicit workspace name argument so these commands work outside the project directory. Also useful for managing multiple workspaces from a central location.

#### Enhanced `crib list`

Accept arguments to filter/show state details (container status, services, ports, etc.).

#### `crib build` command

Build the image / container without starting it. Useful for CI or pre-warming caches.

#### Feature lockfile for reproducible resolution

The official devcontainers CLI has an experimental `devcontainer-lock.json` that records resolved digests, versions, and integrity hashes for each feature. Enables `--frozen-lockfile` for deterministic builds. crib currently re-resolves features on every build. A lockfile would help teams pin exact feature versions in CI. Not part of the spec (CLI extension), but useful. Format: `{ features: { "<id>": { version, resolved, integrity, dependsOn } } }`.

### Plugins

#### Standardize container-side plugin paths

Currently plugin mounts land at ad-hoc paths (`~/.crib_history/`, `/tmp/ssh-agent.sock`). A standard base (XDG `$XDG_DATA_HOME/crib/` for persistent data, `/tmp/crib/` for runtime sockets) would be cleaner, but we need more plugins and real-world usage to understand the right shape before committing to a convention. The cartage plugin (above) would be a good forcing function for settling on a convention.

#### Dotfiles plugin

Clone and install a user's [dotfiles repository](https://dotfiles.github.io/) inside the container on creation. The devcontainer spec doesn't standardize dotfiles, but VS Code, GitHub Codespaces, and DevPod all support them. A crib plugin would clone the repo to a configurable path (default `~/dotfiles`) and run an install script (`install.sh`, `bootstrap.sh`, or a custom command). Configuration via `.cribrc` or global config (`dotfiles.repository`, `dotfiles.installCommand`). See [#17](https://github.com/fgrehm/crib/issues/17).

#### Cartage plugin

A crib plugin for [cartage](https://github.com/fgrehm/cartage) (container-to-host bridge daemon). Cartage forwards notifications, `xdg-open`, and clipboard between containers and the host desktop over a Unix socket.

The plugin would mount the socket and the cartage binary into the container, then create symlinks (`notify-send`, `xdg-open`, `pbcopy`, `pbpaste`, etc.) so tools work as drop-in replacements. It would also set `CARTAGE_PATH_MAP` based on the workspace path mapping. This needs plugin logic (symlink creation, path map computation), not just a global mount.

#### Remote access plugin (SSH into containers)

SSH server inside containers via the plugin system, enabling native filesystem performance on macOS and editor-agnostic remote development. See the [RFC](https://github.com/fgrehm/crib/blob/main/docs/rfcs/remote-access.md) for the full design.

#### Agent sandboxing plugin

Restrict coding agents' filesystem and network access inside dev containers. Built as part of v0.7.x, then reverted: the implementation used [`bubblewrap`](https://github.com/containers/bubblewrap) for filesystem isolation and `iptables` for network blocking, but both require capabilities unavailable in rootless Podman (unprivileged user namespaces and `NET_ADMIN`). See [ADR 002](decisions/002-sandbox-plugin.md) for the full design history.

The viable path forward is [Linux Landlock LSM](https://landlock.io/): works without root or capabilities (kernel 5.13+), Go library available ([go-landlock](https://github.com/landlock-lsm/go-landlock)), supports filesystem access control and TCP port restrictions (kernel 6.7+). Tradeoff: network restriction is port-based only — IP/CIDR blocking (RFC 1918 ranges, cloud metadata endpoints) is not possible, so `blockLocalNetwork` as designed cannot be reimplemented with Landlock alone. Revisit if there's demand or if a solution for IP-level network restriction without `NET_ADMIN` emerges.

See also [NVIDIA OpenShell](https://docs.nvidia.com/openshell/latest/index.html), a full sandbox runtime for AI agents with declarative policy enforcement. It solves the same problem but as an external daemon on top of Docker, which is heavier than what crib needs (a library-level solution within its own container lifecycle).

#### Claude project context inside containers

When Claude Code runs inside a container, it builds a project hash from the workspace path. Because the container path (`/workspaces/foo`) differs from the host path (`/home/user/projects/foo`), the agent gets a different hash and loses all accumulated project insights and learnings.

Mounting the host's `~/.claude/projects/<hash>/` directory at the container's expected hash path would fix this, but it requires reverse-engineering Claude's hashing logic and would break on upstream changes. A Claude Code config to override the project directory association would make this trivial. Parked until an upstream escape hatch appears. As of March 2026, no such config exists in Claude Code's [settings](https://code.claude.com/docs/en/settings).

### UX

#### Structured log events with progress tracking

The official devcontainers CLI uses a typed log event system (`text`, `raw`, `start`, `stop`, `progress`) instead of plain text output. Lifecycle hooks emit `::step::` and `::endstep::` markers that the log handler converts to progress events, enabling rich terminal UI (spinners per hook step) without coupling hooks to the display layer. crib currently uses plain `slog` and a progress callback. A typed event system would decouple output formatting from execution and enable richer feedback during long-running operations.

#### Rich error context

crib's errors are mostly `fmt.Errorf` chains. The official devcontainers CLI wraps errors with structured data: container ID, config that caused the error, suggested recovery actions, and the original error. This enables better error messages ("failed to start container X because feature Y requires privileged mode, try adding `privileged: true`") and potential auto-recovery. Worth adopting incrementally, starting with the most common failure modes (build failures, hook failures, container start failures).

#### Debug mode and build log capture

Pass `CRIB_DEBUG=1` (or `DEVCONTAINER_BUILD_DEBUG`) into containers during builds and hook execution for easier troubleshooting. Write all build output to `~/.crib/workspaces/{id}/logs/{timestamp}-build.txt` for post-mortem debugging.

#### PII scrubbing in logs

Scrub PII patterns from subprocess output and verbose logs. Examples: email addresses, phone numbers, IP addresses, filesystem paths containing usernames. These can leak through lifecycle hook output, build logs, and error messages. (Env var redaction in `--debug` exec logs already done in v0.5.1.)

#### Build progress indicator

Show a spinner or progress bar during `docker commit` and other long-running operations when a TTY is detected. Investigate if `podman`/`docker commit` has machine-readable output we can use.

#### Colored log output

Color-code `crib logs` lines by service name when the terminal supports it, similar to `docker compose logs`. Useful for compose workspaces with multiple services where logs are interleaved.

#### Container health checks

Detect when a container is unhealthy or stuck and surface it in `crib status` / `crib ps`.

#### Dockerfile content change detection in smart restart

`crib restart` detects changes to devcontainer.json fields and compose file contents, but not changes to files referenced by those configs. If a Dockerfile used by a compose `build:` section or by `devcontainer.json`'s `dockerfile` field changes (e.g. a Ruby version bump in `FROM`), `restart` doesn't notice and does a simple restart instead of suggesting `crib rebuild`.

Options: hash Dockerfile contents alongside compose files, parse compose YAML to discover referenced Dockerfiles and `.env` files, or add a lighter-weight mtime check. The tricky part is knowing which files to track (Dockerfiles, `.dockerignore`, build context files, `.env` files) without reimplementing `docker build`'s dependency graph.

### Spec Compliance

#### Recursive `dependsOn` resolution for features

The spec says `dependsOn` should be resolved recursively: if feature A depends on feature B (not explicitly listed in `devcontainer.json`), the tool should automatically pull and install feature B. crib's `OrderFeatures()` in `internal/feature/order.go` instead errors with "not in the feature set." Few real-world features exercise this today, but it's a spec gap.

#### Round-based feature installation ordering

The spec describes a round-based priority system where `overrideFeatureInstallOrder` assigns `roundPriority = n - index`, and within each round only features at the max priority are committed. crib uses topological sort (Kahn's algorithm) with a post-hoc reorder that moves override entries to the front. These produce the same result in most cases but can diverge when override features have dependencies that should interleave with non-override features.

### Housekeeping

#### Workspace flock

Concurrent `crib up` invocations on the same workspace can race (two processes building, creating containers, or writing `result.json` simultaneously). DevPod uses [`gofrs/flock`](https://github.com/gofrs/flock) for file-based workspace locking. Small effort, prevents a real (if uncommon) bug class. Lock file would live at `~/.crib/workspaces/{id}/lock`.

#### Adopt compose-go library for Compose YAML parsing

crib currently generates compose override YAML via string concatenation in `generateComposeOverride` (cyclomatic complexity 26) and shells out to `docker compose` for service inspection. The official [`compose-spec/compose-go/v2`](https://github.com/compose-spec/compose-go) library (used by DevPod) provides programmatic access to service definitions, environment files, build configs, and override generation. Would simplify `generateComposeOverride`, `resolveComposeDockerfileInfo`, and the Dockerfile content change detection item above.

#### `userEnvProbe` session caching

crib re-probes the container's user environment on every `crib up` (twice: pre-hook and post-hook). The official devcontainers CLI caches probe results in a container-side session file. Caching the pre-hook probe and only re-probing post-hook could save ~1s per start.

#### Shell persistence for exec commands

The official devcontainers CLI keeps a persistent shell open inside the container and delimits commands/output with a UTF-8 sentinel character (EOT). A streaming parser extracts stdout, stderr, and exit code per command, avoiding the overhead of spawning a new `docker exec` per lifecycle hook or plugin dispatch. Lower priority since individual `docker exec` calls are fast enough today, but would matter for workspaces with many hooks.

#### XDG-based cache provider

The current cache plugin has per-tool providers (`apt`, `pip`, `npm`, `go`, etc.) that each mount a volume to a specific path. The `downloads` provider adds a general-purpose cache at `~/.cache/crib` with `$CRIB_CACHE`. Consider a single `xdg-cache` provider that mounts a volume at `$XDG_CACHE_HOME` (`~/.cache`), which would cover all tools that follow the XDG Base Directory Spec without per-tool configuration. Recipes and scripts could cache downloads to standard paths like `~/.cache/neovim/` without knowing about crib. Tradeoff: less granular control (can't cache apt but not pip), but simpler and more portable. Could coexist with per-tool providers for cases like `apt` where the path isn't under `~/.cache`.

#### XDG-compliant cache location

The feature cache lives at `~/.crib/feature-cache/` but should be under `$XDG_CACHE_HOME/crib/` (defaults to `~/.cache/crib/`). This aligns with the existing decision to use `~/.config/crib/` for config (XDG standard) and lets system cleanup tools (`bleachbit`, distro scripts) discover purgeable data. Needs a migration path for existing installs.

#### Compose-built image cleanup

`crib remove` and `crib prune` don't clean up images produced by `docker compose build` for services that have a `build:` section but no DevContainer Features. These images are unnamed by crib (compose names them `{project}-{service}` on Docker, `{project}_{service}` on Podman) and carry no `crib.workspace` label. Cleaning them up requires either tracking the compose image name in `result.json` at build time, or deriving it from the stored config at remove time (accounting for runtime differences and explicit `image:` overrides in the compose file).

#### Dangling feature build stage cleanup

When crib installs DevContainer Features, it generates a multi-stage Dockerfile with an intermediate named stage (`dev_containers_base_stage`). BuildKit materializes this stage as a separate image layer that never gets a tag or a `crib.workspace` label. After each rebuild, the old intermediate layer becomes a dangling `<none>:<none>` image. These accumulate silently -- `crib prune` is blind to them because it filters by `crib.workspace` label, and none of the dangling stages carry it. They do carry `devcontainer.metadata` (set by the feature install scripts), which could serve as a fingerprint. Options: apply the `crib.workspace` label to the build call so intermediate stages inherit it (Docker supports `--label` on `buildx build`), or include a `docker image prune` pass in `crib prune` scoped to images that carry `devcontainer.metadata` but no `crib.workspace`.

#### Project rename

The name "crib" is close to [cribl.io](https://cribl.io/), which muddies search results and makes it harder to find crib-specific troubleshooting material. Candidates: `devcrib`, `cribcontainers`, or something else entirely. This is a breaking change (binary name, Go module path, container labels, state directory) so it needs a migration plan.

#### Revisit save-path abstraction (ADR 001)

[ADR 001](decisions/001-no-save-path-abstraction.md) decided against abstracting the 6+ save sites across single/compose/restart paths. The EnvBuilder refactor (v0.5.1) addressed the highest-risk dimension by centralizing env layer ordering and eliminating the bug class where plugin env was silently lost on restart. Structural asymmetries remain (e.g. `${containerEnv:*}` resolution is manual wiring in restart paths, plugin response merging happens in both backend and finalize), but nothing has bitten since EnvBuilder landed. Revisit if new state fields start causing restart-path bugs again.

#### Reduce cyclomatic complexity hotspots

CI gates at gocyclo > 40 (the ratchet that prevents things from getting worse). Several engine functions exceed 15, the practical threshold for maintainability: `upCompose` (38), `syncRemoteUserUID` (29), `generateComposeOverride` (26), `Doctor` (23), `detectConfigChange` (22), `extractTar` (19), `upSingle` (18), `restartRecreateSingle` (18). These should be broken into focused helpers before they become harder to change.

## Not Planned

These are explicitly out of scope for `crib`'s design philosophy.

- **Remote/cloud providers**: `crib` is local-only by design.
- **IDE integration**: No VS Code extension, no JetBrains plugin. CLI only.
- **Agent injection**: All setup happens via `docker exec` from the host.
- **Kubernetes / cloud backends**: Local container runtimes only (Docker, Podman).
