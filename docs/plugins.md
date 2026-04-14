---
title: Built-in Plugins
description: What crib's built-in plugins do and how to configure them.
---

`crib` ships five built-in plugins that hook into the dev container lifecycle. They inject credentials, SSH config, shell history persistence, dotfiles, and shared package caches into a workspace without extra devcontainer.json boilerplate for the ones that need no configuration.

Plugins run during `crib up` and `crib rebuild`. They are fail-open: if a plugin can't find something it needs (no SSH agent running, no Claude credentials on disk), it skips silently and doesn't block container creation.

---

## Shell history

Persists your bash/zsh history across container recreations. Without this, every rebuild starts with an empty history.

**What it does:**

- Creates `~/.crib/workspaces/{id}/plugins/shell-history/.shell_history` on the host
- Bind-mounts that directory into the container at `~/.crib_history/`
- Sets `HISTFILE=~/.crib_history/.shell_history` in the container

Both bash and zsh read `HISTFILE`, so this works regardless of which shell you use inside the container. The history file is mounted as a directory (not a file directly) so the shell can do atomic renames when saving, which avoids `EBUSY` errors on Docker and Podman.

**No configuration needed.** It always runs.

---

## Package cache

Shares package manager caches across all your workspaces via named Docker volumes. Without this, every rebuild re-downloads all dependencies from scratch.

**Configure in `.cribrc`** (in your project root):

```
cache = npm, pip, go
```

Comma-separated list of providers. Each one creates a `crib-cache-{workspace}-{provider}` named volume mounted at the standard cache directory inside the container. Volumes are per-workspace, so different projects don't share cached data.

### Supported providers

| Provider | Mount target | Notes |
|----------|-------------|-------|
| `npm` | `~/.npm` | |
| `yarn` | `~/.cache/yarn` | |
| `pip` | `~/.cache/pip` | |
| `go` | `~/go/pkg/mod` | Also sets `GOMODCACHE` so it works with any `GOPATH` |
| `cargo` | `~/.cargo` | Also sets `CARGO_HOME` so it works with devcontainer images that use `/usr/local/cargo` |
| `maven` | `~/.m2/repository` | |
| `gradle` | `~/.gradle/caches` | |
| `bundler` | `~/.bundle` | Sets `BUNDLE_PATH` and `BUNDLE_BIN`; adds `~/.bundle/bin` to PATH via `/etc/profile.d/` |
| `apt` | `/var/cache/apt` | System path; disables `docker-clean` so cached `.deb` files persist |
| `downloads` | `~/.cache/crib` | General-purpose cache; sets `CRIB_CACHE` env var for easy access |

Unknown provider names produce a warning at startup and are skipped.

The `downloads` provider is a general-purpose persistent directory for anything that doesn't have its own provider. Use it for large downloads, compiled tools, or any files you want to survive rebuilds:

```bash
curl -L -o "$CRIB_CACHE/some-tool.tar.gz" https://example.com/some-tool.tar.gz
tar -xzf "$CRIB_CACHE/some-tool.tar.gz" -C /usr/local/bin
```

