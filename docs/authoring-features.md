---
title: Authoring DevContainer Features
description: How to create, test, and publish reusable DevContainer Features.
---

[DevContainer Features](https://containers.dev/implementors/features/) are modular units that add tools, runtimes, or configuration to any dev container. Instead of maintaining complex Dockerfiles, you package installation logic into a Feature and reference it from `devcontainer.json`.

This guide walks through creating a Feature from scratch, testing it locally with crib, and publishing it to an OCI registry for others to use. For the full specification details, see the [DevContainer Spec Reference](/crib/contributing/devcontainers-spec/#features).

## Feature structure

A Feature is a directory with two required files:

```
my-feature/
  devcontainer-feature.json   # metadata, options, dependencies
  install.sh                  # runs as root during image build
```

You can include additional files (helper scripts, config templates) in the same directory. They'll be available to `install.sh` at build time.

## Writing `devcontainer-feature.json`

This file describes your Feature's metadata and configurable options:

```json
{
  "id": "my-feature",
  "version": "1.0.0",
  "name": "My Feature",
  "description": "Installs my-tool with a configurable version.",
  "options": {
    "version": {
      "type": "string",
      "default": "latest",
      "description": "Version of my-tool to install."
    },
    "enableExtras": {
      "type": "boolean",
      "default": false,
      "description": "Install optional extras."
    }
  },
  "installsAfter": [
    "ghcr.io/devcontainers/features/common-utils"
  ]
}
```

**Required fields:**

| Field | Description |
|-------|-------------|
| `id` | Unique identifier. Must match the directory name. |
| `version` | Semver version string (e.g. `1.0.0`). |
| `name` | Human-readable display name. |

**Common optional fields:**

| Field | Description |
|-------|-------------|
| `options` | User-configurable parameters (string or boolean). |
| `containerEnv` | Environment variables set in the container. |
| `dependsOn` | Hard dependencies on other Features (must be present). |
| `installsAfter` | Soft ordering hints (best-effort, no error if missing). |
| `mounts` | Additional container mounts. |
| `capAdd` | Linux capabilities the Feature needs at runtime. |
| `postCreateCommand` | Hook that runs after container creation. |

## Writing `install.sh`

The install script runs as **root** during `docker build`. User-specified options are passed as environment variables with uppercase names (hyphens become underscores):

```bash
#!/bin/bash
set -e

echo "Installing my-tool version: ${VERSION}"

# Options from devcontainer-feature.json are env vars:
#   "version"      -> $VERSION
#   "enableExtras" -> $ENABLEEXTRAS

# Built-in variables are always available:
#   $_REMOTE_USER       - the user who will use the container
#   $_REMOTE_USER_HOME  - that user's home directory
#   $_CONTAINER_USER    - same as remote user (or overridden)
#   $_CONTAINER_USER_HOME

if [ "$VERSION" = "latest" ]; then
    apt-get update && apt-get install -y my-tool
else
    apt-get update && apt-get install -y my-tool="$VERSION"
fi

if [ "$ENABLEEXTRAS" = "true" ]; then
    apt-get install -y my-tool-extras
fi

# Install user-specific config as the remote user
su "$_REMOTE_USER" -c 'my-tool init --user'
```

**Tips for `install.sh`:**

- Always `set -e` to fail on errors.
- Use `$_REMOTE_USER` for user-specific setup, not a hardcoded username.
- Clean up apt caches (`rm -rf /var/lib/apt/lists/*`) to keep images small.
- Stick to `bash` for Debian/Ubuntu base images, `sh` for Alpine.
- Make installation idempotent when possible (re-running shouldn't break things).

## Testing locally with crib

During development, reference your Feature as a local path. No registry needed.

### 1. Set up a test project

Create a minimal project to test your Feature:

```
test-project/
  .devcontainer/
    devcontainer.json
    my-feature/
      devcontainer-feature.json
      install.sh
```

With `devcontainer.json`:

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "features": {
    "./my-feature": {
      "version": "3.12",
      "enableExtras": true
    }
  }
}
```

The `./my-feature` path is resolved relative to the directory containing `devcontainer.json`.

### 2. Build and start

```bash
cd test-project
crib up
```

crib resolves the local Feature, generates the install layer, and builds the image. Watch the build output for errors in your `install.sh`.

### 3. Verify

Use `crib exec` to check that your Feature installed correctly:

```bash
crib exec -- my-tool --version
crib exec -- which my-tool
crib exec -- sh -c 'echo $MY_ENV_VAR'
```

### 4. Iterate

After making changes to the Feature files, rebuild:

```bash
crib rebuild
```

This forces a fresh image build with your updated Feature.

### Test with multiple base images

Features should work across common base images. Test against a few:

```jsonc
// Debian/Ubuntu
{ "image": "mcr.microsoft.com/devcontainers/base:ubuntu" }

// Alpine
{ "image": "mcr.microsoft.com/devcontainers/base:alpine" }

// Plain Ubuntu
{ "image": "ubuntu:24.04" }
```

Switch the `image` field and run `crib rebuild` for each.

### Test with different options

Try various option combinations, including defaults (omit the option) and edge cases:

```jsonc
// Default options
"features": { "./my-feature": {} }

// Specific version
"features": { "./my-feature": { "version": "3.11" } }

// All options enabled
"features": { "./my-feature": { "version": "3.12", "enableExtras": true } }
```

## Publishing to an OCI registry

Once your Feature works locally, publish it to a registry so others can use it. The recommended approach uses the [devcontainers/feature-starter](https://github.com/devcontainers/feature-starter) template and GitHub Actions.

### Option A: GitHub Actions (recommended)

1. Create a repo from the [feature-starter template](https://github.com/devcontainers/feature-starter).

2. Place your Feature in `src/my-feature/`:
   ```
   src/
     my-feature/
       devcontainer-feature.json
       install.sh
   ```

3. Push and create a release. The included GitHub Action builds and publishes to GHCR automatically.

4. Your Feature is available at:
   ```
   ghcr.io/<your-username>/<repo>/my-feature:1
   ```

### Option B: Official CLI

If you prefer publishing manually:

```bash
npm install -g @devcontainers/cli

# Package and publish to GHCR
devcontainer features publish ./src/my-feature \
  --registry ghcr.io \
  --namespace <your-username>/features
```

See the [official publishing docs](https://containers.dev/implementors/features-distribution/) for details on registry authentication and versioning.

## Using published Features

After publishing, reference your Feature by its OCI address:

```jsonc
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "features": {
    "ghcr.io/your-username/features/my-feature:1": {
      "version": "3.12"
    }
  }
}
```

crib pulls the Feature from the registry, caches it locally, and installs it during the image build. The `:1` tag uses semver matching, so `1` resolves to the latest `1.x.x` release.

To pin to an exact version:

```jsonc
"ghcr.io/your-username/features/my-feature:1.2.3": {}
```

Or pin by digest for full reproducibility:

```jsonc
"ghcr.io/your-username/features/my-feature@sha256:abc123...": {}
```
