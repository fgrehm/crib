---
title: Implementation Notes
description: Quirks, workarounds, and spec compliance status.
---

Notes on quirks, workarounds, and spec compliance gathered during development.

## Quirks and Workarounds

### Rootless Podman requires userns_mode / --userns=keep-id

When running rootless Podman, bind-mounted files are owned by the host user's UID inside a
user namespace. Without `--userns=keep-id`, the container sees these files as owned by
`nobody:nogroup`, breaking all file operations.

`crib` auto-injects `--userns=keep-id` for single containers and `userns_mode: "keep-id"` in
compose overrides when it detects rootless Podman (non-root UID + `podman` in the runtime
command). This is skipped when the user's compose files already set `userns_mode`.

For compose, the override also sets `x-podman: { in_pod: false }` because podman-compose
creates pods by default and `--userns` and `--pod` are incompatible in Podman.

The same `x-podman: { in_pod: false }` directive must also be passed during `compose down`.
Without it, podman-compose tries to remove a pod named `pod_crib-<id>` that was never created,
causing a "no pod with name or ID ... found" error. `composeDown` generates a temporary
override for this.

**Files**:

- `internal/engine/single.go` (`RunOptions`)
- `internal/engine/compose.go` (`generateComposeOverride`, `composeDown`, `writePodmanDownOverride`)
- `internal/driver/oci/container.go` (`buildRunArgs`)

### Version managers (mise, rbenv, nvm) not in PATH during lifecycle hooks

