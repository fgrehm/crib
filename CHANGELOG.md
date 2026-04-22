# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Plugin configurability. Bundled plugins can now be disabled globally
  (`~/.config/crib/config.toml` under `[plugins]` with `disable = ["ssh",
  ...]` or `disable_all = true`), per-project (`.cribrc` with
  `plugins.disable = ssh, dotfiles` or `plugins = false`), or per-command
  (`--disable-plugin ssh` on `crib up`, `crib rebuild`, `crib restart`).
  Unknown names in the disable list log a warning. Closes
  [#37](https://github.com/fgrehm/crib/issues/37) (users can now skip the
  SSH plugin on macOS + Colima where virtiofs breaks agent socket bind
  mounts).
- Go fuzz tests for config parsing (`ParseBytes`, `ParseMount`,
  `SubstituteString`) and Dockerfile handling (`Parse`, `RemoveSyntaxVersion`,
  `EnsureFinalStageName`). Runs in CI with a 10s per-target budget.
- Cosign keyless signing for release artifacts. `checksums.txt.sigstore.json`
  is now attached to each GitHub release.
- `SECURITY.md` with responsible disclosure instructions.
- pi support in the coding-agents plugin. pi behaves identically to Claude
  Code under the shared `credentials` mode: in host mode,
  `~/.pi/agent/auth.json` is copied into the container when it exists on the
  host; in workspace mode, a persistent state directory is bind-mounted so
  credentials created inside the container survive rebuilds.

### Changed

