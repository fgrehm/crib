# ADR 002: Agent sandbox plugin

**Status:** Draft
**Date:** 2026-03-12

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

Add a new `sandbox` plugin (separate from `codingagents`) that installs `bubblewrap` inside the container and generates a `sandbox` wrapper script. Users launch agents through the wrapper (e.g., `sandbox claude`, `sandbox pi`) or configure aliases so agent commands are transparently wrapped.

### Why a separate plugin

The sandbox concern is orthogonal to credential injection. The `codingagents` plugin manages Claude Code credentials. The `ssh` plugin forwards keys and config. The `sandbox` plugin restricts what processes can do with all of that. Keeping it separate means it composes with any combination of other plugins and applies to any agent, not just Claude Code.

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
        "blockCloudProviders": false, // block outbound to known cloud provider IP ranges

        // Agent command aliases (optional).
        // Creates wrapper scripts in ~/.local/bin/ that print a "sandboxed"
        // banner and exec through the sandbox wrapper.
        "aliases": ["claude", "pi", "aider"]
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
| `codingagents` | `~/.claude/.credentials.json` | deny-read |
| `ssh` | `~/.ssh/config`, `~/.ssh/*.pub` | deny-read |
| `ssh` | `/tmp/ssh-agent.sock` (`SSH_AUTH_SOCK`) | no restriction (see below) |
| `shellhistory` | `~/.crib_history/` | deny-read, allow-write |