Lifecycle hooks run via `sh -c "<command>"`. Tools installed by version managers like
[mise](https://mise.jdx.dev/) activate in `~/.bashrc` (interactive shell), not in
`/etc/profile.d/` (login shell). This means `sh -c` and even `bash -l -c` won't find them.

`crib` implements the spec's `userEnvProbe` (default: `loginInteractiveShell`) to probe the
container user's environment once during setup, then passes the probed variables to all
lifecycle hooks via `docker exec -e` flags. The probed env is also persisted in
`result.json` so `crib exec` and `crib shell` inherit it automatically.

The probe shell type maps to:

| userEnvProbe           | Shell flags |
|------------------------|-------------|
| `loginShell`           | `-l -c env` |
| `interactiveShell`     | `-i -c env` |
| `loginInteractiveShell`| `-l -i -c env` |
| `none`                 | skip probing |

**Files**:

- `internal/engine/setup.go` (`probeUserEnv`, `detectUserShell`)
- `internal/engine/env.go` (`mergeEnv`)

### Container user detection for compose containers

For single containers, `containerUser` and `remoteUser` from `devcontainer.json` control which
user runs hooks. For compose containers, the Dockerfile's `USER` directive sets the running
user, but `devcontainer.json` often doesn't set `remoteUser` or `containerUser`.

There are two detection mechanisms, used at different stages:

**Pre-start (for plugins):** `resolveComposeUser()` runs before the container exists, so plugins
get the correct home directory for mounts and file copies. It checks, in order:
1. `remoteUser`/`containerUser` from devcontainer.json (if set, returns early).
2. The `user:` directive from the compose service definition.
3. For build-based services (`build:` instead of `image:`), parses the Dockerfile to find the
   last `USER` instruction and the base image (`resolveComposeDockerfileInfo`).
4. Inspects the base image metadata via `docker image inspect`.

**Post-start (for hooks):** `detectContainerUser()` runs `whoami` inside the running container.
If the detected user is root, it falls through to the default "root" behavior. Non-root users
(e.g. `vscode` from a `USER vscode` directive) are used as the remote user.

**Files**:

- `internal/engine/compose.go` (`resolveComposeUser`, `resolveComposeDockerfileInfo`)
- `internal/engine/build.go` (`resolveComposeContainerUser`)
- `internal/engine/single.go` (`detectContainerUser`, `setupAndReturn`)

### Smart restart with change detection

`crib restart` compares the current devcontainer config against the stored config from the
last `crib up` to determine the minimal action needed:

- **No changes**: Simple `docker restart` / `docker compose restart`, then run the spec's
  Resume Flow hooks (`postStartCommand` + `postAttachCommand`).
- **Safe changes** (volumes, mounts, ports, env, runArgs, user, etc.): Recreate the
  container with the new config, then run Resume Flow hooks only. Creation-time hooks
  (`onCreateCommand`, `updateContentCommand`, `postCreateCommand`) are skipped since their
  marker files still exist.
- **Image-affecting changes** (image, Dockerfile, features, build args): Error with a
  message suggesting `crib rebuild`, since the image needs to be rebuilt.

This follows the devcontainer spec's distinction between Creation Flow (all hooks) and
Resume Flow (only `postStartCommand` + `postAttachCommand`). The result is that tweaking
a volume mount or environment variable takes seconds instead of minutes.

Change detection uses JSON comparison of the stored `MergedConfig` against a freshly parsed
and substituted config. Fields are classified as "image-affecting" or "safe" based on
whether they require a new image build or just container runtime configuration.

**Files**:

- `internal/engine/engine.go` (`Restart`, `detectConfigChange`, `restartSimple`, `restartWithRecreate`)
- `internal/engine/lifecycle.go` (`runResumeHooks`)

### Early result persistence

`crib` saves the workspace result (container ID, workspace folder, remote user) as soon as
those values are known, before UID sync, environment probing, and lifecycle hooks run. This
means `crib exec` and `crib shell` work immediately, even while hooks are still executing
or if they fail. A second save after setup completes updates the result with the probed
`remoteEnv`.

This is particularly useful when iterating on a new devcontainer setup where lifecycle
hooks often fail (missing dependencies, broken scripts, etc.).

**Files**:

- `internal/engine/engine.go` (`Up`, `saveResult`)
- `internal/engine/single.go` (`setupAndReturn`)

### UID/GID sync conflicts with existing users

When `updateRemoteUserUID` is true (the default), `crib` syncs the container user's UID/GID to
match the host user. On images like `ubuntu:24.04`, standard users/groups may already occupy
the target UID/GID (e.g. the `ubuntu` user at UID 1000). `crib` detects these conflicts and
moves the conflicting user/group to a free UID/GID before performing the sync.

**Files**:

- `internal/engine/setup.go` (`syncRemoteUserUID`, `execFindUserByUID`, `execFindGroupByGID`)

### chown skipped when UIDs already match

After UID sync, if the container and host UIDs already match, `crib` skips `chown -R` on the
workspace directory. This avoids failures on rootless Podman where `CAP_CHOWN` doesn't work
over bind-mounted files (the kernel denies it even for root inside the user namespace).

**Files**:

- `internal/engine/setup.go` (`setupContainer`)

### Feature entrypoints and runtime capabilities

DevContainer Features can declare an `entrypoint` in `devcontainer-feature.json`. These
scripts typically start a daemon and then chain via `exec "$@"` so the container's normal
command runs after the daemon is ready (e.g. docker-in-docker starts `dockerd`).

Features can also declare runtime capabilities (`privileged`, `init`, `capAdd`,
`securityOpt`, `mounts`, `containerEnv`) that must be applied at container creation time,
not during the image build.

`crib` handles these in two separate phases:

**Image build (Dockerfile generation):** `GenerateDockerfile` in `internal/feature/dockerfile.go`
bakes entrypoints into the image. For a single feature entrypoint, it emits a simple
`ENTRYPOINT ["/path/to/script"]`. For multiple features, it generates a wrapper script at
`/usr/local/share/crib-entrypoint.sh` that chains entrypoints in order (later features wrap
earlier ones): `exec /last.sh /prev.sh ... /first.sh "$@"`.

**Container creation (runtime capabilities):** `applyFeatureMetadata` in `internal/engine/single.go`
applies `privileged`, `init`, `capAdd`, `securityOpt`, `mounts`, and `containerEnv` from
feature metadata to `RunOptions`. For compose, `generateComposeOverride` writes these into
the override YAML.

When features declare entrypoints and `overrideCommand` is true (the default for
image/Dockerfile containers), only `CMD` is overridden, not `ENTRYPOINT`. This preserves
the feature entrypoint while still keeping the container alive with a sleep loop. The
`HasFeatureEntrypoints` flag is persisted in `result.json` so restart paths that don't
rebuild the image can apply the same logic.

Feature-declared volume mounts (e.g. docker-in-docker's `/var/lib/docker`) use named Docker
volumes (`dind-var-lib-docker-${devcontainerId}`). Named volumes are managed by the Docker/Podman
daemon and persist independently of containers. This means the volume's contents (layer cache,
container state, etc.) survive `crib restart`, `crib rebuild`, and even `crib remove` followed
by `crib up`, as long as the volume itself isn't explicitly deleted. For docker-in-docker, this
means image layers built inside the container are cached across rebuilds.

**Files**:

- `internal/feature/dockerfile.go` (`GenerateDockerfile`)
- `internal/engine/single.go` (`buildRunOptions`, `applyFeatureMetadata`)
- `internal/engine/compose.go` (`generateComposeOverride`)
- `internal/engine/build.go` (`buildResult.hasEntrypoints`, `featureToMetadata`)
- `internal/workspace/result.go` (`HasFeatureEntrypoints`)

### overrideCommand default differs by scenario

Per the spec, `overrideCommand` defaults to `true` for image/Dockerfile containers and
`false` for compose containers. `crib`'s compose path handles this in the override YAML
generation (injecting entrypoint/command only when the flag is explicitly or implicitly true).
The single container path treats `nil` as `true`.

When features set an `ENTRYPOINT` in the image, `overrideCommand: true` overrides only
`CMD` (not `ENTRYPOINT`), so the feature daemon starts before the keep-alive command.

**Files**:

- `internal/engine/single.go` (`buildRunOptions`)
- `internal/engine/compose.go` (`generateComposeOverride`)

### Plugins must be wired into both single-container and compose paths

The engine has two separate code paths for container creation: `upSingle()` for image/Dockerfile
devcontainers and `upCompose()` for Docker Compose devcontainers. They diverge because single
containers use `docker run` with `RunOptions`, while compose delegates to `docker compose up`
with a generated override YAML.

Any feature that affects container creation (plugins, mounts, env vars, labels) must be wired
into **both** paths. The shared entry point is `dispatchPlugins()`, which builds the plugin
request and returns the response without merging it into any target:

- **Single-container**: `runPreContainerRunPlugins()` merges the response into `RunOptions`
  (mounts, env, runArgs), then `execPluginCopies()` runs after container creation.
- **Compose**: the response is passed to `generateComposeOverride()` which writes plugin
  mounts as `volumes:` entries and plugin env as `environment:` entries in the override YAML.
  `runArgs` are ignored (compose owns the container config). `execPluginCopies()` runs after
  `compose up` finds the container.

The `restart.go` file has the same split: `restartRecreateSingle` vs `restartRecreateCompose`.

**Files**:

- `internal/engine/single.go` (`dispatchPlugins`, `runPreContainerRunPlugins`, `execPluginCopies`)
- `internal/engine/compose.go` (`upCompose`, `generateComposeOverride`)
- `internal/engine/restart.go` (`restartRecreateSingle`, `restartRecreateCompose`)

### Feature installation for compose containers

DevContainer Features (e.g. `ghcr.io/devcontainers/features/node:1`) need special
handling for compose-based containers. Unlike single containers where `crib` controls the
entire image build, compose services define their own images or Dockerfiles.

`crib` handles this by pre-building a feature image on top of the service's base image:

1. Parse compose files to extract the service's image or build config
2. For build-based services, run `compose build` first to produce the base image
3. Generate a feature Dockerfile that layers features on the base image (same
   `GenerateDockerfile` and `PrepareContext` used for single containers)
4. Build the feature image via `doBuild` (with prebuild hash caching)
5. Override the service image in the compose override YAML

The compose `build` step for the primary service is skipped in the main flow since
the feature build already produced the final image. Other services still build normally.

`build.options` (extra Docker build CLI flags) applies to the feature image build
(step 4 above, via `doBuild`) but **not** to the base service image build (step 2,
via `compose build`). This is spec-correct: the compose service image is managed by
`docker-compose.yml`, not by `devcontainer.json`'s `build` section. If you need
extra flags for the compose service build, set them in the compose file directly
(e.g. `build.args`, `build.network`).

**Files**:

- `internal/engine/compose.go` (`buildComposeFeatures`, `generateComposeOverride`)
- `internal/engine/build.go` (`buildComposeFeatureImage`, `resolveComposeContainerUser`)
- `internal/compose/project.go` (`GetServiceInfo`)

### Environment probe runs twice: before and after lifecycle hooks

`probeUserEnv` runs the user's login shell (`zsh -l -i -c env`) to capture environment
variables set by shell profile files (mise, nvm, rbenv, etc.). This probe runs twice
during `setupContainer`:

1. **Before hooks**: provides lifecycle hooks with the user's shell environment (PATH,
   tool paths, etc.) so hooks don't need to explicitly set up their own environment.