- `crib up`, `crib rebuild`, `crib restart`, and `crib status` now display the
  actual container name, including user overrides via `runArgs: ["--name",
  "foo"]`, instead of always showing the default `crib-<ws-id>`. Resolves the
  stale display surfaced by [#35](https://github.com/fgrehm/crib/issues/35).
  The chosen name is persisted on `workspace.Result.ContainerName`; empty for
  compose backends and workspaces created before this change, with fallback to
  the default.
- Config parsing now rejects `runArgs` alongside `dockerComposeFile`. Per the
  devcontainer spec, `runArgs` applies to image/Dockerfile configs only;
  compose workspaces set container runtime options in the compose YAML.

### Fixed

- SSH plugin now verifies that `SSH_AUTH_SOCK` points at an actual Unix
  socket before bind-mounting it. Defense-in-depth for environments like
  macOS + Colima, where virtiofs exposes the socket as a regular file that
  crashes the container runtime when bind-mounted. When the path is not a
  socket the plugin logs a warning and skips agent forwarding.

#### Spec-compliant `remoteUser` / `containerUser` resolution

- `crib shell`, `crib exec`, and `crib run` now re-read `remoteUser` from the
  live `devcontainer.json` on each invocation. Changes take effect without a
  rebuild.
- `remoteUser` and `containerUser` are now inferred from the image when
  `devcontainer.json` does not set them explicitly. Resolution follows the spec:
  config wins, then `devcontainer.metadata` label (last-wins merge), then
  Dockerfile `USER` / `Config.User`. Pre-built images that carry a
  `devcontainer.metadata` label (e.g.
  `mcr.microsoft.com/devcontainers/javascript-node` sets `remoteUser: "node"`)
  now correctly use the label user for feature context and plugin dispatch.
- `containerUser` and `remoteUser` are resolved independently -- neither falls
  back to the other except `remoteUser` → `containerUser` (one direction only,
  per spec).
- Resume and restart recreate paths preserve a previously-inferred `remoteUser`
  instead of silently falling back to `root` when image inspection yields no
  metadata.

### Security

- GitHub Actions pinned to commit SHAs across all workflows.
- `contents: write` permission scoped to the GoReleaser job in `release.yml`
  (was workflow-level).

## [0.8.0] - 2026-04-07

### Added

- `crib stop` command: non-destructive container stop that preserves hook
  markers. The next `crib up` resumes with only start-time hooks.
- **Dotfiles plugin**: clones and installs a dotfiles repository inside the
  container on creation. Configured via global config (`~/.config/crib/config.toml`).
  Supports custom target path, install command override, and auto-detection of
  common install scripts. Per-project overrides and opt-out via `.cribrc`
  (`dotfiles.repository`, `dotfiles.targetPath`, `dotfiles.installCommand`,
  `dotfiles = false`). See [#17](https://github.com/fgrehm/crib/issues/17).
- **Global config** (`~/.config/crib/config.toml`, respects `$XDG_CONFIG_HOME`):
  user-level settings applied across all workspaces. Currently supports
  `[dotfiles]` configuration.
- `PostContainerCreate` plugin hook: runs between `postCreateCommand` and
  `postStartCommand` during fresh container creation. Provides `Exec` and
  `StreamExec` callbacks for running commands inside the container.
- Workspace file lock prevents concurrent state-mutating commands from racing.
- Dead code detection (`go tool deadcode`) as a CI gate.

### Changed

- `stop` is no longer an alias for `down`. `crib stop` pauses the container
  (preserving state), `crib down` removes it.
- `crib.home` container label only applied when `CRIB_HOME` is explicitly set.
- Compose override generation uses compose-go types instead of string
  concatenation.
- Compose stop/down reuse the persisted `compose-override.yml` from `crib up`.
- `LoadProject` now threads caller-supplied environment variables through to
  compose-go's loader for `${VAR}` substitution.

### Fixed

- `crib down`, `crib stop`, and `crib remove` now return a clear error
  immediately when a compose workspace is detected but compose is not
  installed, instead of silently falling through to single-container cleanup.
- Invalid port numbers in container inspect output are now logged at
  `WARN` level instead of `DEBUG`, making them visible in `crib status`
  output without `--debug`.
- Docs website now deploys automatically on `stable` branch push.
- Compose backend captures stderr for diagnostics when container is not found
  after `compose up`.
- Compose override no longer produces duplicate mount destinations when user
  compose files already define a volume for the same target path.
- `crib doctor --fix` no longer deletes containers belonging to a different
  `CRIB_HOME`.
- Integration and e2e tests no longer interfere with active workspaces.
- `runArgs: ["--name", "..."]` in `devcontainer.json` no longer causes a
  duplicate `--name` flag error. The user-specified name now overrides crib's
  default container name. See [#35](https://github.com/fgrehm/crib/issues/35).

## [0.7.1] - 2026-03-25

### Fixed

- Feature `containerEnv` values (e.g. `PATH=/nvm/bin:${PATH}` from the node
  feature) were applied twice: correctly as `ENV` instructions in the Dockerfile
  (where Docker expands `${PATH}` at build time), then incorrectly as `-e` flags
  at runtime (where `${PATH}` stays literal via `docker run`, or gets interpolated
  from the host in compose). Either way the runtime PATH diverges from the
  image's correctly-expanded PATH, breaking command resolution on macOS.
- Dispatch feature-declared lifecycle hooks. Features can declare `onCreateCommand`,
  `updateContentCommand`, `postCreateCommand`, `postStartCommand`, and
  `postAttachCommand` in `devcontainer-feature.json`. These now execute before
  user-defined hooks at each stage, in feature installation order (per the spec).
  Feature hooks are stored in `result.json` so they persist across restarts without
  re-resolving features from OCI registries. Also parses the previously-missing
  `updateContentCommand` field from feature configs.
- `crib restart` now detects changes inside Docker Compose files (volumes,
  ports, environment, etc.) and recreates the container. Previously, only
  changes to `devcontainer.json` fields were detected, so editing a compose
  file's volumes had no effect until a full `crib rebuild`.

## [0.7.0] - 2026-03-10

### Added

- `crib prune` command removes stale and orphan workspace images. Shows a dry-run
  preview with sizes before prompting for confirmation. `--all` includes orphan
  images from workspaces that no longer exist. `--force` / `-f` skips the prompt.
- `crib remove` now shows a preview of what will be deleted (container ID, images,
  state directory) and prompts for confirmation. Use `--force` / `-f` to skip.
- All crib-managed images are now labeled with `crib.workspace={wsID}`, enabling
  label-based discovery without name-pattern heuristics. Applied via `--label` on
  `docker build` and `--change "LABEL ..."` on `docker commit` (snapshots).
- Build images are automatically removed when a new build replaces them (hash
  change). The old image is deleted after the new one is successfully built.
- `crib remove` now deletes all labeled images for the workspace in addition to
  the container and workspace state.

### Changed

- **Breaking**: Workspace IDs now include a 7-character hash of the absolute
  project path: `{slug}-{hash}` (e.g. `my-app-a1b2c3d`). Workspaces created
  before this change will not be recognized. Run `crib remove` (if still
  accessible) or delete `~/.crib/workspaces/` manually, then run `crib up` in
  each project to create a new workspace with the updated ID.

## [0.6.3] - 2026-03-09

### Security

- OCI feature archives containing symbolic links are now rejected outright.
  Previously, crib rejected symlinks whose static target appeared to escape the
  extraction directory, but a chain of individually-safe symlinks could still
  compose into a directory escape: an earlier symlink pointing to the extraction
  root could be overwritten via the chain, redirecting subsequent file writes
  outside the extraction directory. Feature archives contain scripts and JSON
  and have no legitimate need for symlinks.

## [0.6.2] - 2026-03-09

### Fixed

- `crib stop` followed by `crib up` now resumes from a snapshot when available,
  skipping the full image build and running only resume-flow lifecycle hooks
  (postStartCommand, postAttachCommand). Previously, stopping and starting a
  workspace re-ran the entire creation flow. Both single-container and compose
  paths are covered. `crib rebuild` still forces a full rebuild as before.
- SSH agent socket is now mounted at `/run/ssh-agent.sock` instead of
  `/tmp/ssh-agent.sock`. Docker-in-Docker features remount `/tmp` as a fresh
  tmpfs, which hid the bind-mounted socket and broke SSH agent forwarding in
  DinD containers.

## [0.6.1] - 2026-03-08

### Fixed

- Compose container lookup fallback now filters by service name. Previously,
  when the primary service failed to start, crib could pick up the wrong
  container (e.g. postgres instead of rails-app) and show misleading logs in
  the error message. The fallback now uses `compose ps --format json` with
  service label matching, which also works on podman-compose (unlike
  `compose ps -q <service>` which podman-compose doesn't support).

## [0.6.0] - 2026-03-08

### Added

- `crib cache clean --force` / `-f` flag to skip confirmation prompt.
- DevContainer Feature entrypoints are now applied. Features that declare an
  `entrypoint` (e.g. docker-in-docker starting `dockerd`) now have their
  entrypoint baked into the image. Multiple feature entrypoints are chained via
  a wrapper script. Feature runtime capabilities (`privileged`, `init`,
  `capAdd`, `securityOpt`, `mounts`, `containerEnv`) are now applied at
  container creation time for both single-container and compose paths.

### Security

- Reject OCI feature archives containing symlinks that escape the extraction
  directory (absolute targets or relative traversal). Previously, a malicious
  feature archive could write to arbitrary host paths via symlinks.
- CI workflows now use explicit `permissions: contents: read` instead of
  GitHub's default read-write.

### Changed

- **Breaking**: The `-V` shorthand for `--verbose` has been removed. Use
  `--verbose` instead. The `-v` shorthand remains reserved for `--version`,
  matching CLI conventions.
- CLI now exits with code 2 for usage errors (bad flags, missing arguments)
  instead of code 1, making it easier to distinguish user mistakes from runtime
  failures in scripts.
- Noisy host-specific environment variables (`LS_COLORS`, `DISPLAY`,
  `WAYLAND_DISPLAY`, `XDG_SESSION_*`, `DBUS_SESSION_BUS_ADDRESS`,
  `TERM_PROGRAM`, `COLORTERM`, `DESKTOP_SESSION`, etc.) are now filtered from
  the probed environment. These are meaningless inside containers and cluttered
  the output of `crib run -- env`. Users can still force any filtered variable
  via `remoteEnv` in `devcontainer.json`.

### Fixed

- SSH `known_hosts` could not be written inside the container. The SSH plugin
  created `~/.ssh/` as root but only chowned individual files, leaving the
  directory root-owned. The container user could not create new files in it.
- Plugin environment variables (e.g. `BUNDLE_PATH`, `CARGO_HOME`, `HISTFILE`)
  now survive `crib restart`. Previously, simple restart paths only preserved
  plugin `PathPrepend` entries but silently dropped plugin `Env` values. The
  values survived only because they were present in the stored result from a
  previous `crib up`, but a plugin that changed an env value between restarts
  would have the old stored value win over the fresh one.
- `crib delete` now removes named volumes declared in compose files (e.g.
  database data). Previously, `docker compose down` ran without `--volumes`,
  leaving orphaned volumes behind.
- `crib restart` no longer loses software installed by lifecycle hooks (e.g.
  mise-managed ruby/node) on compose workspaces. The compose override now
  references the snapshot image, so even if the container is recreated, the
  hook-installed state is preserved.
- Compose restart paths (`crib restart`, `crib up` on stopped containers) now
  preserve the stored `ImageName` in workspace state. Previously, these paths
  saved an empty `ImageName`, causing subsequent operations to lose track of
  the feature image.
- Preserve Docker image PATH entries (e.g. `/usr/local/bundle/bin` in ruby
  images) that login shells drop during the env probe. Previously, `crib exec`
  could lose these entries, requiring `bundle exec` or similar wrappers.
- Plugin PATH additions (e.g. `~/.bundle/bin` for bundler cache) now work with
  zsh. Previously relied on `/etc/profile.d/` scripts which zsh doesn't source.
  PATH additions are now injected directly via remoteEnv.
- `crib cache clean` and `crib cache list` no longer require workspace state to
  exist. They now work even if `crib up` was never run or the project was
  deleted.
- `crib cache clean --all` now prompts for confirmation since it removes volumes
  from other projects.
- Sensitive env var values (tokens, keys, passwords) are now redacted in
  `--debug` output. Previously, values like `GITHUB_TOKEN` appeared in
  plaintext in exec command logs.
- `crib restart` and redundant `crib up` no longer lose probed environment
  (mise, rbenv, nvm PATH entries) or plugin PATH additions. Previously, restart
  paths that skip env re-probing overwrote the saved `remoteEnv`, so `crib run`
  could not find tools like `ruby` or `node` after a restart.
- Plugin dispatch in the already-running container path now passes arguments in
  the correct order. Previously, the remote user was passed as the image name,
  causing plugins to see an incorrect image and potentially resolve wrong home
  directory paths.

## [0.5.0] - 2026-03-05

### Added

- `crib run` command: runs commands through a login shell so tools installed by
  version managers (mise, asdf, nvm, rbenv) are available on PATH. Use instead
  of `crib exec` when the command depends on shell init files.
- `crib cache list` and `crib cache clean` commands for inspecting and removing
  package cache volumes. `--all` flag operates across all workspaces.
- Package cache volumes are now per-workspace (`crib-cache-{workspace}-{provider}`)
  instead of shared globally. This prevents cross-contamination between projects.
- `crib logs` command with `--follow`/`-f` and `--tail` flags. Shows container
  logs for single-container workspaces; shows all service logs for compose
  workspaces.
- `crib doctor` command to detect and fix workspace health issues. Checks
  runtime availability, compose availability, orphaned workspaces (source
  directory deleted), dangling containers (crib label but no workspace state),
  and stale plugin data. Use `--fix` to auto-clean.
- **Package cache sharing plugin**: shares host package caches (npm, pip, go,
  cargo, maven, gradle, bundler, apt, downloads) via named Docker volumes.
  Configure in `.cribrc`: `cache = npm, pip, go`. The `downloads` provider is a
  general-purpose cache directory at `~/.cache/crib` (exposed via `CRIB_CACHE`
  env var) for ad hoc file caching. The `bundler` provider sets `BUNDLE_BIN` and
  adds `~/.bundle/bin` to PATH via `/etc/profile.d/`, so gem executables like
  `rspec` work directly in `crib shell` and `crib run`.
- **Build-time cache mounts**: when package cache providers are configured, crib
  attaches BuildKit `--mount=type=cache` directives to DevContainer Feature
  install steps. This speeds up feature installation across rebuilds by reusing
  cached packages (especially apt).
- **Auto-snapshot**: after `crib up` completes create-time hooks, the container
  is committed to a local snapshot image. On subsequent `crib restart`
  recreations, the snapshot is used so hook effects are already baked in. If
  hook definitions change, the snapshot is considered stale and full setup runs
  instead. `crib rebuild` always starts fresh.
- Each plugin now emits a progress line ("Running plugin: \<name\>") during
  `crib up` and `crib rebuild`, visible without any flags.

### Fixed

- `bundler` cache provider: mount volume at `~/.bundle` instead of `~/.bundle/cache`
  to avoid permission errors creating `~/.bundle/bin` (Docker created the parent
  directory as root).
- `crib cache list`: compose workspaces showed the full volume name (including
  compose project prefix) in the PROVIDER column. Also fixed compose override to
  declare volumes with explicit `name:` so new volumes use the expected name.
- Installation `curl` command now works in zsh (URL was missing quotes, causing
  a parse error with the embedded `sed` expression). Archive filenames no longer
  include the version, so `releases/latest/download/crib_linux_amd64.tar.gz` is
  a stable URL that always points to the latest release.
- `.env` files with quoted values (`KEY="value with spaces"`, `KEY='value'`) and
  inline comments (`KEY=value # comment`) now parse correctly. Previously only
  bare `KEY=value` syntax was supported.
- `workspaceMount` strings with a missing `target` field now produce an explicit
  error instead of silently creating an unusable mount.
- Compose workspaces: stderr noise from `compose up` and `compose down` (e.g.
  "No container found", SIGTERM warnings) is now suppressed in normal mode. Use
  `-V` / `--verbose` to see full compose output.
- Compose workspaces: `crib up` and `crib rebuild` now detect when the primary
  container exits immediately after `compose up` (e.g. port conflicts) and
  report a clear error with container logs, instead of cascading into confusing
  plugin copy and lifecycle hook failures.
- Plugin file copies now bail out on the first exec failure instead of logging
  identical errors for every remaining copy.
- Compose workspaces: `crib restart` now includes plugin-injected env vars and
  mounts (package cache, SSH agent, shell history, etc.) in the compose override.
  Previously the simple restart path skipped plugin dispatch, so these were
  missing until a full `crib down && crib up`.

## [0.4.1] - 2026-03-02

### Fixed

- Plugins (coding-agents, ssh, shell-history) now run for Docker Compose
  workspaces with the correct container user. Previously, plugins only ran for
  single-container devcontainers, and the initial fix defaulted to root when
  `remoteUser`/`containerUser` weren't set in `devcontainer.json`, causing
  permission errors (e.g. `zsh: locking failed for /root/.crib_history/.shell_history`).
  The engine now resolves the user from the compose service and image before
  dispatching plugins.
- Compose override files are now written to a system temp directory instead of
  inside `.devcontainer/`. Previously, `chown -R` during container setup could
  change ownership of the override file, making it unremovable by the host user.

## [0.4.0] - 2026-03-01

### Added

- **Plugin system** with bundled plugin support for `pre-container-run`. Plugins
  can inject mounts, environment variables, extra run args, and file copies into
  containers. Fail-open error handling (one broken plugin doesn't block container
  creation). See `docs/plugin-development.md`.
- **coding-agents plugin**: automatically injects Claude Code credentials into
  containers so AI coding tools work without re-authentication. Detects
  `~/.claude/.credentials.json` on the host and copies it into the container.
  Supports a **workspace credentials mode** for teams that need org-specific
  accounts: set `customizations.crib.coding-agents.credentials` to `"workspace"`
  in `devcontainer.json` to persist container-created credentials across rebuilds
  instead of injecting host credentials.
- **ssh plugin**: shares SSH configuration with containers. Forwards the SSH
  agent socket, copies `~/.ssh/config` and public keys, and extracts git SSH
  signing config (`gpg.format=ssh`) into a minimal `.gitconfig`. Private keys
  stay on the host; commit signing works via the forwarded agent (OpenSSH 8.2+).
- **shell-history plugin**: persists bash/zsh history across container
  recreations. Bind-mounts a history directory from workspace state and sets
  `HISTFILE`.
- Automatic port publishing for single-container workspaces. `forwardPorts` and
  `appPort` from `devcontainer.json` are now translated into `--publish` flags on
  `docker run`, so ports work without manual `runArgs` workarounds.
- Published ports shown on `crib ps`, `crib up`, `crib restart`, and `crib rebuild`.
  For compose workspaces, ports are parsed from `docker compose ps` output.
- **Plugin customizations**: plugins now receive `customizations.crib` from
  `devcontainer.json`, enabling per-project plugin configuration.
- Documentation website at [fgrehm.github.io/crib](https://fgrehm.github.io/crib/),
  with `llms.txt` for AI tool discovery.
- `-V` / `--verbose` now prints each lifecycle hook command before running it
  (e.g. `  $ npm install`), making it easier to diagnose hook failures.
- `build.options` from `devcontainer.json` is now passed to `docker build` /
  `podman build` as extra CLI flags (e.g. `--network=host`, `--progress=plain`).
- Object-syntax lifecycle hooks now run their named entries in parallel, matching
  the devcontainer spec. String and array hooks are unchanged (sequential).
- `waitFor` is now respected: a "Container ready." progress message is emitted
  after the specified lifecycle stage completes (default: `updateContentCommand`).

### Changed

- `--debug` now implies `--verbose`: subprocess stdout (hooks, build, compose) is shown
  when debug logging is active.
- `crib shell` now rejects arguments with a helpful error suggesting `crib exec`.

### Fixed

- `crib restart` no longer fails with "cannot determine image name" on
  Dockerfile-based workspaces where the stored result had an empty image name.
  Falls back to rebuilding the image instead of erroring.
- `crib rebuild` no longer fails when no container exists (e.g. first build or
  after manual container removal).
- Container deletion is faster (skips stop grace period).
- `make install` now uses correct permissions.

## [0.3.1] - 2026-02-28

### Fixed

- `down` / `stop` on rootless Podman no longer fails with "no pod with name or
  ID ... found". The `x-podman: { in_pod: false }` override was only passed
  during `up`, so `compose down` tried to remove a pod that never existed.
- `rebuild` now actually rebuilds images. Previously it passed `Recreate: false`
  to `Up`, which took the stored-result shortcut and skipped the image build.
- Environment probe now runs after lifecycle hooks (in addition to before), so
  the persisted environment for `shell`/`exec` includes tools installed by hooks
  (e.g. `mise install` in `bin/setup`).
- Filter mise internal state variables (`__MISE_*`, `MISE_SHELL`) from the probed
  environment. These are session-specific and confused mise when injected into a
  new shell via `crib shell`.

## [0.3.0] - 2026-02-27

### Changed

- Rename `stop` to `down` (alias: `stop`). Now stops and removes the container
  instead of just stopping it, clearing lifecycle hook markers so the next `up`
  runs all hooks from scratch.
- Rename `delete` to `remove` (aliases: `rm`, `delete`). Removes container and
  workspace state.
- Add short aliases: `list` (`ls`), `status` (`ps`), `shell` (`sh`).
- `rebuild` no longer needs to re-save workspace state after removing the
  container (uses `down` instead of the old `delete`).
- Display crib version at the start of `up`, `down`, `remove`, `rebuild`,
  and `restart` commands. Dev builds include commit SHA and build timestamp.
- Suppress noisy compose stdout (container name listings) during up/down/restart.
  Use `--verbose` / `-V` to see full compose output.
- `status` / `ps` now shows all compose service statuses for compose workspaces.

### Fixed

- Lifecycle hooks (`onCreateCommand`, `updateContentCommand`, `postCreateCommand`)
  now run after `down` + `up` cycle. Previously, host-side hook markers persisted
  across container removal, causing hooks to be skipped.
- `restart` for compose workspaces now uses `compose up` instead of `compose restart`,
  fixing failures when dependency services (databases, sidecars) were stopped.
- `up` after `down` for compose workspaces no longer rebuilds images. When a
  stored result exists with a previously built image, the build is skipped and
  services are started directly.

## [0.2.0] - 2026-02-26

### Added

- `crib restart` command with smart change detection
  - No config changes: simple container restart, runs only resume-flow hooks
    (`postStartCommand`, `postAttachCommand`)
  - Safe changes (volumes, mounts, ports, env, runArgs, user): recreates container
    without rebuilding the image, runs only resume-flow hooks
  - Image-affecting changes (image, Dockerfile, features, build args): reports error
    and suggests `crib rebuild`
- `RestartContainer` method in container driver interface
- `Restart` method in compose helper
- Smart Restart section in README
- New project logo

### Changed

- Refactor engine package: extract `change.go`, `restart.go` from `engine.go`
- Deduplicate config parsing (`parseAndSubstitute`) and user resolution (`resolveRemoteUser`)

## [0.1.0] - 2026-02-25

### Added

- Core `crib` CLI for managing dev containers
- Support for Docker and Podman via single OCI driver
- `.devcontainer` configuration parsing, variable substitution, and merging
- All three configuration scenarios: image-based, Dockerfile-based, Docker Compose-based
- DevContainer Features support with OCI image resolution and ordering
- Workspace state management in `~/.crib/workspaces/`
- Implicit workspace resolution from current working directory
- Commands: `up`, `stop`, `delete`, `status`, `list`, `exec`, `shell`, `rebuild`, `version`
- All lifecycle hooks: `initializeCommand`, `onCreateCommand`, `updateContentCommand`,
  `postCreateCommand`, `postStartCommand`, `postAttachCommand`
- `userEnvProbe` support for probing user environment (mise, rbenv, nvm, etc.)
- Image metadata parsing (`devcontainer.metadata` label) with spec-compliant merge rules
- `updateRemoteUserUID` with UID/GID sync and conflict resolution
- Auto-injection of `--userns=keep-id` / `userns_mode: "keep-id"` for rootless Podman
- Container user auto-detection via `whoami` for compose containers
- Early result persistence so `exec`/`shell` work while lifecycle hooks are still running
- Version info on error output for debugging
- Container naming with `crib-{workspace-id}` convention
- Container labeling with `crib.workspace={id}` for discovery
- Build and test tooling (Makefile, golangci-lint v2, pre-commit hooks)
- Debug logging via `--debug` flag
