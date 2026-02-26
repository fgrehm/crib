# Implementation Notes

Notes on quirks, workarounds, and spec compliance gathered during development.

## Quirks and Workarounds

### Rootless Podman requires userns_mode / --userns=keep-id

When running rootless Podman, bind-mounted files are owned by the host user's UID inside a
user namespace. Without `--userns=keep-id`, the container sees these files as owned by
`nobody:nogroup`, breaking all file operations.

crib auto-injects `--userns=keep-id` for single containers and `userns_mode: "keep-id"` in
compose overrides when it detects rootless Podman (non-root UID + `podman` in the runtime
command). This is skipped when the user's compose files already set `userns_mode`.

For compose, the override also sets `x-podman: { in_pod: false }` because podman-compose
creates pods by default and `--userns` and `--pod` are incompatible in Podman.

**Files**: `internal/engine/single.go` (RunOptions), `internal/engine/compose.go`
(generateComposeOverride), `internal/driver/oci/container.go` (buildRunArgs).

### Version managers (mise, rbenv, nvm) not in PATH during lifecycle hooks

Lifecycle hooks run via `sh -c "<command>"`. Tools installed by version managers like
[mise](https://mise.jdx.dev/) activate in `~/.bashrc` (interactive shell), not in
`/etc/profile.d/` (login shell). This means `sh -c` and even `bash -l -c` won't find them.

crib implements the spec's `userEnvProbe` (default: `loginInteractiveShell`) to probe the
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

**Files**: `internal/engine/setup.go` (probeUserEnv, detectUserShell),
`internal/engine/env.go` (mergeEnv).

### Container user detection for compose containers

For single containers, `containerUser` and `remoteUser` from devcontainer.json control which
user runs hooks. For compose containers, the Dockerfile's `USER` directive sets the running
user, but devcontainer.json often doesn't set `remoteUser` or `containerUser`.

If neither is set, crib runs `whoami` inside the container to detect the actual user. If the
detected user is root, it falls through to the default "root" behavior. Non-root users (e.g.
`vscode` from a `USER vscode` directive) are used as the remote user.

**Files**: `internal/engine/single.go` (detectContainerUser, setupAndReturn).

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

**Files**: `internal/engine/engine.go` (Restart, detectConfigChange, restartSimple,
restartWithRecreate), `internal/engine/lifecycle.go` (runResumeHooks).

### Early result persistence

crib saves the workspace result (container ID, workspace folder, remote user) as soon as
those values are known, before UID sync, environment probing, and lifecycle hooks run. This
means `crib exec` and `crib shell` work immediately, even while hooks are still executing
or if they fail. A second save after setup completes updates the result with the probed
`remoteEnv`.

This is particularly useful when iterating on a new devcontainer setup where lifecycle
hooks often fail (missing dependencies, broken scripts, etc.).

**Files**: `internal/engine/engine.go` (Up, saveResult),
`internal/engine/single.go` (setupAndReturn).

### UID/GID sync conflicts with existing users

When `updateRemoteUserUID` is true (the default), crib syncs the container user's UID/GID to
match the host user. On images like `ubuntu:24.04`, standard users/groups may already occupy
the target UID/GID (e.g. the `ubuntu` user at UID 1000). crib detects these conflicts and
moves the conflicting user/group to a free UID/GID before performing the sync.

**Files**: `internal/engine/setup.go` (syncRemoteUserUID, execFindUserByUID,
execFindGroupByGID).

### chown skipped when UIDs already match

After UID sync, if the container and host UIDs already match, crib skips `chown -R` on the
workspace directory. This avoids failures on rootless Podman where `CAP_CHOWN` doesn't work
over bind-mounted files (the kernel denies it even for root inside the user namespace).

**Files**: `internal/engine/setup.go` (setupContainer).

### overrideCommand default differs by scenario

Per the spec, `overrideCommand` defaults to `true` for image/Dockerfile containers and
`false` for compose containers. crib's compose path handles this in the override YAML
generation (injecting entrypoint/command only when the flag is explicitly or implicitly true).
The single container path treats `nil` as `true`.

**Files**: `internal/engine/single.go` (buildRunOptions),
`internal/engine/compose.go` (generateComposeOverride).

### Feature installation for compose containers

Devcontainer features (e.g. `ghcr.io/devcontainers/features/node:1`) need special
handling for compose-based containers. Unlike single containers where crib controls the
entire image build, compose services define their own images or Dockerfiles.

crib handles this by pre-building a feature image on top of the service's base image:

1. Parse compose files to extract the service's image or build config
2. For build-based services, run `compose build` first to produce the base image
3. Generate a feature Dockerfile that layers features on the base image (same
   `GenerateDockerfile` and `PrepareContext` used for single containers)
4. Build the feature image via `doBuild` (with prebuild hash caching)
5. Override the service image in the compose override YAML

The compose `build` step for the primary service is skipped in the main flow since
the feature build already produced the final image. Other services still build normally.

**Files**: `internal/engine/compose.go` (buildComposeFeatures, generateComposeOverride),
`internal/engine/build.go` (buildComposeFeatureImage, resolveComposeContainerUser),
`internal/compose/project.go` (GetServiceInfo).

## Spec Compliance

### Fully Implemented

| Feature | Notes |
|---------|-------|
| Config file discovery | All three search paths |
| Image/Dockerfile/Compose scenarios | All three paths |
| Lifecycle hooks | All 6 hooks with marker-file idempotency |
| DevContainer Features | OCI, HTTPS, local; ordering algorithm; feature lifecycle hooks; compose support |
| Variable substitution | All 7 variables including `${localEnv}` and `${containerEnv}` |
| Image metadata | Parsing `devcontainer.metadata` label, merge rules |
| `updateRemoteUserUID` | UID/GID sync with conflict resolution |
| `userEnvProbe` | Shell detection, env probing, merge with remoteEnv |
| `overrideCommand` | Both single and compose paths |
| `mounts` | String and object format, bind and volume types |
| `init`, `privileged`, `capAdd`, `securityOpt` | Passed through to runtime |
| `runArgs` | Passed through as extra CLI args |
| `workspaceMount` / `workspaceFolder` | Custom mount parsing, variable expansion |
| `containerEnv` / `remoteEnv` | Including `${containerEnv:VAR}` resolution |
| Compose `runServices` | Selective service starting |
| Build options | `dockerfile`, `context`, `args`, `target`, `cacheFrom` |

### Parsed but Not Enforced

These fields are parsed from devcontainer.json and merged from image metadata, but crib
does not act on them. This is intentional for a CLI-only tool.

| Feature | Reason |
|---------|--------|
| `forwardPorts` | Port forwarding is IDE-specific; use `runArgs` with `-p` instead |
| `portsAttributes` | Display/behavior hints for IDE port UI |
| `shutdownAction` | crib manages container lifecycle explicitly via `stop`/`delete` |
| `hostRequirements` | Validation not implemented; runtime will fail naturally |
| `appPort` (legacy) | Spec says use `forwardPorts` instead |

### Not Implemented

| Feature | Description |
|---------|-------------|
| `build.options` | Extra Docker build CLI flags; field parsed but not passed to driver |
| `waitFor` | Field parsed but lifecycle hooks always run sequentially to completion |
| Parallel object hooks | Spec says object-syntax hook entries run in parallel; crib runs them sequentially |