2. **After hooks**: captures any changes made by hooks (e.g. `mise install` adding new
   tool paths to PATH). This is the version that gets persisted for `crib shell`/`crib exec`.

Without the post-hook probe, the saved PATH would be missing tools installed during
lifecycle hooks (e.g. a `bin/setup` script that runs `mise install`).

Tool-manager internal state variables (`__MISE_*`, `MISE_SHELL`) are filtered from the
probed env. These are session-specific and would confuse tool managers when injected into
a new shell session via `crib shell`.

**Files**:

- `internal/engine/setup.go` (`setupContainer`)
- `internal/engine/env.go` (`mergeEnv`)

### TTY detection for exec uses isatty, not ModeCharDevice

`crib exec` passes `-i -t` to `docker exec` / `podman exec` only when stdin is an
interactive terminal. The detection must use a proper `isatty` syscall
(`term.IsTerminal(fd)`) rather than Go's `os.ModeCharDevice` file mode check.

`/dev/null` is a character device on Linux, so `ModeCharDevice` returns true for it.
This causes `crib exec` to pass `-t` when stdin is `/dev/null` (e.g. in CI, pipes,
or `exec.Command` with no stdin). Docker strictly validates the TTY and errors with
"the input device is not a TTY." Podman silently ignores `-t` without a real TTY.

