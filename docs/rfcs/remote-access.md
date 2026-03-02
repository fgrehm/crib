# RFC: Remote Access Plugin

**Status:** Draft
**Goal:** Let users SSH into crib containers with native filesystem performance, building toward a self-hosted Codespaces-like experience.

## Problem

crib uses bind mounts for source code. This is fast on Linux but 3x slower on macOS and 5-10x slower on Windows (VM boundary). DevPod solves this by injecting an agent + SSH server, giving editors native filesystem speed. crib needs a similar capability without abandoning its "no agent" philosophy.

The key insight: crib's plugin system can add SSH access as an opt-in behavior, not a core architectural requirement. The user chooses to enable it; containers without the plugin stay clean.

## Design overview

Three independent capabilities that compose well:

```
┌───────────────────────────────────────────────────────┐
│                  Volume workspaces                    │
│  Source code lives in a Docker volume (native speed)  │
│  Clone on create, git push/pull to sync               │
└──────────────────────┬────────────────────────────────┘
                       │ enables
┌──────────────────────▼──────────────────────────────┐
│                  SSH-in plugin                      │
│  sshd inside container, host keys injected          │
│  ssh user@localhost -p 2222 → edit with any tool    │
└──────────────────────┬──────────────────────────────┘
                       │ enables
┌──────────────────────▼──────────────────────────────┐
│              Codespaces-like experience             │
│  crib up from a repo URL (no local clone needed)    │
│  Web terminal / browser IDE (optional, way later)   │
└─────────────────────────────────────────────────────┘
```

Each layer is useful on its own. You can use volume workspaces without SSH (just `crib shell`). You can use SSH with bind mounts (slower but simpler). The Codespaces-like layer is the long-term vision, not an MVP requirement.

---

## Layer 1: SSH-in plugin

### What it does

Starts a lightweight SSH server (dropbear) inside the container during `postStart`, injects the user's public keys, and exposes a port. The user connects with any SSH client and gets a shell with the probed environment.

### Plugin behavior

Runs during `pre-container-run`:

1. **Detect host SSH keys**: read `~/.ssh/*.pub` (already done by existing `ssh` plugin)
2. **Add a published port**: append `--publish` for the SSH port (default 2222, configurable)
3. **After container creation**: install dropbear (if not present), write `authorized_keys`, start the daemon

Runs during `postStart` (via a file copy + exec, similar to existing plugin patterns):

1. Copy a small bootstrap script into the container
2. `docker exec` the script: install dropbear, configure it, start it
3. Print connection info: `SSH: ssh -p 2222 vscode@localhost`

### Configuration

```jsonc
{
  "customizations": {
    "crib": {
      "remote": {
        "ssh": true,           // enable SSH server (default: false)
        "sshPort": 2222,       // host port to publish (default: 2222)
        "sshServer": "dropbear" // or "openssh" (default: dropbear)
      }
    }
  }
}
```

Or enable globally via `~/.config/crib/config.toml`:

```toml
[plugins.remote]
ssh = true
sshPort = 2222
```

### Why dropbear

~100KB static binary, no config files needed, starts instantly. OpenSSH is an option for users who need ProxyJump or advanced features, but dropbear covers 95% of use cases. The plugin could bundle a static dropbear binary (cross-compiled for common architectures) to avoid requiring package installation inside the container.

### Alternative: building on `crib shell`

`crib shell` already solves environment detection, user switching, and shell selection. An SSH server could build on this in two ways:

**Option A: crib as SSH proxy.** crib listens on a port on the host and translates SSH connections into `docker exec` calls — effectively "networked `crib shell`." Nothing installed in the container.

- Pro: true "no agent" — zero footprint inside the container
- Con: no SCP/SFTP support, no SSH subsystems. Editors like nvim-over-SSH need real SSH protocol support (pty allocation, environment passing, subsystem forwarding). A proxy would need to reimplement or tunnel all of this.

**Option B: sshd inside container, environment from `crib shell`.** Run a real dropbear/openssh inside the container for protocol compatibility, but configure it to use the same probed environment that `crib shell` uses. The SSH plugin would write the probed env vars to `/etc/environment` or a profile script so SSH sessions match `crib shell` sessions.

- Pro: full SSH protocol (SCP, SFTP, rsync, editor remote plugins all work)
- Pro: environment consistency with `crib shell` / `crib exec`
- Con: something installed inside the container (but opt-in via plugin)

**Recommendation: Option B.** The SSH protocol is too complex to proxy well, and editors expect it. The probed environment bridge means SSH sessions feel identical to `crib shell`.

A `crib ssh` convenience command wraps this: looks up the workspace's SSH port from the stored result and runs `ssh -p <port> <user>@localhost`.

