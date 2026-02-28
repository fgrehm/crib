---
title: Installation
description: How to install crib on Linux with Docker or Podman.
---

## Prerequisites

`crib` requires a container runtime:

- [Docker](https://docs.docker.com/engine/install/) (with Docker Compose v2), or
- [Podman](https://podman.io/docs/installation) (with [podman-compose](https://github.com/containers/podman-compose))

`crib` auto-detects which runtime is available. To override, set `CRIB_RUNTIME=docker` or `CRIB_RUNTIME=podman`.

:::note[🐧 Linux only]
`crib` is Linux-only. macOS and Windows support may be added if there's interest.
:::

## Install with mise

The easiest way to install and manage `crib` versions:

```bash
mise use github:fgrehm/crib
```

See [mise documentation](https://mise.jdx.dev/) for setup instructions.

## Download from GitHub releases

Download the latest binary from [GitHub releases](https://github.com/fgrehm/crib/releases):

```bash
curl -Lo crib.tar.gz https://github.com/fgrehm/crib/releases/latest/download/crib_linux_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz
tar xzf crib.tar.gz crib
install -m 755 crib ~/.local/bin/crib
rm crib.tar.gz
```

Make sure `~/.local/bin` is in your `PATH`. You can also install the binary to `/usr/local/bin` or any other directory in your `PATH`.

## Verify

```bash
crib version
```
