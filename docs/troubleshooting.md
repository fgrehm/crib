---
title: Troubleshooting
description: Common issues and solutions when using crib.
---

## `.crib-features/` in your project directory

When DevContainer Features are installed, `crib` creates a `.crib-features/` directory inside your project's build context during image builds. It's cleaned up automatically after the build, but if the process is killed (e.g. SIGKILL, power loss), it may be left behind.

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

**Prevention:** Avoid bind-mounting directories where ownership matters (like `.ssh/`). The SSH plugin copies individual files into the container via `docker exec` rather than bind mounts, which sidesteps this entirely.

## Plugin copy fails with "Read-only file system"

If you see warnings like:

```text
level=WARN msg="plugin copy: exec failed" target=/home/vscode/.ssh/id_ed25519-sign.pub
error="... cannot create /home/vscode/.ssh/id_ed25519-sign.pub: Read-only file system"
```

A compose volume is already mounted at the same path the plugin is trying to write to. This
happens when your compose file manually mounts directories like `~/.ssh` or `~/.claude` into
the container:

```yaml
# compose.yaml — these conflict with the ssh and coding-agents plugins
volumes:
  - "./data/ssh:/home/vscode/.ssh"
  - "./data/claude:/home/vscode/.claude"
```

The built-in plugins now handle SSH config/keys and Claude credentials automatically, so these
compose mounts are redundant. Remove them from your compose file and let the plugins manage
the files instead.

If you were using the compose mount as persistent storage (e.g. authenticating Claude inside the
container), switch to the coding-agents plugin's
[workspace mode](/crib/guides/plugins/#workspace-mode) instead.

## `remoteEnv`: `sh` not found after setting PATH with `${PATH}`

If you set `PATH` in `remoteEnv` using a bare `${PATH}` reference:

```jsonc
{
  "remoteEnv": {
    "PATH": "/home/vscode/.local/bin:${PATH}"
  }
}
```

Older versions of crib passed the literal string `${PATH}` to `docker exec -e`, which overwrote the container's real PATH and made basic commands like `sh` unfindable:

```text
crun: executable file `sh` not found in $PATH: No such file or directory
```

**Fix:** Update to crib v0.5.0+, which resolves bare `${VAR}` references in `remoteEnv` against the container's environment.

Alternatively, use the spec-standard `${containerEnv:PATH}` syntax, which works in all versions:

```jsonc
{
  "remoteEnv": {
    "PATH": "/home/vscode/.local/bin:${containerEnv:PATH}"
  }
}
```

## SSH agent not working with Docker-in-Docker

If `SSH_AUTH_SOCK` is set inside the container but the socket file doesn't exist at that path,
the Docker-in-Docker feature is likely remounting `/tmp` as a fresh tmpfs, hiding the SSH agent
bind mount underneath it.

**Verify:** Check if `/tmp` is a separate tmpfs inside the container:

```bash
crib exec -- findmnt /tmp
```

If it shows `tmpfs` (not the container's root filesystem), DinD has remounted `/tmp`.

**Fix:** Update to crib v0.7.0+, which mounts the SSH agent socket at `/run/ssh-agent.sock`
instead of `/tmp/ssh-agent.sock` to avoid this conflict.

If you're on an older version, you can work around it by adding to your `devcontainer.json`:

```jsonc
{
  "postStartCommand": "ln -sf /proc/1/root/tmp/ssh-agent.sock /tmp/ssh-agent.sock"
}
```

This creates a symlink from the DinD-mounted `/tmp` to the original mount point visible in PID 1's
mount namespace.

## Podman: slow networking or connection drops with `pasta`

Rootless Podman 5.3+ uses [pasta](https://passt.top/passt/) as the default networking backend. It has known issues that can cause intermittent slowness or connection drops:

- **Poor throughput** ([containers/podman#28219](https://github.com/containers/podman/issues/28219)): pasta can be significantly slower than host networking (up to 8x in some workloads).
- **Random connection failures** ([containers/podman#27164](https://github.com/containers/podman/issues/27164)): outbound TCP connections randomly drop under pasta, causing timeouts in long-running processes.

**Option 1: Use host networking** (recommended for dev containers)

Host networking bypasses the userspace network layer entirely. In rootless Podman, `--network=host` is still isolated by the user namespace, so it is not a security downgrade.

```jsonc
// devcontainer.json
{
  "runArgs": ["--network=host"]
}
```

**Option 2: Switch to `slirp4netns`**

The older [slirp4netns](https://github.com/rootless-containers/slirp4netns) backend adds a bit more latency but avoids the pasta bugs above.

Per-container:

```jsonc
// devcontainer.json
{
  "runArgs": ["--network=slirp4netns"]
}
```

Or globally in `~/.config/containers/containers.conf`:

```toml
[network]
default_rootless_network_cmd = "slirp4netns"
```

These are upstream Podman/pasta issues, not crib bugs. Check the linked GitHub issues for status updates.

## `crib exec` can't find tools installed by mise/asdf/nvm/rbenv

If `crib exec -- ruby -v` fails with "not found" but `crib shell` followed by `ruby -v` works,
the tool is installed via a version manager that modifies PATH through shell init files.

`crib exec` runs commands directly via `docker exec` without sourcing shell init files, so
PATH additions from mise, asdf, nvm, rbenv, etc. aren't available.

**Fix:** Use `crib run` instead, which wraps commands in a login shell:

```bash
crib run -- ruby -v
crib run -- bundle install
```

`crib run` detects the container's shell (zsh, bash, or sh) and runs your command through
`$SHELL -lc '...'`, which sources login profiles and sets up PATH correctly.

Use `crib exec` for commands that don't depend on shell init (system binaries, scripts with
absolute paths) or when you need raw `docker exec` behavior.

## Package cache: `bundler` provider and version managers (mise/rbenv)

The `bundler` cache provider sets `BUNDLE_PATH=~/.bundle` so that `bundle install` writes
gems into the cached volume. This overrides the default gem location, which means gems end up in
the volume instead of the version manager's directory (e.g.
`~/.local/share/mise/installs/ruby/3.4.7/lib/ruby/gems/`).

This is generally fine for bundler-managed projects. The plugin also sets `BUNDLE_BIN=~/.bundle/bin`
and installs a `/etc/profile.d/` script that adds it to PATH, so gem executables like `rspec` and
`rubocop` are available in `crib shell` and `crib run` after `bundle install`. `crib exec` doesn't
source profile scripts, so use `crib run` for commands that depend on bundler binstubs.

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
