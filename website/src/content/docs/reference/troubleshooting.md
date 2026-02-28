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
