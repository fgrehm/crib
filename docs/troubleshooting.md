---
title: Troubleshooting
description: Common issues and solutions when using crib.
---

## `.crib-features/` in your project directory

When devcontainer features are installed, `crib` creates a `.crib-features/` directory inside your project's build context during image builds. It's cleaned up automatically after the build, but if the process is killed (e.g. SIGKILL, power loss), it may be left behind.

Add it to your global gitignore so it never gets committed in any project:

```bash
echo '.crib-features/' >> ~/.config/git/ignore
```

Or if you use `~/.gitignore` as your global ignore file:

```bash
echo '.crib-features/' >> ~/.gitignore
```

Make sure git knows where your global ignore file is:

```bash
# only needed if you haven't set this before
git config --global core.excludesFile '~/.config/git/ignore'
```

Note: `~/.config/git/ignore` is git's default location (since git 1.7.12), so `core.excludesFile` only needs to be set if you use a different path.

## Podman: short-name image resolution

Podman requires fully qualified image names by default. If you see errors like:

```text
Error: short-name "postgres:16-alpine" did not resolve to an alias and no
unqualified-search registries are defined in "/etc/containers/registries.conf"
```

Add Docker Hub as an unqualified search registry:

```ini
# /etc/containers/registries.conf (or a drop-in under /etc/containers/registries.conf.d/)
unqualified-search-registries = ["docker.io"]
```

This lets Podman resolve short names like `postgres:16-alpine` to `docker.io/library/postgres:16-alpine`, matching Docker's default behavior.

## Podman: "Executing external compose provider" warning

When using `podman compose` (which delegates to `podman-compose`), you'll see this on every invocation:

```text
>>>> Executing external compose provider "/usr/bin/podman-compose". Please see podman-compose(1) for how to disable this message. <<<<
```

Silence it by adding to `~/.config/containers/containers.conf`:

```ini
[engine]
compose_warning_logs = false
```

Note: the `PODMAN_COMPOSE_WARNING_LOGS=false` env var is documented but [does not work](https://github.com/containers/podman/issues/23441).

## Podman: missing `pasta` and `aardvark-dns`

If you see errors about `pasta` not found or `aardvark-dns binary not found`, install the networking packages:

```bash
# Debian/Ubuntu
sudo apt install passt aardvark-dns
```

`pasta` provides rootless network namespace setup and `aardvark-dns` enables container DNS resolution. Without them, rootless Podman containers can't start or resolve hostnames.

## `localhost:port` not reachable even though the port is published

If a container port is published but `http://localhost:PORT` fails (connection refused or reset),
check how `localhost` resolves on your machine:

```bash
getent hosts localhost
```

If it resolves to `::1` (IPv6) rather than `127.0.0.1` (IPv4), the port listener and the
address don't match. Podman publishes ports to `0.0.0.0` (IPv4 only) by default in rootless
mode, so `localhost` → `::1` misses it.

**Fix:** Use `127.0.0.1` instead of `localhost`:

```
http://127.0.0.1:PORT
```

Or, if you need `localhost` to work, add an explicit IPv4 entry to `/etc/hosts`:

```
127.0.0.1  localhost
```

(Most systems have this but some distros only keep the IPv6 entry.)

## Rootless Podman and bind mount permissions

In rootless Podman, the host UID is remapped to UID 0 inside the container's user namespace. This means bind-mounted workspace files appear as `root:root` inside the container, so a non-root `remoteUser` (like `vscode`) can't write to them.

`crib` automatically adds `--userns=keep-id` when running rootless Podman. This maps your host UID to the same UID inside the container, so workspace files have correct ownership without any manual configuration.

If you need to override this behavior, set a different `--userns` value in your `devcontainer.json`:

```jsonc
// devcontainer.json
{
  "runArgs": ["--userns=host"]
}
```

When an explicit `--userns` is present in `runArgs`, `crib` won't inject `--userns=keep-id`.

For Docker Compose workspaces, `crib` injects `userns_mode: "keep-id"` in the compose override. Since podman-compose 1.0+ creates pods by default and `--userns` is incompatible with `--pod`, `crib` also disables pod creation via `x-podman: { in_pod: false }` in the override.

## Workspace files owned by a high UID (100000+) after switching to a non-root user

If a lifecycle hook (e.g. `postCreateCommand: npm install`) fails with permission denied, and
the workspace contains files owned by a UID like `100000`, it means an earlier container run
created those files as root inside a rootless Podman container.

**Why this happens:** In rootless Podman with `--userns=keep-id`, container root (UID 0) maps to
a subordinate UID on the host (typically `100000`). Any files created inside the container as
root — such as from a first run without `remoteUser` set, or before adding `remoteUser` to
`devcontainer.json` — are owned by that subordinate UID on the host filesystem. A subsequent run
with `remoteUser` set to a non-root user (like `node` or `vscode`) can't write to those files.

**Check for affected files:**

```bash
ls -lan /path/to/project/node_modules | head -5
```

If you see a high UID (100000+) instead of your own UID, those files are from a previous root
container run.

**Fix:** Delete the affected directories from the host and then rebuild:

```bash
rm -rf node_modules  # or whatever directory was created by the hook
crib rebuild
```

If the directory is large and `rm -rf` is slow, you can use `podman unshare` to remove it
faster from inside the same user namespace:

```bash
podman unshare rm -rf node_modules
```

## Bind mount changed permissions on host files

If you ran `chown` or `chmod` inside a container on a bind-mounted directory and your host files now have wrong ownership, here's how to recover.

**Check the damage:**

```bash
ls -la ~/.ssh/
```

If you see an unexpected UID/GID (e.g., `100999` or `root`) instead of your username, the container wrote through to the host.

**Rootless Podman/Docker:** Use `podman unshare` (or `docker` equivalent) to run the fix inside the same user namespace that caused the problem:

```bash
podman unshare chown -R 0:0 ~/.ssh
```

This maps container root (UID 0) back to your host user through the subordinate UID range, restoring your ownership. Then fix permissions:

```bash
chmod 700 ~/.ssh
chmod 600 ~/.ssh/*
chmod 644 ~/.ssh/*.pub ~/.ssh/allowed_signers 2>/dev/null
```

**Rootful Docker:** Your host user can't chown these back without root:

```bash
sudo chown -R $(id -u):$(id -g) ~/.ssh
chmod 700 ~/.ssh
chmod 600 ~/.ssh/*
chmod 644 ~/.ssh/*.pub ~/.ssh/allowed_signers 2>/dev/null
```

**Prevention:** Avoid mounting into directories where ownership matters (like `.ssh/`). Mount individual files to paths outside sensitive directories instead, as shown in the [commit signing](/crib/guides/git-integration/#commit-signing-with-ssh-keys) section.

## Go: "permission denied" writing to module cache

When using a Go base image (e.g. `golang:1.26`) with a non-root `remoteUser`, you may see:

```text
go: writing stat cache: mkdir /go/pkg/mod/cache/download/...: permission denied
```

The `/go/pkg/mod/cache/` directory is owned by root in official Go images. When running as a non-root user, Go can't write to it.

**Fix in your Dockerfile:**

```dockerfile
RUN chmod -R a+w /go/pkg
```

**Or set a user-writable cache location** in `devcontainer.json`:

```jsonc
{
  "remoteEnv": {
    "GOMODCACHE": "/home/vscode/.cache/go/pkg/mod"
  }
}
```

This is not a crib issue. It affects any tool that runs Go containers with non-root users.
