---
title: Plugin Development
description: How to write bundled plugins for crib.
---

Guide for writing bundled (in-process) plugins. External plugin support is planned but not yet implemented.

## Architecture

```
internal/plugin/
  plugin.go           -> Plugin interface and types
  manager.go          -> Registration and event dispatch
  codingagents/       -> Claude Code credentials plugin
  sandbox/            -> Coding agent sandboxing (bubblewrap + iptables)
  shellhistory/       -> Persistent shell history plugin
  ssh/                -> SSH agent forwarding, keys, and git signing
```

Plugins are registered in `cmd/root.go` via `setupPlugins()` and dispatched by the engine during both `upSingle()` (initial container creation) and `restartRecreateSingle()` (container recreation triggered by `crib restart`).

## Plugin Interface

Every plugin implements `plugin.Plugin`:

```go
type Plugin interface {
    Name() string
    PreContainerRun(ctx context.Context, req *PreContainerRunRequest) (*PreContainerRunResponse, error)
}
```

### Request

`PreContainerRunRequest` provides context about the workspace and the container that is about to be created:

| Field             | Description                                     |
|-------------------|-------------------------------------------------|
| `WorkspaceID`     | Unique workspace identifier                     |
| `WorkspaceDir`    | `~/.crib/workspaces/{id}/` (for staging files)  |
| `SourceDir`       | Project root on host                            |
| `Runtime`         | `"docker"` or `"podman"`                        |
| `ImageName`       | Resolved image name                             |
| `RemoteUser`      | User inside the container (from config)         |
| `WorkspaceFolder` | Path inside container (e.g. `/workspaces/proj`) |
| `ContainerName`   | `crib-{workspace-id}`                           |
| `Customizations`  | `customizations.crib` from devcontainer.json    |

The `Customizations` field contains the `crib` namespace from devcontainer.json customizations. Plugins can look up their own config key:

```go
func getMyConfig(customizations map[string]any) map[string]any {
    if customizations == nil {
        return nil
    }
    if v, ok := customizations["my-plugin"]; ok {
        if m, ok := v.(map[string]any); ok {
            return m
        }
    }
    return nil
}
```

Example devcontainer.json:

```json
{
  "customizations": {
    "crib": {
      "my-plugin": {
        "setting": "value"
      }
    }
  }
}
```

### Response

`PreContainerRunResponse` specifies what to inject. Return `nil` for no-op.

| Field     | Type                | Merge rule                  | When applied              |
|-----------|---------------------|-----------------------------|---------------------------|
| `Mounts`  | `[]config.Mount`    | Appended in plugin order    | Before container creation |
| `Env`     | `map[string]string` | Merged, last plugin wins    | Before container creation |
| `RunArgs` | `[]string`          | Appended in plugin order    | Before container creation |
| `Copies`  | `[]plugin.FileCopy` | Appended in plugin order    | After container creation  |

### FileCopy

`FileCopy` describes a file to inject into the container after creation via `docker exec`:

```go
type FileCopy struct {
    Source      string // path on host
    Target      string // path inside container
    Mode        string // chmod mode (e.g. "0600"), empty for default
    User        string // chown user (e.g. "vscode"), empty for default
    IfNotExists bool   // if true, skip copy when target already exists
}
```

Copies run as root and use `sh -c "mkdir -p <dir> && cat > <file>"` with stdin piped.

## Post-Container-Create (optional)

Plugins that need to run commands inside the container after creation can implement the optional `PostContainerCreator` interface:

```go
type PostContainerCreator interface {
    PostContainerCreate(ctx context.Context, req *PostContainerCreateRequest) error
}
```

This is dispatched after file copies and volume chown in `finalize()`. Errors are logged and skipped (fail-open).

### PostContainerCreateRequest

| Field             | Description                                         |
|-------------------|-----------------------------------------------------|
| `WorkspaceID`     | Unique workspace identifier                         |
| `WorkspaceDir`    | `~/.crib/workspaces/{id}/` (for staging files)      |
| `ContainerID`     | Running container ID                                |
| `RemoteUser`      | User inside the container                           |
| `WorkspaceFolder` | Path inside container                               |
| `Customizations`  | `customizations.crib` from devcontainer.json        |
| `Runtime`         | `"docker"` or `"podman"`                            |
| `ExecFunc`        | Run a command inside the container (output discarded)|
| `ExecOutputFunc`  | Run a command and capture stdout as a string        |