Use `crib cache list` to see which cache volumes exist and how much space they use, and `crib cache clean` to remove them. See [Commands](/crib/reference/commands/#crib-cache) for details.

### Build-time caching

When cache providers are configured, crib also attaches [BuildKit cache mounts](https://docs.docker.com/build/cache/optimize/#use-cache-mounts) to the `RUN` instructions that install DevContainer Features. This speeds up feature installation across rebuilds by reusing cached packages (especially `apt` packages, which most features install).

Build-time caching applies to the feature-generated Dockerfile only, not to user Dockerfiles or compose service builds. The build cache is managed by BuildKit and is separate from the runtime named volumes.

For `apt`, crib also disables the `docker-clean` hook in the generated Dockerfile so that cached `.deb` files are preserved across builds.

:::note[First run]
The first `crib up` after adding a cache provider still downloads everything (the volume is empty). Subsequent rebuilds reuse the cached data.
:::

---

## Coding agents

Shares Claude Code credentials with the container so you can run `claude` without authenticating every time. Two modes are available.

### Host mode (default)

Copies your host's `~/.claude/.credentials.json` into the container on each `crib up`. If the file doesn't exist on the host, the plugin is a no-op.

A minimal `~/.claude.json` with `{"hasCompletedOnboarding":true}` is also injected to skip the onboarding prompt. It won't overwrite an existing file in the container.

This mode is transparent — nothing to configure.

### Workspace mode

For teams using a shared Claude organization account, or when you want to authenticate once inside the container and have those credentials persist across rebuilds.

Configure it in `devcontainer.json`:

```jsonc
{
  "customizations": {
    "crib": {
      "coding-agents": {
        "credentials": "workspace"
      }
    }
  }
}
```

In workspace mode:
- Host credentials are **not** injected
- `~/.crib/workspaces/{id}/plugins/coding-agents/claude-state/` is bind-mounted to `~/.claude/` inside the container
- Credentials you create inside the container (by running `claude` and authenticating) are stored in that directory and survive rebuilds
- The `~/.claude.json` onboarding config is still re-injected via `docker exec` on each rebuild (with `IfNotExists` semantics, so your customizations are preserved)

This is the right choice when:
- Your team shares a Claude organization account that requires SSO or a different login than your personal account
- You want credentials scoped to a specific project workspace
- You don't have Claude Code installed on the host

:::note[First use]
After switching to workspace mode, run `claude` inside the container and authenticate. From that point on, credentials persist automatically.
:::

### Choosing a mode

| Scenario | Mode |
|---|---|
| Personal Claude account, CLI installed on host | Host (default) |
| Team/org account with SSO | Workspace |
| Claude not installed on host | Workspace |
| Credentials should stay per-project | Workspace |

### pi support

In addition to Claude Code, crib also supports [pi](https://github.com/badlogic/pi-mono/tree/main/packages/coding-agent) credentials. Both agents can run side-by-side in the same container and share the same `credentials` mode.

**Enablement is auto-detected.** pi is only active when `~/.pi/agent/auth.json` exists on the host. If the file is missing, crib produces no pi artifacts regardless of mode. Once enabled, pi follows whichever `credentials` mode you pick for Claude:

| Mode | `~/.pi/agent/auth.json` on host? | Behavior |
|---|---|---|
| `host` (default) | yes | `auth.json` is copied into the container on each `crib up`. |
| `workspace` | yes | `~/.crib/workspaces/{id}/plugins/coding-agents/pi-state/` is bind-mounted over `~/.pi/agent/`. The mount shadows the host file; authenticate inside the container and the credentials persist across rebuilds. |
| either | no | Nothing. |

:::note[Enabling pi without installing it on the host]
If you want per-workspace pi credentials but don't have pi installed on the host, `touch` the auth file to signal intent, then use workspace mode:

```sh
mkdir -p ~/.pi/agent && touch ~/.pi/agent/auth.json
```

Then set `credentials: "workspace"` and authenticate inside the container on first use. (Don't use this with host mode — an empty auth file would get copied into the container.)
:::

### Credential cleanup

In both modes, plugin data (including credentials) is stored on the host under `~/.crib/workspaces/{id}/plugins/coding-agents/`. Running `crib remove` deletes the workspace and all plugin data with it.

If you delete the project directory without running `crib remove` first, the workspace state (including any cached credentials) remains on disk. To clean it up manually, delete the workspace directory listed by `crib list`, or remove it directly from `~/.crib/workspaces/`.

---

## Dotfiles

Clones and installs your dotfiles repository inside the container on creation. Useful for shell aliases, editor config, git settings, and other personal customizations.

**Configure in `~/.config/crib/config.toml`** (or `$XDG_CONFIG_HOME/crib/config.toml`):

```toml
[dotfiles]
repository = "https://github.com/user/dotfiles"
targetPath = "~/dotfiles"        # optional, default ~/dotfiles
installCommand = "make install"  # optional, auto-detects if omitted
```

When `repository` is set, the plugin runs for every workspace. Use a `.cribrc` to override this per project (see below).

**Per-project override via `.cribrc`:**

```ini
# Use different dotfiles for this project:
dotfiles.repository = https://github.com/user/work-dotfiles
dotfiles.targetPath = ~/work-dotfiles    # optional
dotfiles.installCommand = make install   # optional

# Or disable dotfiles entirely for this project:
dotfiles = false
```

Per-project settings override the global config. Setting `dotfiles.repository` in `.cribrc` also works when no global config is set, enabling dotfiles for a single project without a global config file.

**What it does:**

1. Clones the repository into `targetPath` inside the container (default `~/dotfiles`)
2. Looks for an install script in the cloned repo, checking in order: `install.sh`, `bootstrap.sh`, `setup.sh`
3. Runs the first script found, or skips if none exist

If `installCommand` is set, it runs that instead of auto-detecting.

Tilde (`~`) in `targetPath` expands to the remote user's home directory inside the container.

The plugin runs during `crib up` (first creation only). Because the effects are baked into the snapshot, dotfiles survive `crib stop` + `crib up` and restarts. A `crib rebuild` re-runs the clone and install.

**No-op when `repository` is not set in either the global config or `.cribrc`.** If you don't configure dotfiles, the plugin does nothing.

:::note[Git required]
The container must have `git` installed for the clone to work. Most devcontainer base images include it. If yours doesn't, add it via a Feature or `postCreateCommand`.
:::

:::caution[SSH repos on rootless Podman with root containers]
If your container runs as `root` and you use rootless Podman, SSH-based repository URLs (e.g. `git@github.com:user/dotfiles`) will fail to authenticate. The SSH agent socket is bind-mounted with the host user's permissions, and rootless Podman's UID remapping prevents the container's `root` from accessing it. Use an HTTPS URL instead, or switch to a non-root `remoteUser`.
:::

---

## SSH

Shares your SSH configuration with the container so that git operations, remote connections, and commit signing work the same way inside the container as they do on your host.

### SSH agent forwarding

If `SSH_AUTH_SOCK` is set on your host and the socket exists, the plugin:
- Bind-mounts the socket into the container at `/tmp/ssh-agent.sock`
- Sets `SSH_AUTH_SOCK=/tmp/ssh-agent.sock` inside the container

This lets `git push`, `ssh`, and other tools use your host's keys without any keys being copied into the container.

Make sure your SSH agent is running on the host before `crib up`:

```bash
eval $(ssh-agent)
ssh-add ~/.ssh/id_ed25519
```

Or add `ssh-add` to your shell startup file so keys are always loaded.

### SSH config

If `~/.ssh/config` exists, it's copied into the container at `~/.ssh/config`. Host aliases, `ProxyJump` rules, and other SSH settings are available inside the container.

### SSH public keys

`*.pub` files from `~/.ssh/` are copied into the container. Private keys are never copied. This is enough for git commit signing (see below), since signing uses the forwarded agent rather than the private key directly.

`authorized_keys` files are skipped.

### Git SSH signing

If your host's global git config has `gpg.format = ssh`, the plugin extracts your signing configuration and generates a minimal `.gitconfig` for the container.

The generated config includes:

| Setting | Source |
|---------|--------|
| `user.name` | `git config --global user.name` |
| `user.email` | `git config --global user.email` |
| `user.signingkey` | `git config --global user.signingkey` (path rewritten to container home) |
| `gpg.format` | `ssh` |
| `gpg.ssh.program` | `git config --global gpg.ssh.program` (if set) |
| `commit.gpgsign` | `git config --global commit.gpgsign` (if set) |
| `tag.gpgsign` | `git config --global tag.gpgsign` (if set) |

If `user.signingkey` is a path under `~/.ssh/` (e.g. `~/.ssh/id_ed25519-sign.pub`), it's rewritten to the equivalent path inside the container. The public key file is copied there by the SSH public keys step above.

**Signing works via the agent.** OpenSSH 8.2+ can sign commits using only the public key file plus the forwarded agent socket — no private key in the container.

**Host git config setup:**

```bash
# Enable SSH signing globally.
git config --global gpg.format ssh
git config --global user.signingkey ~/.ssh/id_ed25519.pub

# Auto-sign all commits and tags.
git config --global commit.gpgsign true
git config --global tag.gpgsign true
```

If you already sign commits on your host, nothing else is needed. The plugin reads your existing config.

:::note[Per-project gitconfig]
The generated `.gitconfig` is injected before lifecycle hooks run. If your `postCreateCommand` sets up a dotfiles repo that writes a `.gitconfig`, that will take precedence. The plugin-generated file is a fallback, not a hard override.
:::

**If `gpg.format` is not `ssh`,** the plugin skips the gitconfig step entirely. Git settings for GPG signing (OpenPGP) are not forwarded.
