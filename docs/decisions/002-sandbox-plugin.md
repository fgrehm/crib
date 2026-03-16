# ADR 002: Agent sandbox plugin

**Status:** Reverted
**Date:** 2026-03-12

## Status

**Reverted.** The initial implementation used `bubblewrap` for filesystem isolation
and `iptables` for network blocking. Both require capabilities not available in
rootless Podman (`NET_ADMIN`, unprivileged user namespaces), making the plugin
unusable in the primary development environment.

[Linux Landlock LSM](https://landlock.io/) is the viable path forward: it works
without root or capabilities (kernel 5.13+), has a Go library
([go-landlock](https://github.com/landlock-lsm/go-landlock)), and supports both
filesystem access control and TCP port restrictions (kernel 6.7+). The tradeoff:
network restriction is port-based only — IP/CIDR blocking (RFC 1918 ranges) is not
supported, so `blockLocalNetwork` as designed cannot be reimplemented with Landlock
alone.

See the roadmap entry for the future direction.

## Context

`crib` sets up devcontainers and injects credentials (SSH agent, Claude Code tokens) via plugins. Users then run coding agents (`claude`, [`pi`](https://pi.dev/), `aider`, Goose) inside those containers. The container provides isolation from the host, but inside the container, agents have unrestricted access to everything: mounted workspace files, forwarded SSH agent, cached credentials, full network.

A compromised or misbehaving agent (via prompt injection, hallucination, or plain bugs) could:

- Delete or corrupt workspace files (`rm -rf /workspaces/project`)
- Read injected credentials (`~/.claude/.credentials.json`, `~/.ssh/config`)
- Use the SSH agent socket to authenticate to remote servers
- Exfiltrate data to arbitrary hosts
- Access cloud metadata endpoints (`169.254.169.254` and others) for instance credentials (see [cloud metadata reference](../reference/cloud-metadata-endpoints.md))
- Reach internal services on the local network (RFC 1918 ranges)

Claude Code has [native sandbox support](https://code.claude.com/docs/en/sandboxing) via [`bubblewrap`](https://github.com/containers/bubblewrap) (Linux) and Seatbelt (macOS). [Agent Safehouse](https://agent-safehouse.dev/) provides macOS-only sandboxing via `sandbox-exec`. Both are agent-specific or platform-specific. `crib` needs a generic solution that works for any agent running inside a Linux container.

## Decision

Add a new `sandbox` plugin (separate from `coding-agents`) that installs `bubblewrap` inside the container and generates a `sandbox` wrapper script. Users launch agents through the wrapper (e.g., `sandbox claude`, `sandbox pi`).

### Why a separate plugin

The sandbox concern is orthogonal to credential injection. The `coding-agents` plugin manages Claude Code credentials. The `ssh` plugin forwards keys and config. The `sandbox` plugin restricts what processes can do with all of that. Keeping it separate means it composes with any combination of other plugins and applies to any agent, not just Claude Code.

### Why `bubblewrap`

- Linux-only, but `crib` runs everything in Linux containers, so that's fine.
- Mature, well-understood. Originally part of [Flatpak](https://flatpak.org/), later extracted as a standalone project. Also used by Claude Code's own [`@anthropic-ai/sandbox-runtime`](https://github.com/anthropic-experimental/sandbox-runtime).
- Lightweight (single binary, no daemon, no runtime overhead).
- Supports filesystem and network namespace isolation.
- Available in Debian/Ubuntu/Fedora package repositories.
- Alternative: [`@anthropic-ai/sandbox-runtime`](https://www.npmjs.com/package/@anthropic-ai/sandbox-runtime) (npm, wraps `bubblewrap` on Linux, Seatbelt on macOS). Could be offered as an option later, but `bubblewrap` directly avoids a Node dependency.

### Why not container-level restrictions only

Docker/Podman flags (`--network=none`, `--cap-drop=ALL`, `--read-only`) affect everything in the container, not just the agent. Users need package managers, LSPs, build tools, and web access for non-agent work. The wrapper approach restricts only the agent's process tree.

## Design

### Configuration

In `devcontainer.json`, under `customizations.crib.sandbox`:

```jsonc
{
  "customizations": {
    "crib": {
      "sandbox": {
        // Filesystem restrictions (paths relative to container user's home).
        // workspaceFolder is always writable, /tmp is always writable.
        "denyRead": [],              // extra paths to deny reads on
        "denyWrite": [],             // extra paths to deny writes on
        "allowWrite": [],            // extra writable paths beyond workspace + /tmp

        // Network restrictions.
        "blockLocalNetwork": true,   // block RFC 1918, link-local, metadata endpoints

      }
    }
  }
}
```

`denyRead` and `denyWrite` use the same path prefix conventions as Claude Code's [sandbox settings](https://code.claude.com/docs/en/settings#sandbox-settings):

| Prefix | Meaning | Example |
|--------|---------|---------|
| `~/` | Relative to container user's home | `~/.ssh` |
| `/` | Absolute path inside the container | `/etc/shadow` |

### Plugin awareness of other plugins

The `sandbox` plugin scans `{workspaceDir}/plugins/*/` at dispatch time to discover what other plugins have staged. This avoids changing the plugin interface or requiring explicit ordering.

Known plugin artifacts and their default sandbox treatment:

| Plugin | Artifacts | Default sandbox rule |
|--------|-----------|----------------------|
| `coding-agents` | `~/.claude/` | allow-write (agent needs to refresh OAuth tokens) |
| `ssh` | `~/.ssh/` | deny-read |
| `ssh` | `/tmp/ssh-agent.sock` (`SSH_AUTH_SOCK`) | no restriction (see below) |
| `shell-history` | `~/.crib_history/` | deny-read (tmpfs) |

**SSH agent socket:** the private key never enters the container. The socket only exposes a signing oracle ("sign this data with key X"). A process cannot extract the key through the socket ([ref](https://smallstep.com/blog/ssh-agent-explained/)). However, it can *use* the key to authenticate (git push, SSH to servers). This is an accepted tradeoff, since agents need git access to be useful. Blocking the socket entirely would break git operations.

**Shell history:** the history file (`~/.crib_history/.shell_history`) may contain sensitive data (e.g., `export TOKEN=...` commands). Denied via `--tmpfs`, which masks the real directory with an empty ephemeral filesystem. The sandboxed process can still write to it (shell history works during the session), but writes are lost when the process exits and existing history is not visible.

### What the plugin does

**Pre-container-run (v1):**

- Adds `RunArgs` for network restrictions if `blockLocalNetwork` is true: `--cap-add=NET_ADMIN` (needed for `iptables` inside the container).
- Inspects other plugins' staged artifacts to build the default deny lists.

**Post-create (via `FileCopy` + exec):**

1. Install `bubblewrap` if not present (`apt-get install -y bubblewrap`).
2. Generate `~/.local/bin/sandbox` wrapper script with the configured policy baked in. The script calls `bwrap` with:
   - `--ro-bind / /` (read-only root by default)
   - `--bind` for `workspaceFolder` and `/tmp` (writable)
   - `--tmpfs` for denied paths (masks the real contents)
   - `--dev /dev`, `--proc /proc`
   - (Network rules are applied separately via `iptables` in the shared namespace, not via bwrap flags)
### Network isolation and `0.0.0.0` binding

`blockLocalNetwork` uses `iptables` OUTPUT chain rules applied at container setup time (not via bwrap). The bwrap wrapper inherits the container's network namespace by default (no `--unshare-net`). This means:

- **Outbound** traffic to RFC 1918 ranges and metadata endpoints is blocked.
- **Inbound** connections and `0.0.0.0` binding are unaffected. Services started by the agent (dev servers, LSPs) can still accept connections normally.
- Full internet access for web searches, API calls, and package downloads is preserved.

If `--unshare-net` were used instead, the sandbox would get an isolated network namespace with only loopback. That would be more secure but would break most agent workflows (API calls to LLM providers, web research, package installs).

**Blocked destinations** (when `blockLocalNetwork` is true):

- RFC 1918: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`
- Link-local: `169.254.0.0/16` (covers most cloud metadata endpoints)
- CGN range: `100.64.0.0/10` (RFC 6598, covers Alibaba Cloud metadata at `100.100.100.200`)
- Cloud-specific outliers: `168.63.129.16` (Azure), `192.0.0.192` (Oracle Cloud at Customer)
- IPv6 link-local: `fe80::/10`
- IPv6 metadata: `fd00:ec2::254` (AWS), `fd20:ce::254` (GCP), `fd00:42::42` (Scaleway)

See [cloud metadata endpoints reference](../reference/cloud-metadata-endpoints.md) for the full list with sources.

**Cloud provider IP ranges (deferred to v2):**

IP-range blocklisting of cloud providers was evaluated for v1 but deferred. The approach has practical weaknesses that make it a poor fit for this use case:

- The blocklist is necessarily incomplete: cloud providers don't publish exhaustive machine-readable lists, some providers (e.g. Azure) change their download URLs unpredictably, and new providers are never covered automatically.
- Embedded CIDRs go stale as providers reallocate ranges; keeping them current requires ongoing CI work.
- Breaking legitimate traffic (package registries, APIs, SaaS tools hosted on major clouds) without a clean allowlisting mechanism creates operational friction that outweighs the security benefit.
- No other agent sandbox tool uses IP-range blocklisting. The industry pattern is default-deny + explicit domain allowlist (e.g. Claude Code's proxy-based sandbox, Trail of Bits' devcontainer).

`blockLocalNetwork` covers the highest-value targets (cloud metadata endpoints, RFC 1918 lateral movement) with a stable, well-defined set of CIDRs. A proper cloud egress story requires a proxy component for domain-level allowlisting and is tracked in v2.

### Wrapper script (sketch)

```bash
#!/usr/bin/env bash
# ~/.local/bin/sandbox - generated by crib sandbox plugin
# Policy: workspaceFolder=/workspaces/project

exec bwrap \
  --ro-bind / / \
  --dev /dev \
  --proc /proc \
  --bind '/workspaces/project' '/workspaces/project' \
  --bind /tmp /tmp \
  --tmpfs '/home/vscode/.crib_history' \
  --tmpfs '/home/vscode/.ssh' \
  --tmpfs '/home/vscode/.claude' \
  -- "$@"
```

## Scope

### v1 (this ADR)

- `bubblewrap`-based filesystem isolation via wrapper script.
- Plugin awareness via workspace state dir scanning.
- `blockLocalNetwork` via `iptables` rules (metadata endpoints, RFC 1918).
- Configuration in `devcontainer.json` customizations.

### v2 (future)

- `blockCloudProviders`: block outbound traffic to cloud provider IP ranges. IP-range blocklisting was evaluated and rejected for v1 (see Network isolation section above). A proper solution needs a proxy component for domain-level allowlisting.
- `gh` CLI restrictions: wrapper script that intercepts mutation subcommands (`gh issue create`, `gh pr create`, `gh pr comment`, `gh pr merge`, `gh pr close`, `gh issue comment`, `gh release create`). Read-only commands (`gh pr view`, `gh issue list`, `gh pr diff`) allowed. `gh api` blocked entirely (covers both REST and GraphQL escape hatches; `gh api graphql -f query='mutation { ... }'` bypasses subcommand checks). Start conservative, relax later.
- Per-agent policy profiles (different restrictions for different agents).
- [`@anthropic-ai/sandbox-runtime`](https://github.com/anthropic-experimental/sandbox-runtime) as alternative backend.
- `blockDomains`: DNS-based domain blocking (not feasible with `iptables` alone, needs a proxy or DNS sinkhole).
- Network domain allowlisting via proxy (v1 only does IP-based blocklisting).

## Alternatives considered

### A. Extend the `coding-agents` plugin

Add sandbox config to the existing `coding-agents` plugin. Rejected because: sandboxing is agent-agnostic. Users want to sandbox `pi`, `aider`, and future agents too. Coupling sandbox to Claude Code credentials doesn't make sense.

### B. Container-level restrictions only

Use `--network=none`, `--cap-drop=ALL`, `--read-only` on the container. Rejected because these affect all processes, not just the agent. Users need unrestricted access for interactive work, package managers, and build tools.

### C. Claude Code native sandbox configuration

Inject Claude Code's `settings.json` with sandbox config so its built-in `bubblewrap` integration handles isolation. Rejected because it only covers Claude Code, and we want multi-agent support.

### D. Agent Safehouse

macOS-only (`sandbox-exec`). Doesn't work inside Linux containers.

## Known limitations

### `bubblewrap` on Ubuntu 24.04+

Ubuntu 23.10+ added AppArmor-based restrictions on unprivileged user namespaces (`kernel.apparmor_restrict_unprivileged_userns=1`). Even though `/proc/sys/kernel/unprivileged_userns_clone` is set to `1`, `bubblewrap` [fails without an explicit AppArmor profile](https://github.com/containers/bubblewrap/issues/632). The `@anthropic-ai/sandbox-runtime` has the [same issue](https://github.com/anthropic-experimental/sandbox-runtime/issues/74).

On Debian and older Ubuntu, unprivileged user namespaces work out of the box. Alpine and hardened images may need `--privileged` or `--cap-add=SYS_ADMIN` on the outer container.

The `sandbox` plugin currently does not probe whether `bubblewrap` works at post-create time. If namespaces are blocked, `bwrap` fails at runtime when the agent is launched (not at container setup time). A future improvement could add a smoke test and warn (not fail) if it doesn't work.

Claude Code's sandbox runtime offers [`enableWeakerNestedSandbox`](https://code.claude.com/docs/en/sandboxing#security-limitations) for Docker environments without privileged namespaces, trading security for compatibility. Worth investigating whether a similar fallback is needed here.

### `iptables` in rootless mode

`iptables` inside a container requires real root privileges. In rootless Docker/Podman, even `CAP_NET_ADMIN` may not be sufficient because the host user lacks real root. `blockLocalNetwork` may silently fail in rootless setups. Individual rule failures are suppressed (remaining rules are still attempted). If the entire network setup script fails, the plugin manager logs a warning and continues (fail-open).

### SSH agent usage (not extraction)

The SSH agent socket cannot leak private keys, but a sandboxed agent can still *use* the agent to authenticate to remote servers. This is by design (agents need git access), but means a compromised agent could push to repos or SSH into machines the user has access to.

## Revisit if

- A standard emerges for agent sandboxing configuration across tools.
- `bubblewrap` proves insufficient (e.g., agents that need nested namespaces).
- Network-level restrictions need to move beyond blocklisting to allowlisting, which would require a proxy component.
