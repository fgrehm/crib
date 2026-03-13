---
title: Agent Sandboxing
description: Restrict what coding agents can do inside your dev container.
---

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
| `/tmp` | read-write | Scratch space for temp files |
| `~/.crib_history/` | deny-read | May contain credentials (`export TOKEN=...`) |
| `~/.ssh/config`, `~/.ssh/*.pub` | deny-read | Injected by the ssh plugin, contains host info |
| `~/.claude/.credentials.json` | deny-read | Injected by the codingagents plugin |

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

When `blockCloudProviders` is enabled, the sandbox additionally blocks outbound traffic to known cloud provider IP ranges (currently AWS, GCP, Oracle Cloud, and Cloudflare) using their published IP range data. This prevents a compromised agent from exfiltrating data to attacker-controlled cloud instances. The IP ranges are embedded in the `crib` binary and updated periodically. Azure is not yet covered (the download URL for their IP ranges changes weekly).

:::note
`blockCloudProviders` is opt-in because many APIs, package registries, and SaaS tools are hosted on major cloud providers. Enabling it may break workflows that depend on cloud-hosted services. Test with your specific setup before enabling for your team.
:::

## Configuration reference

All options go under `customizations.crib.sandbox` in `devcontainer.json`:

```jsonc
{
  "customizations": {
    "crib": {
      "sandbox": {
        // Filesystem restrictions.
        // workspaceFolder and /tmp are always writable.
        "denyRead": [],              // extra paths to block reads on
        "denyWrite": [],             // extra paths to block writes on
        "allowWrite": [],            // extra writable paths beyond workspace + /tmp

        // Network restrictions.
        "blockLocalNetwork": true,   // block RFC 1918 + metadata endpoints
        "blockCloudProviders": false, // block known cloud provider IP ranges

        // Agent aliases.
        "aliases": ["claude", "pi", "aider"]
      }
    }
  }
}
```

### Path syntax

Paths in `denyRead`, `denyWrite`, and `allowWrite` support two prefixes:

| Prefix | Meaning | Example |
|--------|---------|---------|
| `~/` | Relative to container user's home | `~/.ssh` |
| `/` | Absolute path inside the container | `/etc/shadow` |

### Aliases

The `aliases` list creates wrapper scripts in `~/.local/bin/` inside the container. Each wrapper:

1. Prints `[crib sandbox] Running {name} in sandboxed mode`
2. Launches the real binary through the `sandbox` wrapper

The real binary path is resolved at container setup time (skipping `~/.local/bin/` to avoid self-reference). If the real binary isn't found, the alias is skipped with a warning.

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
| `codingagents` | `~/.claude/.credentials.json` | deny-read |
| `ssh` | `~/.ssh/config`, `~/.ssh/*.pub` | deny-read |
| `ssh` | `/tmp/ssh-agent.sock` | allowed (see below) |
| `shellhistory` | `~/.crib_history/` | deny-read |

User-specified `denyRead`/`denyWrite`/`allowWrite` in the config are merged on top of these defaults.

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
        "blockCloudProviders": true,
        "aliases": ["claude", "pi", "aider"],
        "denyRead": ["/tmp/ssh-agent.sock"],
        "denyWrite": ["~/.config", "~/.local"]
      }
    }
  }
}
```

Blocks cloud provider IPs, SSH agent access, and writes to config directories. The agent can only write to the workspace and `/tmp`. Note that `blockCloudProviders` may break workflows that depend on cloud-hosted services.

## Limitations

### Ubuntu 24.04+

Ubuntu 23.10+ added [AppArmor restrictions on unprivileged user namespaces](https://ubuntu.com/blog/ubuntu-23-10-restricted-unprivileged-user-namespaces) that [break `bubblewrap`](https://github.com/containers/bubblewrap/issues/632) even when the kernel sysctl is enabled. If the sandbox can't initialize, `crib` prints a warning but doesn't block container creation. On Debian and older Ubuntu, it works out of the box.

### Rootless Docker/Podman

The `iptables`-based network restrictions require real root privileges inside the container. In rootless setups, `blockLocalNetwork` and `blockCloudProviders` may not work. The plugin detects this and warns.

### SSH agent usage

The SSH agent socket can't leak private keys, but a sandboxed agent can still *use* your keys to authenticate (push to repos, SSH into servers). This is by design, since agents need git access. See the SSH agent socket section above.

### Not a security boundary

The sandbox limits the damage from agent mistakes and prompt injection, but it is not a hard security boundary. Someone with code execution inside the container could find ways around `bubblewrap` (kernel exploits, misconfigured capabilities). It's defense-in-depth, not a replacement for paying attention to what agents do.