**Files**:

- `cmd/exec.go` (`stdinIsTerminal`)

### Image lifecycle management

`crib` labels all images it builds or commits with `crib.workspace={wsID}` (the same
label key used for containers). This enables discovery via `docker images --filter
label=crib.workspace` without relying on name-pattern heuristics.

| Image type | How the label is applied |
|------------|------------------------|
| Build image (`crib-{wsID}:{hash}`) | `--label` flag on `docker build` / `podman build` |
| Snapshot image (`crib-{wsID}:snapshot`) | `--change "LABEL ..."` on `docker commit` / `podman commit` |

Compose-built images (those produced by `docker compose build`) are not labeled because
adding a `build:` section to the compose override triggers a build attempt even for
image-only services that have no Dockerfile.

Images are cleaned up automatically at three points:

1. **During build:** when the prebuild hash changes, the previous build image is removed
   before saving the new result. Base images (those not prefixed `crib-`) are never touched.
2. **On `crib remove`:** all labeled images for the workspace are swept via `ListImages`,
   plus the active build image from `result.json`.
3. **On `crib prune`:** stale images (labeled but not referenced by `result.json`) and
   orphan images (workspace no longer exists in `~/.crib/workspaces/`) are removed.
   Supports `--all` (global) and dry-run preview with sizes.

All removals are best-effort: failures are logged and skipped so a single in-use image
doesn't block cleanup of the rest.

Existing unlabeled images from before this change are not discovered by label-based
cleanup. Clean them up manually with `docker rmi $(docker images --filter
reference='crib-*' -q)`.

**Files**:

- `internal/driver/oci/image.go` (`ListImages`, `BuildOptions.Labels`, `CommitContainer`)
- `internal/engine/build.go` (`cleanupPreviousBuildImage`)
- `internal/engine/engine.go` (`cleanupWorkspaceImages`, `PreviewRemove`)
- `internal/engine/prune.go` (`PruneImages`)
- `cmd/prune.go` (`crib prune`)

### Sandbox plugin discovers other plugins via workspace state directory

The `sandbox` plugin needs to know which sensitive files other plugins have
injected (SSH config, Claude credentials) so it can automatically deny reads
on them. Rather than changing the plugin interface to support ordering or
inter-plugin communication, the plugin scans `{workspaceDir}/plugins/*/`
at dispatch time.

This works because all bundled plugins stage their artifacts in predictable
subdirectories:

| Plugin | State directory | Sensitive artifacts |
|--------|----------------|---------------------|
| `codingagents` | `plugins/coding-agents/claude-code/` | `credentials.json` |
| `ssh` | `plugins/ssh/` | `config`, `*.pub` |
| `shellhistory` | `plugins/shell-history/` | `.shell_history` (may contain credentials, e.g. `export TOKEN=...`) |
| `packagecache` | (uses named volumes, no staged files) | n/a |

