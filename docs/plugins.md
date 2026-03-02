---
title: Built-in Plugins
description: What crib's built-in plugins do and how to configure them.
---

`crib` ships three plugins that run automatically before each container is created. They inject credentials, SSH config, and shell history persistence into every workspace without any devcontainer.json boilerplate.

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

:::note[First use]
After switching to workspace mode, run `claude` inside the container and authenticate. From that point on, credentials persist automatically.
:::

### Credential cleanup

In both modes, plugin data (including credentials) is stored on the host under `~/.crib/workspaces/{id}/plugins/coding-agents/`. Running `crib remove` deletes the workspace and all plugin data with it.

If you delete the project directory without running `crib remove` first, the workspace state (including any cached credentials) remains on disk. To clean it up manually, delete the workspace directory listed by `crib list`, or remove it directly from `~/.crib/workspaces/`.

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
