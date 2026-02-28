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
  shellhistory/       -> Persistent shell history plugin
```

Plugins are registered in `cmd/root.go` via `setupPlugins()` and dispatched by the engine during `upSingle()`.

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
    Source string // path on host
    Target string // path inside container
    Mode   string // chmod mode (e.g. "0600"), empty for default
    User   string // chown user (e.g. "vscode"), empty for default
}
```

Copies run as root and use `sh -c "mkdir -p <dir> && cat > <file>"` with stdin piped.

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
upSingle()
  buildImage()
  buildRunOptions()                    <- base config
  runPreContainerRunPlugins()          <- plugins inject mounts/env/runArgs
    manager.RunPreContainerRun()
      plugin1.PreContainerRun()
      plugin2.PreContainerRun()
      ...merge responses...
    merge into RunOptions
  driver.RunContainer()                <- container created with merged options
  execPluginCopies()                   <- file copies injected via docker exec
  setupAndReturn()                     <- lifecycle hooks, env probe, etc.
```