The plugin maps discovered artifacts to container paths using
`InferRemoteHome(remoteUser)` and generates deny/allow rules for the
[`bubblewrap`](https://github.com/containers/bubblewrap) wrapper script.
User-specified `denyRead`/`denyWrite`/`allowWrite` in
`customizations.crib.sandbox` are merged on top.

If a new plugin stages files in a directory the `sandbox` plugin doesn't
recognize, those files are not automatically protected. New plugins must be
registered in the sandbox plugin's discovery logic, or the user must add
explicit deny rules.

See [ADR 002](decisions/002-sandbox-plugin.md) for the full design.

**Files:**

- `internal/plugin/sandbox/`

### `bubblewrap` inside containers requires user namespaces

`bubblewrap` needs `CAP_SYS_ADMIN` (via setuid root installation) or
unprivileged user namespaces to function.

- **Debian 11+**: unprivileged user namespaces enabled by default via
  `kernel.unprivileged_userns_clone=1`. `bubblewrap` works out of the box.
- **Ubuntu 23.10+/24.04+**: the sysctl is set to `1`, but AppArmor
  [restricts unprivileged user namespace creation](https://ubuntu.com/blog/ubuntu-23-10-restricted-unprivileged-user-namespaces)
  via `kernel.apparmor_restrict_unprivileged_userns=1`. `bubblewrap`
  [fails unless an explicit AppArmor profile is added](https://github.com/containers/bubblewrap/issues/632).
  The `@anthropic-ai/sandbox-runtime` has the
  [same issue](https://github.com/anthropic-experimental/sandbox-runtime/issues/74).
- **Alpine and hardened images**: may need `--privileged` or
  `--cap-add=SYS_ADMIN` on the outer container.

The `sandbox` plugin currently does not probe whether `bubblewrap` works at
post-create time. If namespaces are blocked, `bwrap` fails at runtime when
the agent is launched (not at container setup time). A future improvement
could add a smoke test and warn (not fail) if it doesn't work.

Claude Code's sandbox runtime offers
[`enableWeakerNestedSandbox`](https://code.claude.com/docs/en/sandboxing#security-limitations)
for Docker environments without privileged namespaces, trading security for
compatibility. Worth investigating whether a similar fallback is needed here.

**Files:**

- `internal/plugin/sandbox/`

## Spec Compliance

### Fully Implemented

| Feature | Notes |
|---------|-------|
| Config file discovery | All three search paths |
| Image/Dockerfile/Compose scenarios | All three paths |
| Lifecycle hooks | All 6 hooks with marker-file idempotency |
| DevContainer Features | OCI, HTTPS, local; ordering algorithm; feature lifecycle hooks; compose support; entrypoints and runtime capabilities (`privileged`, `init`, `capAdd`, `securityOpt`, `mounts`, `containerEnv`) |
| Variable substitution | All 7 variables including `${localEnv}` and `${containerEnv}` |
| Image metadata | Parsing `devcontainer.metadata` label, merge rules |
| `updateRemoteUserUID` | UID/GID sync with conflict resolution |
| `userEnvProbe` | Shell detection, env probing, merge with remoteEnv |
| `overrideCommand` | Both single and compose paths |
| `mounts` | String and object format, bind and volume types |
| `forwardPorts` | Published as `-p` flags for single containers; compose uses native port config |
| `appPort` (legacy) | Same handling as `forwardPorts`, deduplicated |
| `init`, `privileged`, `capAdd`, `securityOpt` | Passed through to runtime |
| `runArgs` | Passed through as extra CLI args |
| `workspaceMount` / `workspaceFolder` | Custom mount parsing, variable expansion |
| `containerEnv` / `remoteEnv` | Including `${containerEnv:VAR}` resolution |
| Compose `runServices` | Selective service starting |
| Build options | `dockerfile`, `context`, `args`, `target`, `cacheFrom`, `options` (extra CLI flags; for compose, applies to feature layer builds only — see quirk above) |
| `waitFor` | "Container ready." progress message fires after the named stage; all hooks still run to completion; default `updateContentCommand` |
| Parallel object hooks | Object-syntax hooks (named entries) run concurrently via `errgroup`; all must succeed |

### Parsed but Not Enforced

These fields are parsed from `devcontainer.json` and merged from image metadata, but `crib`
does not act on them. This is intentional for a CLI-only tool.

| Feature | Reason |
|---------|--------|
| `portsAttributes` | Display/behavior hints for IDE port UI |
| `shutdownAction` | `crib` manages container lifecycle explicitly via `down`/`remove` |
| `hostRequirements` | Validation not implemented; runtime will fail naturally |