### Relationship to existing ssh plugin

The existing `ssh` plugin forwards the host's SSH agent *into* the container (outbound SSH: git push, ssh to remote servers). This new capability is the reverse direction: SSH *into* the container from the host. They're complementary — you'd typically want both.

Options:
- **Extend the existing ssh plugin** (add an `ssh.server` config section alongside the existing agent forwarding)
- **New plugin** (`remote`) that depends on or coordinates with the `ssh` plugin

Extending the existing plugin is probably cleaner since both deal with SSH configuration.

### Open questions

- **Port conflicts**: what if 2222 is already taken? Auto-increment? Let the user specify?
- **Multiple workspaces**: each needs a different port. Auto-assign from a range?
- **Connection helper**: `crib ssh` command that looks up the port and runs `ssh -p <port> <user>@localhost`? Saves the user from remembering ports.

---

## Layer 2: Volume workspaces

### What it does

Instead of bind-mounting source from the host, creates a Docker volume and clones the repository into it. The container sees native filesystem speed. The trade-off: you can't edit files from the host with a local editor (unless you SSH in).

### How it works

1. User enables volume mode (per-project or global config)
2. During `crib up`, instead of `--mount type=bind,source=<host-path>`:
   - Create a named volume: `crib-{workspace-id}-src`
   - Mount it at the workspace folder
   - Clone the repo into the volume (using the host's git and SSH agent)
3. Subsequent `crib up` runs skip the clone if the volume already exists
4. Source changes are managed via git inside the container (push/pull)

### Plugin hook requirements

This is the hard part. The current plugin API runs during `pre-container-run` and can inject mounts, env vars, and run args. Volume workspace mode needs to:

1. **Replace** the default workspace mount (not just add a mount)
2. **Run a clone step** after the volume is created but before lifecycle hooks

Two approaches:

**Option A: New hook point** — `onConfigureWorkspace` that can modify `RunOptions.WorkspaceMount`. Clean, but requires core changes.

**Option B: Config override** — the plugin writes a `workspaceMount` override that the engine reads before creating the container. More declarative, less engine change.

**Option C: Two-phase** — plugin creates the volume and clones via a temporary container during `pre-container-run`. Then sets `workspaceMount` to point at the volume. The engine doesn't need to know about volumes specifically.

Option C is the most plugin-shaped. The plugin:
1. Creates the Docker volume (`docker volume create crib-{id}-src`)
2. Runs a temporary container to clone into the volume if empty
3. Returns a response that overrides `workspaceMount` to use the volume

This requires one new capability in the plugin API: the ability to override `workspaceMount`. Currently plugins can only *add* mounts.

### Configuration

```jsonc
{
  "customizations": {
    "crib": {
      "remote": {
        "workspace": "volume",    // "bind" (default) or "volume"
        "repository": ""          // optional: override git remote URL
      }
    }
  }
}
```

### Sync strategy

**Git-based (recommended):** Source lives in the volume. You edit inside the container (via SSH + nvim/vim, or via `crib shell`). You push/pull via git. No sync daemon, no conflict resolution, no moving parts.

**Not planned: bidirectional file sync.** Mutagen-style sync adds a daemon, conflict resolution, ignore rules, and a whole category of "why is my file different" bugs. The git-based approach is simpler and matches how Codespaces works.

### What about uncommitted changes?

If you `crib down` with uncommitted changes in a volume workspace, the volume persists (Docker volumes survive container removal). Next `crib up` picks up where you left off. `crib remove` could optionally delete the volume too (with a warning).

---

## Layer 3: Codespaces-like experience (future vision)

This is the long-term picture, not something to build now. Documenting it to make sure layers 1 and 2 don't paint us into a corner.

### `crib up` from a URL

```bash
crib up github.com/org/repo
```

No local clone needed. crib:
1. Creates a volume workspace
2. Clones the repo into the volume
3. Reads `.devcontainer/devcontainer.json` from the clone
4. Builds and starts the container
5. Starts SSH server
6. Prints connection info

This is a natural extension of layers 1 + 2. The main new thing is accepting a repository URL instead of requiring a local directory.

### Web terminal (way later)

A small HTTP server that serves a terminal (xterm.js or similar) connected to the container. Think `ttyd` or `gotty` bundled as a crib plugin. Not a priority, but the architecture doesn't block it.

### Multi-machine (out of scope)

Running containers on remote machines (cloud VMs, Kubernetes) is what DevPod providers do. crib is local-only by design. If you want remote, use DevPod (or did, when it was maintained), Coder, or Codespaces. crib's value is simplicity on local machines.

---

## Implementation phases

### Phase 1: SSH-in (small, high value)

- Extend `ssh` plugin with server capability
- Install dropbear via `docker exec` after container creation
- Inject authorized_keys from host
- Publish port, print connection info
- Add `crib ssh` convenience command

**Effort:** ~1-2 days. Most complexity is in the existing ssh plugin infrastructure.
**Value:** Your teammates can `ssh -p 2222 user@localhost` and use nvim at native speed (with bind mounts on Linux, and it's the prerequisite for volume mode on macOS).

### Phase 2: Volume workspaces

- Extend plugin API to allow `workspaceMount` override
- Volume creation + clone logic in the remote plugin
- Handle `crib remove` with volume cleanup
- Test with SSH-in for the full "edit inside container" workflow

**Effort:** ~3-5 days. The plugin API change is the risk area.
**Value:** macOS/Windows users get native filesystem performance. Combined with SSH, this is the DevPod experience without the agent.

### Phase 3: URL-based workspaces

- Accept `crib up <repo-url>`
- Clone into volume, discover devcontainer config
- Wire up to existing `up` flow

**Effort:** ~2-3 days on top of phase 2.
**Value:** Zero-local-clone workflow. "Send someone a URL, they run one command, they're developing."

### Phase 4+ (future, not designed yet)

- Web terminal plugin
- `crib connect` that auto-configures SSH config entries (`Host crib-myproject`)
- Editor integration (neovim remote plugin, JetBrains Gateway config generation)
- Pre-built volume images (like Codespaces prebuilds but for volumes)

---

## What the plugin API needs

Summarizing the changes to crib core that this RFC requires:

### Phase 1 (SSH-in)

- **No core changes needed.** The existing plugin API already supports adding published ports (via `RunArgs`), file copies, and environment variables. The SSH server setup happens via `docker exec` after container creation, which is already how `FileCopy` works.

### Phase 2 (Volume workspaces)

- **Plugin API addition**: ability to override `workspaceMount` in `PreContainerRunResponse`. Currently plugins can only add to `Mounts[]`. The new field would be something like `WorkspaceMountOverride *config.Mount` — if set, replaces the default workspace mount.
- **Plugin API addition**: ability to run a "pre-create" step (create volume, clone repo) that happens before the container is created. This could be a new hook (`PreCreate`) or could be done inside `PreContainerRun` by shelling out to `docker volume create` + a temporary clone container.

### Phase 3 (URL workspaces)

- **Core addition**: accept a repository URL as workspace source in `cmd/up.go`. This changes workspace resolution — instead of walking up from CWD, it creates a workspace from a URL. The workspace store would need a new source type.

---

## Risks and alternatives

### "Just use DevPod"

DevPod already does all of this. But it's abandoned, and its agent injection model means extra processes and complexity inside every container. If DevPod comes back to life, crib's plugin-based approach is still valuable for users who want a simpler tool.

### "Just use VS Code"

VS Code Dev Containers is excellent. But it requires VS Code. Terminal-first developers using nvim, helix, or emacs don't have this option. crib's SSH-in fills that gap.

### Dropbear security

Running an SSH server inside a dev container is a local-development convenience, not a production security boundary. The container is on `localhost`, the port is only published to the host. But it's worth noting in docs that this shouldn't be exposed to untrusted networks.

### Volume data loss

Docker volumes are local to the Docker host. No replication, no backup. If the host dies, the volume is gone. Git is the backup strategy — commit and push regularly. This matches how Codespaces works (volumes are ephemeral, git is the source of truth).

---

## References

- [DevPod architecture](https://github.com/loft-sh/devpod) — agent injection + SSH approach (reference for what to emulate / avoid)
- [VS Code: Clone Repository in Container Volume](https://code.visualstudio.com/docs/devcontainers/containers#_quick-start-open-a-git-repository-or-github-pr-in-an-isolated-container-volume) — volume workspace UX to match
- [VS Code: Improve disk performance](https://code.visualstudio.com/remote/advancedcontainers/improve-performance) — named volume trick for heavy directories
- [Dropbear SSH](https://matt.ucc.asn.au/dropbear/dropbear.html) — lightweight SSH server candidate
- [Docker on macOS performance (2025)](https://www.paolomainardi.com/posts/docker-performance-macos-2025/) — bind mount benchmarks motivating volume workspaces
- [crib plugin development guide](https://fgrehm.github.io/crib/contributing/plugin-development/) — current plugin API to extend
- [GitHub Codespaces architecture](https://docs.github.com/en/codespaces/overview) — the "north star" UX for the long-term vision
- [ttyd](https://github.com/tsl0922/ttyd) — web terminal candidate for future Phase 4