Both exec functions take `(ctx context.Context, cmd []string, user string)`. Use `"root"` for privileged operations (installing packages, chown) and the remote user for user-scoped operations.

## Writing a Plugin

### 1. Create the package

```
internal/plugin/yourplugin/
  plugin.go
  plugin_test.go
```

### 2. Implement the interface

```go
package yourplugin

import (
    "context"
    "github.com/fgrehm/crib/internal/plugin"
)

type Plugin struct{}

func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "your-plugin" }

func (p *Plugin) PreContainerRun(_ context.Context, req *plugin.PreContainerRunRequest) (*plugin.PreContainerRunResponse, error) {
    // Return nil for no-op (e.g. when prerequisite files don't exist).
    // Return response with mounts/env/copies as needed.
    return nil, nil
}

// Optional: implement PostContainerCreator to run commands inside
// the container after creation.
func (p *Plugin) PostContainerCreate(ctx context.Context, req *plugin.PostContainerCreateRequest) error {
    // Install tools, generate config files, etc.
    return req.ExecFunc(ctx, []string{"sh", "-c", "echo hello"}, req.RemoteUser)
}
```

### 3. Register in setupPlugins

In `cmd/root.go`:

```go
import "github.com/fgrehm/crib/internal/plugin/yourplugin"

func setupPlugins(eng *engine.Engine, d *oci.OCIDriver) {
    // ...
    mgr.Register(yourplugin.New())
    // ...
}
```

### 4. Write tests (TDD)

Follow the pattern in existing plugins. Tests should cover:
- Plugin name
- Happy path (returns expected mounts/env/copies)
- No-op path (returns nil when prerequisites missing)
- User variants (vscode, root, empty)
- File staging (if applicable)

## Key Patterns

### Staging directory

Plugins can stage files in `{req.WorkspaceDir}/plugins/{plugin-name}/`. This directory persists across container recreations but is scoped to the workspace.

### Inferring remote home

The container doesn't exist during `pre-container-run`, so you can't run `getent` or similar. Use the convention:

```go
func inferRemoteHome(user string) string {
    if user == "" || user == "root" {
        return "/root"
    }
    return "/home/" + user
}
```

This matches devcontainer base images and covers the vast majority of cases.

### Bind mount: file vs directory

**Never bind-mount a single file** if any process inside the container might do an atomic rename on it (write to `.file.new`, then `rename()` over the original). This causes `EBUSY` errors on Docker/Podman because the mount holds the inode.

Instead, **bind-mount the parent directory** and point env vars to the file inside it. This lets processes create temp files alongside and rename freely.

Example from the shell-history plugin:
- Mount: `~/.crib/workspaces/{id}/plugins/shell-history/` -> `~/.crib_history/`
- Env: `HISTFILE=~/.crib_history/.shell_history`

The coding-agents plugin uses `FileCopy` instead of mounts for the same reason (Claude Code does atomic renames on `~/.claude.json`).

### Error handling

