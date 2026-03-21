---
title: Examples
description: Example devcontainer.json configurations for common project types.
---

<!-- Keep in sync with examples/ directory -->

The [`examples/`](https://github.com/fgrehm/crib/tree/main/examples) directory in the crib repository contains working projects you can try directly:

```bash
git clone https://github.com/fgrehm/crib.git
cd crib/examples/simple
crib up && crib shell
```

Each example below shows the key parts of the configuration. All examples are self-contained and ready to run.

## Simple (base image only)

The minimal setup: a base image, no Dockerfile, no features.

```jsonc
// examples/simple/.devcontainer/devcontainer.json
{
  "name": "simple",
  "image": "docker.io/library/debian:12-slim",
  "overrideCommand": true,
  "remoteUser": "root",
  "postCreateCommand": "echo '==> container ready'"
}
```

`overrideCommand: true` replaces the image's default entrypoint with a long-running sleep, keeping the container alive for `crib shell` and `crib exec`.

Most of these examples use `remoteUser: "root"` for simplicity since the base images don't ship a non-root user. The [devcontainer base images](https://github.com/devcontainers/images) (like the one in the [Rust example](#rust-with-cargo-and-apt-cache)) come with a `vscode` user preconfigured, which is what most projects use in practice.

## Node.js

A Node.js project using the official Node image directly.

```jsonc
// examples/nodejs-project/.devcontainer/devcontainer.json
{
  "name": "nodejs-project",
  "image": "docker.io/library/node:22-bookworm",
  "overrideCommand": true,
  "remoteUser": "root",
  "postCreateCommand": "apt-get update && apt-get install -y git make curl"
}
```

## Python (custom Dockerfile)

When you need to install system packages or project dependencies at build time, use a Dockerfile.

```jsonc
// examples/python-dockerfile/.devcontainer/devcontainer.json
{
  "name": "python-dockerfile",
  "build": {
    "dockerfile": "Dockerfile",
    "context": "."
  },
  "overrideCommand": true,
  "remoteUser": "root"
}
```

```dockerfile
# examples/python-dockerfile/.devcontainer/Dockerfile
FROM docker.io/library/python:3.12-bookworm

RUN apt-get update && apt-get install -y \
    git make curl build-essential \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspaces/python-dockerfile

RUN pip install --no-cache-dir pytest pytest-cov black flake8
```

## Go

```jsonc
// examples/go-project/.devcontainer/devcontainer.json
{
  "name": "go-project",
  "image": "docker.io/library/golang:1.26-bookworm",
  "overrideCommand": true,
  "remoteUser": "root",
  "postCreateCommand": "apt-get update && apt-get install -y git make curl"
}
```

## Ruby (with package cache)

Shows the [package cache plugin](/crib/guides/plugins/#package-cache) caching bundler gems across rebuilds.

```jsonc
// examples/ruby-project/.devcontainer/devcontainer.json
{
  "name": "ruby-project",
  "image": "docker.io/library/ruby:3.3-bookworm",
  "overrideCommand": true,
  "remoteUser": "root",
  "postCreateCommand": "bundle install"
}
```

```ini
# examples/ruby-project/.cribrc
cache = bundler
```

The `bundler` cache provider sets `BUNDLE_PATH` so gems are stored in a named Docker volume that survives rebuilds.

## Rust (with cargo and apt cache)

Uses a [Microsoft devcontainer base image](https://github.com/devcontainers/images/tree/main/src/rust) with a non-root user and multiple cache providers.

```jsonc
// examples/rust-project/.devcontainer/devcontainer.json
{
  "name": "rust-project",
  "image": "mcr.microsoft.com/devcontainers/rust:1",
  "remoteUser": "vscode",
  "postCreateCommand": "sudo apt-get update && sudo apt-get install -y pkg-config libssl-dev && cargo fetch"
}
```

```ini
# examples/rust-project/.cribrc
cache = cargo, apt
```

No `overrideCommand` needed here because the devcontainer base image already has a long-running entrypoint.

## Docker Compose (multi-service)

A Node.js app with a Redis database, managed together by Docker Compose.

```jsonc
// examples/compose-app/.devcontainer/devcontainer.json
{
  "name": "compose-app",
  "dockerComposeFile": "docker-compose.yml",
  "service": "app",
  "runServices": ["app", "db"],
  "workspaceFolder": "/workspaces/${localWorkspaceFolderBasename}",
  "overrideCommand": true,
  "remoteUser": "root",
  "remoteEnv": {
    "PROJECT_NAME": "${containerEnv:PROJECT}",
    "REDIS_URL": "redis://db:6379"
  }
}
```

```yaml
# examples/compose-app/.devcontainer/docker-compose.yml
services:
  app:
    image: docker.io/library/node:20-slim
    environment:
      NODE_ENV: development
      PROJECT: ${localWorkspaceFolderBasename}
  db:
    image: docker.io/library/redis:7-alpine
```

Key points:
- `service` specifies which container crib attaches to for `shell`, `run`, and `exec`.
- `runServices` controls which services start on `crib up`.
- `${localWorkspaceFolderBasename}` is substituted by crib before passing to compose.
- `${containerEnv:PROJECT}` resolves against the running container's environment.

## DevContainer Features (remote)

Install tools from the [devcontainer features registry](https://containers.dev/features) without touching a Dockerfile.

```jsonc
// examples/with-remote-features/.devcontainer/devcontainer.json
{
  "name": "with-remote-features",
  "image": "docker.io/library/ubuntu:24.04",
  "features": {
    "ghcr.io/devcontainers/features/go:1": {},
    "ghcr.io/devcontainers/features/node:1": {},
    "ghcr.io/devcontainers/features/github-cli:1": {}
  },
  "overrideCommand": true,
  "remoteUser": "root"
}
```

Features are OCI artifacts pulled from container registries. Pass options as key-value pairs in the feature object (e.g., `"version": "20"` for a specific Node version).

## DevContainer Features (local)

Define project-specific features as local directories.

```jsonc
// examples/with-local-features/.devcontainer/devcontainer.json
{
  "name": "with-local-features",
  "image": "docker.io/library/ubuntu:24.04",
  "features": {
    "./features/node": {}
  },
  "overrideCommand": true,
  "remoteUser": "root"
}
```

Local features are referenced with `./` paths relative to the `.devcontainer/` directory. Each feature needs a `devcontainer-feature.json` and an `install.sh` script. See [Authoring Features](/crib/guides/authoring-features/) for details.