**SSH agent socket:** the private key never enters the container. The socket only exposes a signing oracle ("sign this data with key X"). A process cannot extract the key through the socket ([ref](https://smallstep.com/blog/ssh-agent-explained/)). However, it can *use* the key to authenticate (git push, SSH to servers). This is an accepted tradeoff, since agents need git access to be useful. Blocking the socket entirely would break git operations.

**Shell history:** the history file (`~/.crib_history/.shell_history`) may contain sensitive data (e.g., `export TOKEN=...` commands). It should be deny-read by default so sandboxed agents cannot search it for credentials. Write access is allowed so history continues to work when running commands through the sandbox.

### What the plugin does

**Pre-container-run (v1):**

- Adds `RunArgs` for network restrictions if `blockLocalNetwork` is true: `--cap-add=NET_ADMIN --cap-add=NET_RAW` (both needed for `iptables` inside the container).
- Inspects other plugins' staged artifacts to build the default deny lists.

**Post-create (via `FileCopy` + exec):**

1. Install `bubblewrap` if not present (`apt-get install -y bubblewrap`).
2. Generate `~/.local/bin/sandbox` wrapper script with the configured policy baked in. The script calls `bwrap` with:
   - `--ro-bind / /` (read-only root by default)
   - `--bind` for `workspaceFolder` and `/tmp` (writable)
   - `--tmpfs` for denied paths (masks the real contents)
   - `--dev /dev`, `--proc /proc`
   - Network namespace with `iptables` rules if `blockLocalNetwork` is true
3. For each entry in `aliases`, generate `~/.local/bin/{alias}` that:
   - Prints a note: `[crib sandbox] Running {alias} in sandboxed mode`
   - Execs `sandbox {real-binary-path} "$@"`
   - Resolves the real binary path at generation time (skipping `~/.local/bin` to avoid self-reference).

### Network isolation and `0.0.0.0` binding

`blockLocalNetwork` uses `--share-net` (shared network namespace) with `iptables` OUTPUT chain rules, not `--unshare-net` (isolated network namespace). This means:

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

**Cloud provider IP ranges** (when `blockCloudProviders` is true):

Blocks outbound traffic to known cloud provider IP ranges using their published machine-readable IP range lists. Currently covers AWS, GCP, Azure, Oracle Cloud, and Cloudflare. This prevents a compromised agent from reaching arbitrary cloud infrastructure (e.g., exfiltrating data to an attacker-controlled EC2 instance or calling cloud APIs with stolen metadata credentials). Uses `ipset` (`hash:net` sets) for efficient matching regardless of the number of CIDRs.

The IP ranges are version-controlled and embedded in the `crib` binary (not fetched at runtime). A `lastUpdated` timestamp is stored alongside the data so staleness is visible. A CI job or manual script periodically pulls the latest ranges from provider sources (see [cloud metadata endpoints reference](../reference/cloud-metadata-endpoints.md#cloud-provider-ip-ranges-machine-readable)) and commits updates. This avoids network dependencies at container setup time and makes the blocklist auditable via git history.

This is opt-in because it blocks legitimate traffic to cloud-hosted services (many APIs, registries, and SaaS tools run on major cloud providers).

Allowlisted destinations (to avoid breaking common workflows when `blockCloudProviders` is enabled):

- LLM provider API endpoints (built-in allowlist of `api.anthropic.com`, `api.openai.com`, `generativelanguage.googleapis.com`, etc.)
- Package registries (`registry.npmjs.org`, `pypi.org`, `proxy.golang.org`, etc.)

The exact allowlist mechanism is TBD. IP-based allowlisting is fragile (CDN IPs change), so a DNS-based approach or proxy may be needed. If the allowlist proves too complex for v1, `blockCloudProviders` will ship as best-effort with clear documentation of what breaks.

### Wrapper script (sketch)

```bash
#!/usr/bin/env bash
# ~/.local/bin/sandbox - generated by crib sandbox plugin
# Policy: workspaceFolder=/workspaces/project, user=vscode

exec bwrap \
  --ro-bind / / \
  --dev /dev \
  --proc /proc \
  --bind /workspaces/project /workspaces/project \
  --bind /tmp /tmp \
  --bind /home/vscode/.crib_history /home/vscode/.crib_history \
  --tmpfs /home/vscode/.ssh \
  --tmpfs /home/vscode/.claude \
  -- "$@"
```

### Alias wrapper (sketch)

```bash
#!/usr/bin/env bash
# ~/.local/bin/claude - generated by crib sandbox plugin
echo "[crib sandbox] Running claude in sandboxed mode"
exec ~/.local/bin/sandbox /home/vscode/.local/bin/claude "$@"
```

## Scope

### v1 (this ADR)

- `bubblewrap`-based filesystem isolation via wrapper script.
- Plugin awareness via workspace state dir scanning.
- Optional agent aliases with banner message.
- `blockLocalNetwork` via `iptables` rules (metadata endpoints, RFC 1918).
- `blockCloudProviders` via `iptables` rules using published cloud provider IP ranges.
- Configuration in `devcontainer.json` customizations.

### v2 (future)

- `gh` CLI restrictions: wrapper script that intercepts mutation subcommands (`gh issue create`, `gh pr create`, `gh pr comment`, `gh pr merge`, `gh pr close`, `gh issue comment`, `gh release create`). Read-only commands (`gh pr view`, `gh issue list`, `gh pr diff`) allowed. `gh api` blocked entirely (covers both REST and GraphQL escape hatches; `gh api graphql -f query='mutation { ... }'` bypasses subcommand checks). Start conservative, relax later.
- Per-agent policy profiles (different restrictions for different agents).
- [`@anthropic-ai/sandbox-runtime`](https://github.com/anthropic-experimental/sandbox-runtime) as alternative backend.
- `blockDomains`: DNS-based domain blocking (not feasible with `iptables` alone, needs a proxy or DNS sinkhole).
- Network domain allowlisting via proxy (v1 only does IP-based blocklisting).

## Alternatives considered

### A. Extend the `codingagents` plugin

Add sandbox config to the existing `codingagents` plugin. Rejected because: sandboxing is agent-agnostic. Users want to sandbox `pi`, `aider`, and future agents too. Coupling sandbox to Claude Code credentials doesn't make sense.

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

The `sandbox` plugin should detect whether `bubblewrap` works at post-create time and warn (not fail) if it doesn't. This avoids breaking container creation for images that don't support nested namespaces.

Claude Code's sandbox runtime offers [`enableWeakerNestedSandbox`](https://code.claude.com/docs/en/sandboxing#security-limitations) for Docker environments without privileged namespaces, trading security for compatibility. Worth investigating whether a similar fallback is needed here.

### `iptables` in rootless mode

`iptables` inside a container requires real root privileges. In rootless Docker/Podman, even `CAP_NET_ADMIN` + `CAP_NET_RAW` may not be sufficient because the host user lacks real root. `blockLocalNetwork` and `blockCloudProviders` may silently fail in rootless setups. The plugin should detect this and warn.

### SSH agent usage (not extraction)

The SSH agent socket cannot leak private keys, but a sandboxed agent can still *use* the agent to authenticate to remote servers. This is by design (agents need git access), but means a compromised agent could push to repos or SSH into machines the user has access to.

## Revisit if

- A standard emerges for agent sandboxing configuration across tools.
- `bubblewrap` proves insufficient (e.g., agents that need nested namespaces).
- Network-level restrictions need to move beyond blocklisting to allowlisting, which would require a proxy component.
