---
title: Comparison with alternatives
description: How crib compares to devcontainers/cli, DevPod, and VS Code Dev Containers
---

:::caution[Living document]
This page reflects the state of things as of March 2026. The tools listed here are actively developed (except DevPod), so details may shift over time.
:::

`crib` isn't the only way to use devcontainers outside VS Code. Here's how the alternatives compare for terminal-first developers.

## The landscape

There are three main ways to use `devcontainer.json` without VS Code:

- **`crib`**: CLI-only, no agents, plugin system for DX niceties
- **`devcontainers/cli`**: the official reference implementation, powers VS Code and Codespaces behind the scenes
- **DevPod**: client-only tool with SSH agent injection, provider system for running on any backend (local, cloud, Kubernetes). [Effectively abandoned](https://github.com/loft-sh/devpod/issues/1915) since April 2025; a [community fork](https://github.com/skevetter/devpod) is continuing development.

## Quick comparison

| | `crib` | `devcontainers/cli` | DevPod |
|---|---|---|---|
| **Language** | Go | TypeScript (Node.js) | Go |
| **Binary** | Native | Bundled Node.js | Native |
| **Workspace from CWD** | ✅ Day one | ✅ [v0.82.0](https://github.com/devcontainers/cli/issues/29) | ❌ Named workspaces |
| **`shell` command** | ✅ (detects `zsh`/`bash`/`sh`) | ❌ (`exec` only) | ✅ (via SSH) |
| **[Smart restart](/crib/guides/smart-restart/)** | ✅ (change detection) | ❌ | ❌ |
| **Plugin system** | ✅ (`ssh`, `shell-history`, `coding-agents`) | ❌ | ❌ (providers, not plugins) |
| **Stop (keep container)** | ❌ (`stop` = `down`) | ✅ `stop` | ✅ `stop` |
| **Stop + remove** | ✅ `down` / `stop` | ✅ `down` | ✅ `delete` |
| **`build --push`** | ❌ | ✅ | ❌ |
| **`read-configuration`** | ❌ | ✅ (JSON output) | ❌ |
| **Feature/template testing** | ❌ | ✅ | ❌ |
| **Dotfiles support** | _Can be implemented with plugins_ | ✅ (`--dotfiles-*`) | ✅ |
| **macOS / Windows** | [Works, not primary target](/crib/guides/macos-windows/) | ✅ | ✅ |
| **Podman (rootless)** | ✅ First-class | Partial | Partial |
| **SSH into container** | [Being considered](/crib/contributing/roadmap/) | ❌ | ✅ (agent injection) |
| **Remote/cloud backends** | ❌ Local only | ❌ Local only | ✅ (providers) |
| **IDE integration** | ❌ By design (_could be a plugin_) | ✅ (VS Code, Codespaces) | ✅ (VS Code, JetBrains) |
| **Status** | Active (v0.4.0, Mar 2026) | Active | Abandoned (Apr 2025) |

## When to use what

**Use `crib` if** you want a terminal-first workflow, care about Podman support, and want plugins that handle SSH forwarding, shell history, and AI coding tool credentials without touching your `devcontainer.json`.

**Use `devcontainers/cli` if** you need CI prebuilds (`build --push`), scripting integration (`read-configuration`), or are authoring features/templates. It's also the safest choice for maximum spec compliance since it *is* the reference implementation.

**Use DevPod if** you need remote backends (cloud VMs, Kubernetes) and your team already has it working. The SSH-into-container approach gives native filesystem performance on macOS. Note that the original project has had no updates since April 2025, though a [community fork](https://github.com/skevetter/devpod) is carrying it forward.

## Architecture differences

### How source code gets into the container

This is the fundamental difference that affects everything else:

**`crib` and `devcontainers/cli`** bind-mount the source from the host into the container. Your editor runs on the host and edits files directly. Simple, but on macOS/Windows this means file operations cross the VM boundary (see [macOS & Windows](/crib/guides/macos-windows/)).

**VS Code Dev Containers** runs a VS Code Server *inside* the container. The UI is on the host, but file I/O happens container-local. When you use "[Open Folder in Container](https://code.visualstudio.com/docs/devcontainers/containers)" with a volume, source lives in a Docker volume at native speed.

**DevPod** injects an agent binary and SSH server into the container. Your editor (`nvim`, VS Code, JetBrains) connects over SSH. Source can live in a volume (native speed) or be bind-mounted. Editors that connect over SSH edit on the container's filesystem, not through a bind mount, so there's no performance penalty.

### What happens inside the container

| | `crib` | `devcontainers/cli` | DevPod |
|---|---|---|---|
| Agent injected | ❌ | ❌ | ✅ (Go binary) |
| SSH server | ❌ | ❌ | ✅ (started by agent) |
| Extra processes | None | None | Agent daemon, SSH |
| Setup method | `docker exec` | `docker exec` | Agent via SSH/gRPC |

`crib` aims for nothing inside the container you didn't ask for (though bundled plugins are enabled by default and can inject mounts, env vars, and files). DevPod's model is "full remote development environment." Neither is wrong, they serve different use cases.

## Plugin system vs Features vs Providers

These three extensibility models solve different problems:

**DevContainer Features** (all tools support these) are OCI-distributed install scripts that run at image build time. They add tools to the image (`node`, `go`, Docker-in-Docker). They can't do anything at container creation or runtime.

**`crib` plugins** run at container creation time. They inject mounts, environment variables, and files into the container. The bundled plugins handle SSH agent forwarding, shell history persistence, and Claude Code credentials, things that need to happen at runtime, not build time.

**DevPod providers** are a completely different concept. They control *where* the container runs (local Docker, AWS, Kubernetes, etc.). `crib` is local-only by design, so providers aren't relevant.

The gap that `crib` plugins fill: with `devcontainers/cli`, if you want SSH forwarding or persistent history, you write it into your `devcontainer.json` (mounts, env vars, lifecycle hooks). With `crib`, plugins handle it automatically for every workspace. Less boilerplate, works everywhere without per-project config.

## What `crib` doesn't have (and whether it matters)

**`build --push` for CI prebuilds.** If you're prebaking images in CI, `devcontainers/cli` is the right tool. `crib` focuses on the local development workflow. You could use `devcontainers/cli` in CI and `crib` locally, they read the same `devcontainer.json`.

**`read-configuration` for scripting.** Useful for tooling that needs to parse devcontainer config programmatically. `crib`'s `--json` flag is planned but not shipped yet.

**Feature/template testing tools.** You can [test Features locally with `crib`](/crib/guides/authoring-features/#testing-locally-with-crib), but for automated test suites and template scaffolding, use `devcontainers/cli`'s `features test` and `templates apply`.

**Stopping without removing the container.** `crib`'s `down` (and its `stop` alias) always removes the container. This is a deliberate choice, lifecycle hook markers are cleared so the next `up` is clean. If you need to pause a container without removing it, use `docker stop` directly.

## Links

- [`devcontainers/cli` source](https://github.com/devcontainers/cli)
- [devcontainer spec](https://containers.dev/)
- [DevPod source](https://github.com/loft-sh/devpod) (abandoned, [discussion](https://github.com/loft-sh/devpod/issues/1963))
- [DevPod community fork](https://github.com/skevetter/devpod)
- [VS Code Dev Containers docs](https://code.visualstudio.com/docs/devcontainers/containers)
- [Coder Dev Containers integration](https://coder.com/docs/admin/templates/extending-templates/devcontainers) (uses `devcontainers/cli` under the hood)
