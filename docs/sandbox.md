---
title: Agent Sandboxing
description: Restrict what coding agents can do inside your dev container.
---

> [!WARNING]
> The sandbox plugin is experimental and has not been through full QA yet. Behavior
> may change and rough edges are expected. Feedback welcome via
> [GitHub issues](https://github.com/fgrehm/crib/issues).

A coding agent running inside a dev container has full access to everything in that container: your workspace files, forwarded SSH agent, cached credentials, and the network. The `sandbox` plugin locks that down so agents can only touch what they need.

Works with any agent: Claude Code, [`pi`](https://pi.dev/), Aider, Goose, or anything else you run from the terminal.

## Quick start

Add the sandbox config to your `devcontainer.json`:

```jsonc
{
  "customizations": {
    "crib": {
      "sandbox": {
        "blockLocalNetwork": true,
        "aliases": ["claude", "pi"]
      }
    }
  }
}
```

Run `crib up` (or `crib rebuild` if the container already exists), then start your agent:

```bash
# Explicitly sandboxed (runs "sandbox claude" inside the container):
crib run sandbox claude

# Or just use the alias (same thing, automatically wrapped):
crib run claude
```

Inside the container, the alias prints a banner so you know the sandbox is active:

```
[crib sandbox] Running claude in sandboxed mode
```

## What gets restricted

### Filesystem

The sandbox makes the entire filesystem read-only, then selectively opens up writable paths:

| Path | Access | Why |
|------|--------|-----|
| `/` (everything) | read-only | Default for all paths not listed below |
| `workspaceFolder` | read-write | The agent needs to edit project files |
| git worktree dirs | read-write | Auto-detected sibling worktree directories |
| `/tmp` | read-write | Scratch space for temp files |
| `~/.crib_history/` | deny-read | May contain credentials (`export TOKEN=...`) |
| `~/.ssh/` | deny-read | Injected by the ssh plugin, contains host info |
| `~/.claude/` | read-only | Claude Code needs to read its own config to authenticate |

The sandbox automatically discovers what other `crib` plugins have injected and applies appropriate restrictions. You don't need to manually list credential paths.

### Network

When `blockLocalNetwork` is enabled, the sandbox blocks outbound traffic to:

- **RFC 1918 private ranges**: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`
- **Link-local addresses**: `169.254.0.0/16` (covers cloud metadata endpoints for AWS, GCP, Azure, DigitalOcean, and others)
- **CGN range**: `100.64.0.0/10` (RFC 6598, covers Alibaba Cloud metadata at `100.100.100.200`)
- **Cloud-specific outliers**: `168.63.129.16` (Azure Wire Server), `192.0.0.192` (Oracle Cloud at Customer)
- **IPv6 metadata**: `fd00:ec2::254` (AWS), `fd20:ce::254` (GCP), `fd00:42::42` (Scaleway), `fe80::/10` (link-local)

See the [cloud metadata endpoints reference](/crib/reference/cloud-metadata-endpoints/) for the full list.

Everything else is allowed. Web searches, LLM API calls, package installs, and all other internet traffic work normally. Services bound to `0.0.0.0` inside the container (dev servers, LSPs) can still accept incoming connections.

## Configuration reference

All options go under `customizations.crib.sandbox` in `devcontainer.json`:

```jsonc
{
  "customizations": {
    "crib": {
      "sandbox": {
        // Filesystem restrictions.
        // workspaceFolder and /tmp are always writable.
        "denyRead": [],              // extra directory paths to block reads on
        "denyWrite": [],             // extra directory paths to block writes on
        "allowWrite": [],            // extra writable paths beyond workspace + /tmp
        "hideFiles": [],             // individual files to mask (relative to workspace)

        // Network restrictions.
        "blockLocalNetwork": true,   // block RFC 1918 + metadata endpoints

        // Agent aliases.
        "aliases": ["claude", "pi", "aider"]
      }
    }
  }
}
```

### Path syntax

Paths in `denyRead`, `denyWrite`, and `allowWrite` must be **directories** (bubblewrap's `--tmpfs` and `--bind` operate on mount points, not individual files).

| Prefix | Meaning | Example |
|--------|---------|---------|
| `~/` | Relative to container user's home | `~/.ssh` |
| `/` | Absolute path inside the container | `/etc/secrets` |

### Hiding individual files

The `hideFiles` option masks specific files so the agent sees empty content when reading them. Paths are **relative to the workspace folder**:

```jsonc
"hideFiles": [".netrc", "secrets/api-key.txt"]
```

Under the hood, each listed file is replaced with `/dev/null` via `bwrap --ro-bind-try`. If the file doesn't exist, the entry is silently skipped.

**Important tradeoff**: file hiding applies to the agent's entire process tree, including any child processes the agent spawns. If you hide `.env` or `config/master.key`, commands like `rails runner`, `rake`, or `bundle exec` that the agent runs will also be unable to read those files, breaking application boot. Use `hideFiles` only for files that no child process needs at runtime (standalone API key files, `.netrc`, token files, etc.). For files like `.env` that the application reads at startup, use agent-level restrictions (e.g. Claude Code's `permissions.deny` in [settings.json](https://docs.anthropic.com/en/docs/claude-code/settings)) instead.

### Aliases

The `aliases` list creates wrapper scripts in `~/.local/bin/` inside the container. Each wrapper:

1. Prints `[crib sandbox] Running {name} in sandboxed mode`
2. Launches the real binary through the `sandbox` wrapper

The real binary path is resolved at container setup time (skipping `~/.local/bin/` to avoid self-reference). If the real binary isn't found, the alias is silently skipped.

Without aliases, you can always use the `sandbox` command directly:

```bash
sandbox claude
sandbox pi --model gemini-2.5-pro
sandbox aider --model sonnet
```

## How it works

The plugin uses [`bubblewrap`](https://github.com/containers/bubblewrap) (`bwrap`), the same sandboxing tool used by [Flatpak](https://flatpak.org/) and [Claude Code's own sandbox](https://code.claude.com/docs/en/sandboxing). It creates a restricted view of the filesystem using Linux namespaces, where denied paths are replaced with empty `tmpfs` mounts and the rest of the filesystem is mounted read-only.

Network restrictions use `iptables` OUTPUT chain rules applied once at container setup time in the shared network namespace. Because these rules live in the shared namespace, they affect outbound traffic from all processes in the container, not just the sandboxed agent, and remain in effect until the container is restarted or the rules are explicitly removed.

Filesystem and process isolation are scoped to the agent's process tree: only the agent (and any children it spawns) see the restricted view of the filesystem. Other processes in the container (your interactive shell, build tools, package managers) see the full filesystem.

### Plugin awareness

The sandbox plugin automatically scans `~/.crib/workspaces/{id}/plugins/*/` to discover what other plugins have staged. It applies deny rules for sensitive artifacts without manual configuration:

| Plugin | What it injected | Sandbox rule |
|--------|-----------------|--------------|
| `coding-agents` | `~/.claude/` | read-only (agent needs its own config) |
| `ssh` | `~/.ssh/` | deny-read |
| `ssh` | `/tmp/ssh-agent.sock` | allowed (see below) |
| `shell-history` | `~/.crib_history/` | deny-read |

User-specified `denyRead`/`denyWrite`/`allowWrite` in the config are merged on top of these defaults.

### Git worktree detection

If your workflow uses [git worktrees](https://git-scm.com/docs/git-worktree), the sandbox automatically detects them. At container setup time, the plugin runs `git worktree list` inside the container. When worktrees exist outside the workspace folder (the common pattern with sibling directories like `/workspaces/project-worktrees/`), the plugin adds their parent directory as a writable path.

This means a sandboxed agent can read and write to worktree checkouts without any manual configuration. The detection is logged at debug level:

```
sandbox: auto-detected git worktree directory  path=/workspaces/project-worktrees
```

If git is not installed or the workspace is not a git repository, detection is silently skipped. You can always add writable paths manually via `allowWrite` if needed.

### SSH agent socket

The forwarded SSH agent socket is not restricted. The private key never enters the container; the socket only allows [signing operations](https://smallstep.com/blog/ssh-agent-explained/) and never transmits the key itself. A sandboxed agent can *use* the key to authenticate (git push, SSH to servers) but cannot *extract* it.

This is an intentional tradeoff. Agents need git access to be useful. If you want to block the socket entirely, add `/tmp/ssh-agent.sock` to `denyRead`.

## Examples

### Minimal (filesystem only)

```jsonc
{
  "customizations": {
    "crib": {
      "sandbox": {
        "aliases": ["claude"]
      }
    }
  }
}
```

The agent can read the full filesystem but only write to the workspace and `/tmp`. No network restrictions. Credentials from other plugins are automatically denied.

### Recommended for teams

```jsonc
{
  "customizations": {
    "crib": {
      "sandbox": {
        "blockLocalNetwork": true,
        "aliases": ["claude", "pi", "aider"],
        "denyWrite": ["~/.config"]
      }
    }
  }
}
```

Filesystem isolation plus network protection against metadata endpoint access and lateral movement.

### Maximum restriction

```jsonc
{
  "customizations": {
    "crib": {
      "sandbox": {
        "blockLocalNetwork": true,
        "aliases": ["claude", "pi", "aider"],
        "denyRead": ["/tmp/ssh-agent.sock"],
        "denyWrite": ["~/.config", "~/.local"]
      }
    }
  }
}
```

Blocks metadata endpoints and RFC 1918 ranges, SSH agent access, and writes to config directories. The agent can only write to the workspace and `/tmp`.

## Running agents in autonomous mode

The sandbox makes it safer to run coding agents with reduced permission prompts. By containing filesystem access and network reach at the OS level, you get a safety net that the agent cannot bypass, even when you relax the agent's own permission model.

### Claude Code

Claude Code's `--dangerously-skip-permissions` flag (sometimes called "yolo mode") disables all interactive permission prompts, letting the agent run commands and edit files without confirmation. Combining it with the crib sandbox gives you autonomous operation with OS-level guardrails:

```jsonc
// devcontainer.json
{
  "customizations": {
    "crib": {
      "sandbox": {
        "blockLocalNetwork": true,
        "aliases": ["claude"],
        "denyWrite": ["~/.config", "~/.local"]
      }
    }
  }
}
```

Then run Claude Code in autonomous mode:

```bash
crib run claude --dangerously-skip-permissions
```

What the sandbox enforces regardless of Claude's permission settings:

| Concern | What the sandbox does |
|---------|----------------------|
| Filesystem writes | Limited to workspace folder, worktree dirs, and `/tmp` |
| Credential files | `~/.ssh/` and `~/.crib_history/` hidden; `~/.claude/` read-only (agent needs its own config) |
| Network | Local network and cloud metadata endpoints blocked (when `blockLocalNetwork` is on) |
| SSH keys | Agent can use keys for signing (git push) but cannot read private key material |

What the sandbox does **not** cover:

- **Reading project files**: the agent can read everything in the workspace, including `.env`, `config/master.key`, and other secrets checked into (or gitignored within) the project. Use the agent's own `permissions.deny` settings to restrict reads on specific files.
- **Git operations**: the agent can commit, push, and create branches. It has full git access to the repository.
- **External API calls**: outbound internet traffic (except local network) is allowed. The agent can call any public API.
- **Process spawning**: the agent can run any command available in the container, subject to the filesystem restrictions above.

### Other agents

The same pattern works with any agent that supports unattended operation. Run it through the sandbox alias:

```bash
crib run pi --dangerously-skip-permissions
crib run aider --yes  # aider's equivalent flag
```

The sandbox restrictions are agent-agnostic. They apply to whatever process the wrapper launches.

## Limitations

### Aliases and postCreateCommand installs

Alias wrappers are generated during `PostContainerCreate`, which runs before
lifecycle hooks (`postCreateCommand`, `onCreateCommand`, etc.). If your bootstrap
script installs or updates the agent binary in a lifecycle hook (e.g.
`postCreateCommand: bash bin/setup` where `bin/setup` runs the Claude Code
installer), the installer will overwrite the alias wrapper crib just created.

The workaround is to install the agent binary in the Dockerfile or a DevContainer
Feature, so it is baked into the image before `PostContainerCreate` runs. If that
isn't practical, use `sandbox <agent>` directly instead of relying on the alias.
This may be improved in a future release.

### Ubuntu 24.04+

Ubuntu 23.10+ added [AppArmor restrictions on unprivileged user namespaces](https://ubuntu.com/blog/ubuntu-23-10-restricted-unprivileged-user-namespaces) that [break `bubblewrap`](https://github.com/containers/bubblewrap/issues/632) even when the kernel sysctl is enabled. The sandbox plugin installs bubblewrap and generates the wrapper script, but doesn't probe whether bwrap actually works. If namespaces are blocked, bwrap will fail at runtime when the agent is launched (not at container setup time). On Debian and older Ubuntu, it works out of the box.

### Rootless Docker/Podman

The `iptables`-based network restrictions require real root privileges inside the container. In rootless setups, `blockLocalNetwork` may fail to apply. Individual rule failures do not stop the script (remaining rules are still attempted). If the network setup script fails entirely (e.g. `iptables` is unavailable), the plugin manager logs a warning and continues.

### SSH agent usage

The SSH agent socket can't leak private keys, but a sandboxed agent can still *use* your keys to authenticate (push to repos, SSH into servers). This is by design, since agents need git access. See the SSH agent socket section above.

### Not a security boundary

The sandbox limits the damage from agent mistakes and prompt injection, but it is not a hard security boundary. Someone with code execution inside the container could find ways around `bubblewrap` (kernel exploits, misconfigured capabilities). It's defense-in-depth, not a replacement for paying attention to what agents do.
