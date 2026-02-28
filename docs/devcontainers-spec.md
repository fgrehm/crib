---
title: DevContainer Spec Reference
description: A distilled reference of the Dev Container Specification for quick lookup.
---

:::tip[🤖 AI-generated reference]
This page was distilled from the [Dev Container Specification](https://containers.dev/implementors/spec/) by Claude (Opus 4.6) for quick lookup when working on crib. It may contain inaccuracies or be out of date. For the authoritative reference, see the [official spec](https://containers.dev/implementors/spec/).
:::

---

## Config File Discovery

> [Spec: Dev Containers](https://containers.dev/implementors/spec/)

Tools search for `devcontainer.json` in this order:

1. `.devcontainer/devcontainer.json`
2. `.devcontainer.json`
3. `.devcontainer/<folder>/devcontainer.json` (one level deep only)

Multiple files may coexist. When more than one is found, the tool should let the user choose.
The search starts at the **project workspace folder**, typically the git repository root.

---

## Three Configuration Scenarios

> [Spec: Dev Containers](https://containers.dev/implementors/spec/)

| Scenario | Required Fields | Notes |
|---|---|---|
| **Image-based** | `image` | Pulls a container image directly |
| **Dockerfile-based** | `build.dockerfile` | Builds from a Dockerfile |
| **Docker Compose-based** | `dockerComposeFile`, `service` | Uses Compose to orchestrate one or more services |

Compose configurations handle images and Dockerfiles natively, so `image` and `build.dockerfile`
are not used in that scenario.

---

## devcontainer.json Properties

> [JSON Reference](https://containers.dev/implementors/json_reference/)

### General Properties (All Scenarios)

| Property | Type | Default | Description |
|---|---|---|---|
| `name` | string | - | Display name for the dev container |
| `features` | object | - | Feature IDs mapped to their options |
| `overrideFeatureInstallOrder` | string[] | - | Override automatic Feature install ordering |
| `forwardPorts` | (number\|string)[] | `[]` | Ports to forward from container to host |
| `portsAttributes` | object | - | Per-port configuration (see [Port Attributes](#port-attributes)) |
| `otherPortsAttributes` | object | - | Defaults for ports not listed in `portsAttributes` |
| `containerEnv` | object | - | Env vars set on the container itself |
| `remoteEnv` | object | - | Env vars injected by the tool post-ENTRYPOINT |
| `containerUser` | string | root or last Dockerfile USER | User for all container operations |
| `remoteUser` | string | same as containerUser | User for lifecycle hooks and tool processes |
| `updateRemoteUserUID` | boolean | `true` | Sync UID/GID to local user (Linux only) |
| `userEnvProbe` | enum | `loginInteractiveShell` | Shell type for probing env vars (`none`, `interactiveShell`, `loginShell`, `loginInteractiveShell`) |
| `overrideCommand` | boolean | `true` (image/Dockerfile), `false` (Compose) | Replace default command with a sleep loop |
| `shutdownAction` | enum | `stopContainer` or `stopCompose` | Action on close (`none`, `stopContainer`, `stopCompose`) |
| `init` | boolean | `false` | Use tini init process |
| `privileged` | boolean | `false` | Run in privileged mode |
| `capAdd` | string[] | `[]` | Linux capabilities to add |
| `securityOpt` | string[] | `[]` | Security options |
| `mounts` | (string\|object)[] | - | Additional mounts (Docker `--mount` syntax) |
| `customizations` | object | - | Tool-specific properties (namespaced by tool) |
| `hostRequirements` | object | - | See [Host Requirements](#host-requirements) |

### Image / Dockerfile Properties

| Property | Type | Default | Description |
|---|---|---|---|
| `image` | string | **required** (image scenario) | Container registry image |
| `build.dockerfile` | string | **required** (Dockerfile scenario) | Path to Dockerfile, relative to devcontainer.json |
| `build.context` | string | `"."` | Docker build context directory |
| `build.args` | object | - | Docker build arguments |
| `build.options` | string[] | `[]` | Extra Docker build CLI flags |
| `build.target` | string | - | Multi-stage build target |
| `build.cacheFrom` | string\|string[] | - | Cache source images |
| `workspaceMount` | string | - | Custom mount for source code |
| `workspaceFolder` | string | - | Default path opened inside the container |
| `runArgs` | string[] | `[]` | Extra `docker run` CLI flags |
| `appPort` | int\|string\|array | `[]` | **Legacy**, use `forwardPorts` instead |

### Docker Compose Properties

| Property | Type | Default | Description |
|---|---|---|---|
| `dockerComposeFile` | string\|string[] | **required** | Path(s) to Compose file(s) |
| `service` | string | **required** | Primary service to connect to |
| `runServices` | string[] | all services | Subset of services to start. The primary `service` is always started regardless. |
| `workspaceFolder` | string | `"/"` | Default path opened inside the container |

### Lifecycle Hooks

| Property | Type | Runs | When |
|---|---|---|---|
| `initializeCommand` | string\|string[]\|object | On **host** | Initialization (may run multiple times) |
| `onCreateCommand` | string\|string[]\|object | In container | First creation only |
| `updateContentCommand` | string\|string[]\|object | In container | After content is available (creation only) |
| `postCreateCommand` | string\|string[]\|object | In container | After user setup (creation only, background by default) |
| `postStartCommand` | string\|string[]\|object | In container | Every container start |
| `postAttachCommand` | string\|string[]\|object | In container | Every tool attachment |
| `waitFor` | enum | - | Block until this stage completes. Default: `updateContentCommand`. Values: `initializeCommand`, `onCreateCommand`, `updateContentCommand`, `postCreateCommand`, `postStartCommand` |

Command type details:

- **string**: Executed via the default shell (`/bin/sh`).
- **string[]**: Direct exec, no shell interpretation.
- **object**: Keys are unique names, values are string or string[]. All entries run in **parallel**. Every entry must succeed for the stage to succeed.

### Host Requirements

| Property | Type | Description |
|---|---|---|
| `hostRequirements.cpus` | integer | Minimum CPU count |
| `hostRequirements.memory` | string | Minimum RAM (suffix: `tb`, `gb`, `mb`, `kb`) |
| `hostRequirements.storage` | string | Minimum storage (same suffixes) |
| `hostRequirements.gpu` | boolean\|string\|object | `true` = required, `"optional"` = preferred, or `{"cores": N, "memory": "Xgb"}` |

### Port Attributes

Properties inside `portsAttributes` entries:

| Property | Type | Default | Description |
|---|---|---|---|
| `label` | string | - | Display name |
| `protocol` | enum | - | `http` or `https` |
| `onAutoForward` | enum | `notify` | `notify`, `openBrowser`, `openBrowserOnce`, `openPreview`, `silent`, `ignore` |
| `requireLocalPort` | boolean | `false` | Require the same port number locally |
| `elevateIfNeeded` | boolean | `false` | Auto-elevate permissions for low ports |

---

## Variable Substitution

> [JSON Reference: Variables](https://containers.dev/implementors/json_reference/)

| Variable | Usable In | Description |
|---|---|---|
| `${localEnv:VAR}` | Any property | Host env var. Supports default: `${localEnv:VAR:fallback}` |
| `${containerEnv:VAR}` | `remoteEnv` only | Container env var. Supports default: `${containerEnv:VAR:fallback}` |
| `${localWorkspaceFolder}` | Any property | Full path to the opened local folder |
| `${containerWorkspaceFolder}` | Any property | Full path inside the container |
| `${localWorkspaceFolderBasename}` | Any property | Local folder basename only |
| `${containerWorkspaceFolderBasename}` | Any property | Container folder basename only |
| `${devcontainerId}` | `name`, lifecycle hooks, `mounts`, env vars, user properties, `customizations` | Stable unique ID across rebuilds |

Substitution happens at the time the value is applied (runtime, not build time).
`${containerEnv}` is restricted to `remoteEnv` because container env vars only exist after
the container is running.

---

## Lifecycle

> [Spec: Lifecycle](https://containers.dev/implementors/spec/)

### Creation Flow

```
initializeCommand          (host, may run multiple times)
        |
        v
Image build / pull         (Features applied as Dockerfile layers)
        |
        v
Container creation         (mounts, containerEnv, containerUser applied)
        |                  (remoteUser/remoteEnv NOT applied yet)
        v
onCreateCommand            (in container, with remoteUser/remoteEnv)
        |
        v
updateContentCommand       (in container)
        |
        v
postCreateCommand          (in container, background by default)
        |
        v
[implementation-specific]  (tool-specific init, e.g. extension install)
        |
        v
postStartCommand           (in container)
        |
        v
postAttachCommand          (in container)
```

The `waitFor` property controls at which point the tool reports the environment as ready and
proceeds to implementation-specific steps. Default is `updateContentCommand`.

### Resume Flow

```
Restart containers
        |
        v
[implementation-specific steps]
        |
        v
postStartCommand
        |
        v
postAttachCommand
```

Remote env vars and user configuration apply during resume.

### Hook Execution Details

- Commands within an **object** run in **parallel**. All must succeed.
- Feature-provided lifecycle hooks run **before** user-defined hooks, in Feature installation order.
- `onCreateCommand`, `updateContentCommand`, and `postCreateCommand` run only on first creation.
- `postStartCommand` runs on every start (creation and resume).
- `postAttachCommand` runs on every tool attachment.
- Remote env vars and `userEnvProbe` results are available for all post-creation hooks.

---

## Users and Permissions

> [Spec: Users](https://containers.dev/implementors/spec/)

Two distinct user concepts:

- **Container User** (`containerUser` for image/Dockerfile, `user` in Compose): Runs all
  container operations including the ENTRYPOINT.
- **Remote User** (`remoteUser`): Runs lifecycle hooks and tool processes. Defaults to
  `containerUser` if not set. This separation allows different permissions for container
  operations vs. developer activity.

### updateRemoteUserUID

- Linux only, default `true`.
- When enabled and a remote/container user is specified, the tool updates the image user's
  UID/GID to match the local user before container creation.
- Prevents permission mismatches on bind mounts.
- Implementations may skip this when not using bind mounts or when the container engine
  provides automatic UID translation.

---

## Environment Variables

> [Spec: Environment Variables](https://containers.dev/implementors/spec/)

Two classes:

| Type | Property | Set When | Available |
|---|---|---|---|
| **Container** | `containerEnv` | At container build/create time | Entire container lifecycle |
| **Remote** | `remoteEnv` | Post-ENTRYPOINT by the tool | Lifecycle hooks and tool processes |

Remote env vars support `${containerEnv:VAR}` substitution since the container is already
running when they are applied.

### userEnvProbe

Tools probe the user's environment using the configured shell type and merge the resulting
variables with `remoteEnv` for injected processes. This emulates the behavior developers
expect from their profile/rc files.

---

## Workspace Mount and Folder

> [Spec: Workspace](https://containers.dev/implementors/spec/)

- `workspaceMount` (image/Dockerfile only): Defines the source mount for the workspace.
  The default is a bind mount of the local folder into `/workspaces/<folder-name>`.
- `workspaceFolder`: The default working directory inside the container.
- Both should reference the repository root (where `.git` lives) for proper source control.
- For monorepos, `workspaceFolder` can point to a subfolder while `workspaceMount` targets
  the repo root.

---

## Image Metadata

> [Spec: Image Metadata](https://containers.dev/implementors/spec/)

Configuration can be baked into images via the `devcontainer.metadata` label. The label value
is a JSON string containing either:

- An **array** of metadata objects (one per Feature, plus one for the devcontainer.json config).
- A **single top-level object**.

The array format is preferred because subsequent builds can simply append entries.

### Storable Properties

These properties can appear in image metadata: `forwardPorts`, `portsAttributes`,
`otherPortsAttributes`, `containerEnv`, `remoteEnv`, `remoteUser`, `containerUser`,
`updateRemoteUserUID`, `userEnvProbe`, `overrideCommand`, `shutdownAction`, `init`,
`privileged`, `capAdd`, `securityOpt`, `mounts`, `customizations`, `hostRequirements`,
all lifecycle hooks (`onCreateCommand` through `postAttachCommand`), and `waitFor`.

### Metadata Merge Rules

When merging image metadata with `devcontainer.json`, the local file is considered **last**
(highest priority where order matters).

| Strategy | Properties |
|---|---|
| Boolean OR (true if any is true) | `init`, `privileged` |
| Union without duplicates | `capAdd`, `securityOpt` |
| Union, last wins on conflicts | `forwardPorts` |
| Collected list (append in order) | All lifecycle hooks, `entrypoint` |
| Collected list, last wins on conflicts | `mounts` |
| Last value wins (scalar) | `waitFor`, `containerUser`, `remoteUser`, `userEnvProbe`, `shutdownAction`, `updateRemoteUserUID`, `overrideCommand` |
| Per-variable, last wins | `remoteEnv`, `containerEnv` |
| Per-port, last wins | `portsAttributes` |
| Last value wins | `otherPortsAttributes` |
| Maximum value wins | `hostRequirements` (cpus, memory, storage, gpu) |
| Tool-specific logic | `customizations` |

---

## Features

> [Features Specification](https://containers.dev/implementors/features/) |
> [Features Overview](https://containers.dev/features/)

Features are modular, self-contained units that add tools, runtimes, and capabilities to dev
containers without writing complex Dockerfiles.

### Feature Structure

A Feature is a directory containing:

- `devcontainer-feature.json` (metadata, required)
- `install.sh` (entrypoint script, required)
- Additional supporting files (optional)

### devcontainer-feature.json

#### Required

| Property | Type | Description |
|---|---|---|
| `id` | string | Unique within the repository, must match directory name |
| `version` | string | Semver (e.g., `1.0.0`) |
| `name` | string | Human-friendly display name |

#### Optional Metadata

| Property | Type | Description |
|---|---|---|
| `description` | string | Feature overview |
| `documentationURL` | string | Docs link |
| `licenseURL` | string | License link |
| `keywords` | string[] | Search terms |
| `deprecated` | boolean | Deprecation flag |
| `legacyIds` | string[] | Previous IDs (for renaming) |

#### Configuration

| Property | Type | Description |
|---|---|---|
| `options` | object | User-configurable parameters (see [Feature Options](#feature-options)) |
| `containerEnv` | object | Env vars added as Dockerfile `ENV` before `install.sh` |
| `privileged` | boolean | Requires privileged mode |
| `init` | boolean | Requires tini init |
| `capAdd` | string[] | Linux capabilities |
| `securityOpt` | string[] | Security options |
| `entrypoint` | string | Custom startup script path |
| `mounts` | object | Additional mounts |

#### Dependencies

| Property | Type | Description |
|---|---|---|
| `dependsOn` | object | Hard dependencies (recursive). Feature fails if unresolvable. Values include options and version. |
| `installsAfter` | string[] | Soft ordering (non-recursive). Only affects ordering of already-queued Features. Ignored if the referenced Feature is not being installed. |

#### Lifecycle Hooks

Features can declare their own lifecycle hooks: `onCreateCommand`, `updateContentCommand`,
`postCreateCommand`, `postStartCommand`, `postAttachCommand`. Same types as the main config.

#### Customizations

| Property | Type | Description |
|---|---|---|
| `customizations` | object | Tool-specific config. Arrays merge as union, objects merge values. |

### Feature Options

```json
"options": {
  "optionId": {
    "type": "string|boolean",
    "description": "What this option controls",
    "proposals": ["val1", "val2"],
    "enum": ["strict1", "strict2"],
    "default": "val1"
  }
}
```

- `proposals`: Suggested values, free-form input allowed.
- `enum`: Strict allowed values only.
- `default`: Fallback when the user provides nothing.

Option IDs are converted to environment variables for `install.sh`:

```
replace non-word chars with _  ->  strip leading digits/underscores  ->  UPPERCASE
```

Written to `devcontainer-features.env` and sourced before the script runs.

### Built-in Variables for install.sh

| Variable | Description |
|---|---|
| `_REMOTE_USER` | Configured remote user |
| `_CONTAINER_USER` | Container user |
| `_REMOTE_USER_HOME` | Remote user's home directory |
| `_CONTAINER_USER_HOME` | Container user's home directory |

Features always execute as **root** during image build.

### Referencing Features

In `devcontainer.json`:

```json
"features": {
  "ghcr.io/devcontainers/features/go:1": {},
  "ghcr.io/user/repo/node:18": {"version": "18"},
  "https://example.com/feature.tgz": {},
  "./local-feature": {"optionA": "value"}
}
```

Three source types:
1. **OCI Registry**: `<registry>/<namespace>/<id>[:<semver>]`
2. **HTTPS Tarball**: Direct URL to `.tgz`
3. **Local Directory**: `./path` relative to devcontainer.json

### Installation Order Algorithm

> [Features Spec: Installation Order](https://containers.dev/implementors/features/)

**Step 1 - Build dependency graph:**
- Traverse `dependsOn` recursively and `installsAfter` non-recursively.
- Deduplicate using [Feature Equality](#feature-equality).

**Step 2 - Assign round priority:**
- Default: 0.
- `overrideFeatureInstallOrder` assigns priorities: `array_length - index` (first item = highest priority).

**Step 3 - Round-based sorting:**
1. Identify Features whose dependencies are all satisfied.
2. From those, commit only Features with the maximum `roundPriority`.
3. Return lower-priority Features to the worklist for the next round.
4. Within a round, stable sort lexicographically by: resource name, version tag, options count, option keys/values, canonical name.
5. If a round makes no progress, fail (circular dependency).

### Feature Equality

| Source | Equal When |
|---|---|
| OCI Registry | Manifest digests match AND options are identical |
| HTTPS Tarball | Content hashes match AND options are identical |
| Local Directory | Always unique |

### Dockerfile Layer Generation

- Each Feature's `install.sh` runs as its own Dockerfile layer (for caching).
- `containerEnv` values become `ENV` instructions **before** `install.sh` runs.
- `install.sh` is invoked as: `chmod +x install.sh && ./install.sh`
- Default shell: `/bin/sh`.
- `privileged` and `init`: required if **any** Feature needs them (boolean OR across all).
- `capAdd` and `securityOpt`: union across all Features.

### Feature Lifecycle Hooks

- Feature hooks execute **before** user-defined hooks.
- Hooks run in Feature installation order.
- Object-syntax commands within a single Feature run in parallel.
- All hooks run from the project workspace folder.

---

## Features Distribution (OCI)

> [Features Distribution](https://containers.dev/implementors/features-distribution/)

### Packaging

Distributed as `devcontainer-feature-<id>.tgz` tarballs containing the Feature directory.

### OCI Artifact Format

| Artifact | Media Type |
|---|---|
| Config | `application/vnd.devcontainers` |
| Feature layer | `application/vnd.devcontainers.layer.v1+tar` |
| Collection metadata | `application/vnd.devcontainers.collection.layer.v1+json` |

### Naming

`<registry>/<namespace>/<id>[:version]`

Example: `ghcr.io/devcontainers/features/go:1.2.3`

### Version Tags

Multiple tags are pushed for each release: `1`, `1.2`, `1.2.3`, and `latest`.

### Collection Metadata

An auto-generated `devcontainer-collection.json` aggregates all Feature metadata in a namespace.
Published at `<registry>/<namespace>:latest`.

### Manifest Annotations

Published manifests include a `dev.containers.metadata` annotation containing the escaped JSON
from `devcontainer-feature.json`.

### Authentication

1. Docker config: `$HOME/.docker/config.json`
2. Environment variable: `DEVCONTAINERS_OCI_AUTH` (format: `service|user|token`)

---

## Templates

> [Templates Specification](https://containers.dev/implementors/templates/) |
> [Templates Distribution](https://containers.dev/implementors/templates-distribution/) |
> [Templates Overview](https://containers.dev/templates/)

Templates are pre-configured dev environment blueprints. They are not directly relevant to
crib's runtime behavior but are useful context for understanding the ecosystem.

### Structure

```
template/
  devcontainer-template.json
  .devcontainer.json   (or .devcontainer/devcontainer.json)
  (supporting files)
```

### devcontainer-template.json

Required: `id`, `version`, `name`, `description`.

Optional: `documentationURL`, `licenseURL`, `platforms`, `publisher`, `keywords`, `options`,
`optionalPaths`.

### Option Substitution

Template options use `${templateOption:optionId}` syntax. The tool replaces these placeholders
with user-selected values when applying a template.

### Distribution

Same OCI pattern as Features (tarballs with custom media types, semver tagging).
The namespace for Templates **must be different** from the namespace for Features.

---

## devcontainerId Computation

> [Spec: devcontainerId](https://containers.dev/implementors/spec/)

Computed from container labels:

1. Serialize labels as sorted JSON (keys alphabetically, no whitespace).
2. Compute SHA-256 of the UTF-8 encoded string.
3. Base-32 encode the hash.
4. Left-pad to 52 characters with `0`.

The ID is deterministic across rebuilds and unique per Docker host.

---

## JSON Schema

> [JSON Schema](https://containers.dev/implementors/json_schema/)

- Conforms to **JSON Schema Draft 7**.
- Permits comments (JSONC), disallows trailing commas.
- Base schema: `devContainer.base.schema.json`.
- Main schema: `devContainer.schema.json` (references base + tool-specific schemas).
- Source: [devcontainers/spec on GitHub](https://github.com/devcontainers/spec).

---

## Official Reference Links

| Resource | URL |
|---|---|
| Main Specification | https://containers.dev/implementors/spec/ |
| JSON Reference | https://containers.dev/implementors/json_reference/ |
| JSON Schema | https://containers.dev/implementors/json_schema/ |
| Features Overview | https://containers.dev/features/ |
| Features Spec (Implementors) | https://containers.dev/implementors/features/ |
| Features Distribution | https://containers.dev/implementors/features-distribution/ |
| Templates Overview | https://containers.dev/templates/ |
| Templates Spec (Implementors) | https://containers.dev/implementors/templates/ |
| Templates Distribution | https://containers.dev/implementors/templates-distribution/ |
| Supporting Tools | https://containers.dev/supporting |
| GitHub Repository | https://github.com/devcontainers/spec |