- Return `error` for problems that should be visible to the user.
- The manager logs plugin errors as warnings and skips the plugin (fail-open), so one broken plugin doesn't block container creation.
- Return `nil, nil` for graceful no-op (e.g. prerequisite files don't exist on the host).

## Execution Flow

```
upSingle()                               restartRecreateSingle()
  buildImage()                             removeContainer()
  buildRunOptions()      <- base config    buildRunOptions()      <- base config
  runPreContainerRunPlugins()              runPreContainerRunPlugins()
    manager.RunPreContainerRun()             manager.RunPreContainerRun()
      plugin1.PreContainerRun()                plugin1.PreContainerRun()
      plugin2.PreContainerRun()                plugin2.PreContainerRun()
      ...merge responses...                    ...merge responses...
    merge into RunOptions                    merge into RunOptions
  driver.RunContainer()                    driver.RunContainer()
  finalize()                               finalize()
    execPluginCopies()                       execPluginCopies()
    chownPluginVolumes()                     (skip on restart)
    dispatchPostContainerCreate()            dispatchPostContainerCreate()
      plugin1.PostContainerCreate()            plugin1.PostContainerCreate()
    resolveRemoteUser()                      (already set)
    saveResult() (early)                     saveResult() (early)
    setupContainer() / hooks                 resumeHooks()
    saveResult() (final)                     saveResult() (final)
```

## Bundled Plugins

### coding-agents

Shares Claude Code credentials with containers. Two modes:

**Host mode (default):** Copies `~/.claude/.credentials.json` from the host into the container. Uses `FileCopy` (not bind mounts) because Claude Code does atomic renames on `~/.claude.json`.

**Workspace mode:** Configured via devcontainer.json:

```json
{
  "customizations": {
    "crib": {
      "coding-agents": {
        "credentials": "workspace"
      }
    }
  }
}
```

In workspace mode, host credentials are not injected. Instead, a persistent directory is bind-mounted to `~/.claude/` inside the container. The user authenticates inside the container on first use, and credentials survive container rebuilds.

A minimal `~/.claude.json` with `{"hasCompletedOnboarding":true}` is re-injected via FileCopy on each rebuild to skip the Claude Code onboarding flow. This file is not persisted because Claude Code does atomic renames on it.

### shell-history

Persists bash/zsh history across container recreations by bind-mounting a history directory from workspace state and setting `HISTFILE`.

### sandbox

Restricts coding agents' filesystem and network access inside containers using [bubblewrap](https://github.com/containers/bubblewrap). Configured via devcontainer.json:

```json
{
  "customizations": {
    "crib": {
      "sandbox": {
        "denyRead": ["~/.ssh", "~/.claude"],
        "blockLocalNetwork": true,
        "blockCloudProviders": true,
        "aliases": ["claude", "pi", "aider"]
      }
    }
  }
}
```

Uses both lifecycle events:
- **PreContainerRun**: adds `--cap-add=NET_ADMIN --cap-add=NET_RAW` when network blocking is enabled.
- **PostContainerCreate**: installs bubblewrap, generates wrapper scripts at `~/.local/bin/sandbox` and alias wrappers for configured commands.

Auto-discovers other plugins' artifacts (ssh keys, credentials, shell history) and generates deny rules to protect them. Network blocking uses iptables OUTPUT chain rules for RFC 1918 and cloud metadata endpoints, and ipset (`hash:net`) for cloud provider IP ranges (AWS, GCP, Azure, Oracle Cloud, Cloudflare; embedded in binary, updated via `scripts/update-cloud-ips.sh`).

See the [sandbox guide](sandbox.md) for full configuration reference.

### ssh

Shares SSH configuration and enables agent forwarding. Four components:

1. **SSH agent forwarding:** Bind-mounts the host's `SSH_AUTH_SOCK` socket into the container at `/tmp/ssh-agent.sock` and sets the `SSH_AUTH_SOCK` env var. No-op if the agent is not running.

2. **SSH config:** Copies `~/.ssh/config` into the container so host aliases, proxy settings, and other SSH config are available.

3. **SSH public keys:** Copies `*.pub` files from `~/.ssh/` into the container. Private keys are not copied. Git commit signing works via the forwarded SSH agent (requires OpenSSH 8.2+, which can sign using only the public key plus the agent).

4. **Git SSH signing config:** If the host's git config has `gpg.format = ssh`, the plugin extracts signing-related settings (`user.name`, `user.email`, `user.signingkey`, `gpg.format`, `gpg.ssh.program`, `commit.gpgsign`, `tag.gpgsign`) and generates a minimal `.gitconfig` for the container. The `user.signingkey` path is rewritten from the host home to the container home. Skipped entirely if git is not configured for SSH signing.

## Future: External Plugins

Currently all plugins are bundled Go code compiled into the crib binary. A planned external plugin system will let users write plugins as standalone executables in any language. The protocol uses environment variables for input and JSON on stdout for output.

To illustrate what this looks like, here are the two existing bundled plugins reimagined as bash scripts.

### shell-history (bash version)

```bash
#!/usr/bin/env bash
# shell-history — persist bash/zsh history across container recreations
set -euo pipefail

PLUGIN_DIR="${CRIB_WORKSPACE_DIR}/plugins/shell-history"
HIST_FILE=".shell_history"

# Infer remote home from user.
if [ -z "${CRIB_REMOTE_USER}" ] || [ "${CRIB_REMOTE_USER}" = "root" ]; then
  REMOTE_HOME="/root"
else
  REMOTE_HOME="/home/${CRIB_REMOTE_USER}"
fi

MOUNT_TARGET="${REMOTE_HOME}/.crib_history"

# Create plugin dir and touch the history file if it doesn't exist.
mkdir -p "${PLUGIN_DIR}"
[ -f "${PLUGIN_DIR}/${HIST_FILE}" ] || touch "${PLUGIN_DIR}/${HIST_FILE}"

# Output JSON response.
cat <<JSON
{
  "mounts": [
    {
      "type": "bind",
      "source": "${PLUGIN_DIR}",
      "target": "${MOUNT_TARGET}"
    }
  ],
  "env": {
    "HISTFILE": "${MOUNT_TARGET}/${HIST_FILE}"
  }
}
JSON
```

### coding-agents (bash version)

```bash
#!/usr/bin/env bash
# coding-agents — inject Claude Code credentials into the container
set -euo pipefail

CREDS="${HOME}/.claude/.credentials.json"

# No-op if credentials don't exist on the host.
if [ ! -f "${CREDS}" ]; then
  echo '{}'
  exit 0
fi

# Scope staging directory per agent so different tools don't collide.
PLUGIN_DIR="${CRIB_WORKSPACE_DIR}/plugins/coding-agents/claude-code"
mkdir -p "${PLUGIN_DIR}"

# Stage credentials.
cp "${CREDS}" "${PLUGIN_DIR}/credentials.json"
chmod 600 "${PLUGIN_DIR}/credentials.json"

# Generate minimal config to skip onboarding.
echo '{"hasCompletedOnboarding":true}' > "${PLUGIN_DIR}/claude.json"

# Infer remote home and owner.
if [ -z "${CRIB_REMOTE_USER}" ] || [ "${CRIB_REMOTE_USER}" = "root" ]; then
  REMOTE_HOME="/root"
  OWNER="root"
else
  REMOTE_HOME="/home/${CRIB_REMOTE_USER}"
  OWNER="${CRIB_REMOTE_USER}"
fi

# Output JSON response with file copies (not mounts, to avoid EBUSY on
# atomic renames).
cat <<JSON
{
  "copies": [
    {
      "source": "${PLUGIN_DIR}/credentials.json",
      "target": "${REMOTE_HOME}/.claude/.credentials.json",
      "mode": "0600",
      "user": "${OWNER}"
    },
    {
      "source": "${PLUGIN_DIR}/claude.json",
      "target": "${REMOTE_HOME}/.claude.json",
      "user": "${OWNER}"
    }
  ]
}
JSON
```

### Protocol Summary

External plugins receive context via environment variables set by crib:

| Variable | Description |
|----------|-------------|
| `CRIB_EVENT` | Event name (e.g. `pre-container-run`) |
| `CRIB_WORKSPACE_ID` | Workspace identifier |
| `CRIB_WORKSPACE_DIR` | State directory (`~/.crib/workspaces/{id}/`) |
| `CRIB_SOURCE_DIR` | Project root on host |
| `CRIB_RUNTIME` | `docker` or `podman` |
| `CRIB_REMOTE_USER` | Container user (from config) |
| `CRIB_WORKSPACE_FOLDER` | Workspace path inside container |
| `CRIB_CONTAINER_NAME` | Container name (`crib-{workspace-id}`) |
| `CRIB_VERBOSE` | `1` if verbose mode is on |

Plugins write JSON to stdout. Stderr is for diagnostic messages (suppressed in normal mode, shown in verbose mode). Exit code 0 means success, non-zero means the plugin failed (fail-open: crib logs a warning and continues).

Plugin configuration is delivered via stdin as JSON (from `customizations.crib.plugins.<name>` in devcontainer.json or `~/.config/crib/config.toml`).