---
title: macOS & Windows
description: Platform-specific quirks, performance tips, and workarounds
---

`crib` compiles and runs on macOS and Windows, but it's developed and tested on Linux. This page covers what to expect and how to get the best experience on other platforms.

:::caution[Untested]
The recommendations on this page have not been personally tested by the author. They're based on how Docker/Podman work on macOS and Windows generally, and should apply to `crib` since it's just a Docker/Podman client. If you try these and find issues, please [open an issue](https://github.com/fgrehm/crib/issues).
:::

## The short version

`crib` talks to Docker (or Podman) through the CLI, it doesn't care what OS you're on. If `docker run hello-world` works, `crib` will work. The catch is **bind mount performance**: on macOS and Windows, Docker runs inside a Linux VM, so every file read/write from your host crosses a virtualization boundary. On Linux this cost is zero. On macOS/Windows it's noticeable.

## macOS

### Install

```bash
# Grab the darwin/arm64 or darwin/amd64 binary from releases
# https://github.com/fgrehm/crib/releases

# Or use mise
mise use github:fgrehm/crib
```

### Docker runtime options

Any of these should work with `crib`:

- **[Docker Desktop](https://www.docker.com/products/docker-desktop/)**: most common, includes VirtioFS support
- **[OrbStack](https://orbstack.dev/)**: lightweight alternative, generally faster mounts
- **[Colima](https://github.com/abiosoft/colima)**: open-source, supports VirtioFS with `--vm-type=vz --mount-type=virtiofs`
- **[Rancher Desktop](https://rancherdesktop.io/)**: another option, uses Lima under the hood
- **[Podman Desktop](https://podman-desktop.io/)**: if you prefer Podman (set `CRIB_RUNTIME=podman`)

### Bind mount performance

This is the main thing you'll notice. Your project source is bind-mounted from macOS into the container, which means every file operation goes through the VM's filesystem layer.

**How much slower?** Roughly [3x slower than native with VirtioFS](https://www.paolomainardi.com/posts/docker-performance-macos-2025/) enabled. Without VirtioFS (older gRPC-FUSE), it can be [5-6x slower](https://www.docker.com/blog/speed-boost-achievement-unlocked-on-docker-desktop-4-6-for-mac/). For small projects you won't notice. For large projects with tens of thousands of files (think: `node_modules`, large Go modules, big monorepos), you will.

**How VS Code sidesteps this:** VS Code's Dev Containers extension runs a server process *inside* the container. File operations happen container-local, avoiding the mount penalty for editor operations. When you use "[Open Folder in Dev Container](https://code.visualstudio.com/docs/devcontainers/containers)" with a volume, the source lives in a Docker volume (native speed) rather than a bind mount. `crib` uses bind mounts by default because it's tool-agnostic, it doesn't assume you're using any particular editor.

### Recommended setup

:::note
These recommendations are based on general Docker-on-macOS best practices, not first-hand testing with `crib`. Your mileage may vary.
:::

1. **Enable VirtioFS** in Docker Desktop (Settings > General > "VirtioFS"). It's the default on recent versions but worth confirming. If using Colima: `colima start --vm-type=vz --mount-type=virtiofs`.

2. **Use named volumes for heavy directories.** The `mounts` field in `devcontainer.json` lets you put things like `node_modules` or `vendor` on a Docker volume while keeping your source on a bind mount:

   ```json
   {
     "image": "node:20",
     "mounts": [
       "source=${localWorkspaceFolderBasename}-node_modules,target=${containerWorkspaceFolder}/node_modules,type=volume"
     ]
   }
   ```

   This is the single biggest performance win. The bind mount handles your source code (which needs to be editable from the host), while heavy dependency trees stay container-local. See the [VS Code performance guide](https://code.visualstudio.com/remote/advancedcontainers/improve-performance) for more on this technique.

3. **Keep your source on the Mac filesystem**, not on a network drive or external volume. APFS + VirtioFS is the fast path.

### SSH agent forwarding

The `ssh` plugin forwards your SSH agent into the container. On macOS, Docker Desktop exposes the host SSH agent socket, so this should work out of the box. If you're using Colima or another runtime, you may need to ensure the SSH agent socket is mounted. Check your runtime's documentation.

### File watching

File watchers (`inotify` inside the container) may not fire reliably through bind mounts on some runtimes. If your dev server doesn't pick up changes, try polling-based watch modes:

- **Webpack/Vite:** `CHOKIDAR_USEPOLLING=true` or `usePolling: true` in config
- **nodemon:** `--legacy-watch`
- **Go (air/reflex):** Check if your tool supports polling mode

OrbStack and Docker Desktop with VirtioFS handle `inotify` events better than older configurations, but polling is the reliable fallback.

## Windows

`crib` does not currently publish Windows binaries (only Linux and macOS). That said, it should work on Windows through WSL 2, since WSL runs a real Linux environment where the Linux binary works natively.

### WSL 2 (recommended)

The expected approach on Windows is running `crib` *inside* WSL 2 with your source code on the WSL filesystem:

1. Install WSL 2 with a Linux distro (Ubuntu, etc.)
2. Install Docker Desktop with the WSL 2 backend enabled
3. Install `crib`'s Linux binary inside WSL
4. Keep your source code in the WSL filesystem (`/home/you/projects/...`), not on `/mnt/c/...`

This matters because files on the WSL 2 filesystem are accessed at near-native speed by Docker. Files on `/mnt/c/` (the Windows filesystem) go through an additional translation layer that's significantly slower.

### Line endings

If your Git config uses `core.autocrlf = true` (the Windows default), files will have CRLF line endings on the host but the Linux container expects LF. This can cause issues with shell scripts and other tools. Options:

- Set `core.autocrlf = input` globally or per-repo
- Add a `.gitattributes` file: `* text=auto eol=lf`

## What about Podman on macOS/Windows?

[Podman Desktop](https://podman-desktop.io/) uses a similar VM-based approach to Docker Desktop. Set `CRIB_RUNTIME=podman` and it should work. The same bind mount performance considerations apply, it's the same fundamental architecture (Linux VM on the host) regardless of which runtime you use.

## Known limitations

These are things that work on Linux but may have rough edges elsewhere:

| Area | Status on macOS/Windows |
|------|------------------------|
| Bind mount performance | Slower (see above) |
| File watching (`inotify`) | May need polling fallback |
| UID/GID mapping | Docker Desktop handles this; Colima/Podman may need config |
| SSH agent forwarding | Works on Docker Desktop; other runtimes vary |
| `userEnvProbe` shell detection | Untested on Windows-native (fine in WSL) |
| Integration tests | Not run on macOS/Windows in CI |

## Reporting issues

If you hit a platform-specific problem, please open an issue with:

- Your OS and version
- Docker/Podman runtime and version (`docker version`, `docker info`)
- If macOS: which runtime (Docker Desktop, OrbStack, Colima) and filesystem driver (VirtioFS, gRPC-FUSE)
- If Windows: WSL 2 or native

We want `crib` to work well everywhere, reports from macOS and Windows users help make that happen.

## Further reading

- [Docker on macOS performance benchmarks (2025)](https://www.paolomainardi.com/posts/docker-performance-macos-2025/) - thorough comparison of Docker Desktop, Colima, OrbStack, and Lima with VirtioFS
- [Docker Desktop VirtioFS announcement](https://www.docker.com/blog/speed-boost-achievement-unlocked-on-docker-desktop-4-6-for-mac/) - the feature that made macOS bind mounts bearable
- [VS Code: Improve disk performance](https://code.visualstudio.com/remote/advancedcontainers/improve-performance) - the named volume trick for `node_modules` and similar
- [Docker Desktop settings](https://docs.docker.com/desktop/settings-and-maintenance/settings/) - where to enable VirtioFS
