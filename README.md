<p align="center">
  <img src="./images/icon.png" alt="crib logo" width="200">
</p>

# crib

Devcontainers without the ceremony.

crib reads your `.devcontainer` config, builds the container, and gets out of your way.

```
cd my-project
crib up        # build and start the devcontainer
crib shell     # drop into a shell
crib restart   # restart (picks up config changes)
crib stop      # stop it
crib delete    # clean up
```

## Installation

Download the latest binary from [GitHub releases](https://github.com/fgrehm/crib/releases):

```bash
# Replace OS and ARCH as needed (linux/darwin, amd64/arm64)
curl -Lo crib.tar.gz https://github.com/fgrehm/crib/releases/latest/download/crib_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz
tar xzf crib.tar.gz crib
install -m 755 crib ~/.local/bin/crib
rm crib.tar.gz
```

Make sure `~/.local/bin` is in your `PATH`.

Or install with [mise](https://mise.jdx.dev/):

```bash
mise use github:fgrehm/crib
```

## Commands

| Command | Description |
|---------|-------------|
| `crib up` | Create or start the workspace container |
| `crib shell` | Open an interactive shell (detects zsh/bash/sh) |
| `crib exec` | Execute a command in the workspace container |
| `crib restart` | Restart the workspace container (picks up safe config changes) |
| `crib stop` | Stop the workspace container |
| `crib delete` | Delete the workspace container and state |
| `crib rebuild` | Rebuild the workspace (delete + up) |
| `crib list` | List all workspaces |
| `crib status` | Show workspace container status |
| `crib version` | Show version information |

## Using a custom devcontainer directory

By default crib finds your devcontainer config by walking up from the current directory, looking for `.devcontainer/devcontainer.json`. If your config lives elsewhere (e.g. you have multiple configs or a non-standard name), use `--config` / `-C` to point directly to the folder that contains `devcontainer.json`:

```
crib -C .devcontainer-custom up
crib -C .devcontainer-custom shell
```

To avoid repeating that flag, create a `.cribrc` file in the directory you run crib from:

```
# .cribrc
config = .devcontainer-custom
```

An explicit `--config` on the command line takes precedence over `.cribrc`.

## Lifecycle hooks

The devcontainer spec defines [lifecycle hooks](https://containers.dev/implementors/spec/#lifecycle) that run at different stages. crib supports all of them:

| Hook | Runs on | When | Runs once? |
|------|---------|------|------------|
| `initializeCommand` | Host | Before image build/pull, every `crib up` | No |
| `onCreateCommand` | Container | After first container creation | Yes |
| `updateContentCommand` | Container | After first container creation | Yes |
| `postCreateCommand` | Container | After `onCreateCommand` + `updateContentCommand` | Yes |
| `postStartCommand` | Container | After every container start | No |
| `postAttachCommand` | Container | On every `crib up` | No |

Note: in the official spec, `updateContentCommand` re-runs when source content changes (e.g. git pull in Codespaces). crib doesn't detect content updates, so it behaves identically to `onCreateCommand`. Similarly, `postAttachCommand` maps to "attach" in editors. crib runs it on every `crib up` since there's no separate attach step.

Each hook accepts a string, an array, or a map of named commands:

```jsonc
// string
"postCreateCommand": "npm install"

// array
"postCreateCommand": ["npm", "install"]

// named commands (all run, order is not guaranteed)
"postCreateCommand": {
  "deps": "npm install",
  "db": "rails db:setup"
}
```

For `initializeCommand` (host-side), the array form runs as a direct exec without a shell. For container hooks, both string and array forms are run through `sh -c`.

Here's a devcontainer.json showing all hooks:

```jsonc
{
  // Host: fail fast if secrets are missing.
  "initializeCommand": "test -f config/master.key || (echo 'Missing config/master.key' >&2 && exit 1)",

  // Container, once: install dependencies and set up the database.
  "onCreateCommand": "bundle install && rails db:setup",

  // Container, once: same timing as onCreateCommand in crib (see note above).
  "updateContentCommand": "bundle install",

  // Container, once: runs after onCreateCommand + updateContentCommand finish.
  "postCreateCommand": "git config --global --add safe.directory /workspaces/myapp",

  // Container, every start: launch background services.
  "postStartCommand": "redis-server --daemonize yes",

  // Container, every crib up: per-session info.
  "postAttachCommand": "ruby -v && rails --version"
}
```

### initializeCommand

`initializeCommand` is the only hook that runs on the host. It runs before the image is built or pulled, making it useful for pre-flight checks and local file setup.

**Fail fast when required secrets are missing:**

```jsonc
{
  "initializeCommand": "test -f config/master.key || (echo 'Missing config/master.key' >&2 && exit 1)"
}
```

If `config/master.key` is missing, `crib up` fails immediately with a clear message instead of building an image that won't start.

**Seed `.env` from a template:**

```jsonc
{
  "initializeCommand": "test -f .env || cp .env.example .env"
}
```

This ensures `.env` is present on the host before the container starts, so bind mounts and docker compose `env_file` directives pick it up.

**Multiple checks with named commands:**

```jsonc
{
  "initializeCommand": {
    "env": "test -f .env || cp .env.example .env",
    "credentials": "test -f config/master.key || (echo 'Missing config/master.key' >&2 && exit 1)"
  }
}
```

## Smart restart

`crib restart` is faster than `crib rebuild` because it knows what changed. When you edit your devcontainer config, `restart` compares the current config against the stored one and picks the right strategy:

| What changed | What happens | Lifecycle hooks |
|---|---|---|
| Nothing | Simple container restart (`docker restart`) | `postStartCommand` + `postAttachCommand` |
| Volumes, mounts, ports, env, runArgs, user | Container recreated with new config | `postStartCommand` + `postAttachCommand` |
| Image, Dockerfile, features, build args | Error — suggests `crib rebuild` | — |

This follows the [devcontainer spec's Resume Flow](https://containers.dev/implementors/spec/#lifecycle): on restart, only `postStartCommand` and `postAttachCommand` run. Creation-time hooks (`onCreateCommand`, `updateContentCommand`, `postCreateCommand`) are skipped since they already ran when the container was first created.

The practical effect: you can tweak a volume mount or add an environment variable, run `crib restart`, and be back in your container in seconds instead of waiting for a full rebuild and all creation hooks to re-execute.

```
# Changed a volume in docker-compose.yml? Or added a mount in devcontainer.json?
crib restart   # recreates the container, skips creation hooks

# Changed the base image or added a feature?
crib restart   # tells you to run 'crib rebuild' instead
```

## Git inside devcontainers

Since crib doesn't inject agents or set up SSH tunnels, git push and commit signing need to be configured through your devcontainer config. The setup has two parts: forwarding your SSH agent into the container, and configuring git to use SSH signing.

### SSH agent forwarding

Mount your host's SSH agent socket into the container and set the `SSH_AUTH_SOCK` environment variable so git (and ssh) can find it:

**Image/Dockerfile-based:**

```jsonc
// devcontainer.json
{
  "mounts": [
    "type=bind,source=${localEnv:SSH_AUTH_SOCK},target=/tmp/ssh-agent.sock"
  ],
  "containerEnv": {
    "SSH_AUTH_SOCK": "/tmp/ssh-agent.sock"
  }
}
```

**Docker Compose-based:**

```yaml
# docker-compose.yml
services:
  app:
    volumes:
      - ${SSH_AUTH_SOCK}:/tmp/ssh-agent.sock
    environment:
      SSH_AUTH_SOCK: /tmp/ssh-agent.sock
```

Verify it works inside the container:

```
crib shell
ssh-add -l          # should list your keys
ssh -T git@github.com
```

### Commit signing with SSH keys

Git supports signing commits with SSH keys (no GPG needed). Add this to a `.gitconfig` that gets mounted or copied into the container, or run the commands via a lifecycle hook.

**Image/Dockerfile-based:**

```jsonc
// devcontainer.json
{
  "mounts": [
    "type=bind,source=${localEnv:SSH_AUTH_SOCK},target=/tmp/ssh-agent.sock"
  ],
  "containerEnv": {
    "SSH_AUTH_SOCK": "/tmp/ssh-agent.sock"
  },
  "postCreateCommand": "git config --global gpg.format ssh && git config --global user.signingkey 'key::ssh-ed25519 AAAA...' && git config --global commit.gpgsign true"
}
```

**Docker Compose-based:**

```yaml
# docker-compose.yml
services:
  app:
    volumes:
      - ${SSH_AUTH_SOCK}:/tmp/ssh-agent.sock
    environment:
      SSH_AUTH_SOCK: /tmp/ssh-agent.sock
```

```jsonc
// devcontainer.json
{
  "dockerComposeFile": "docker-compose.yml",
  "service": "app",
  "postCreateCommand": "git config --global gpg.format ssh && git config --global user.signingkey 'key::ssh-ed25519 AAAA...' && git config --global commit.gpgsign true"
}
```

Replace `ssh-ed25519 AAAA...` with your actual public key. You can get it from `ssh-add -L`.

GitHub and GitLab verify SSH signatures on their end, so this is all you need for signed commits to show as "Verified" on push.

**Optional: local signature verification.** If you also want `git log --show-signature` to work inside the container, you need an `allowed_signers` file on your host. Use [`initializeCommand`](#initializecommand) to generate it automatically on every `crib up`, so the file is ready for bind mounting:

```jsonc
// add to devcontainer.json
"initializeCommand": "mkdir -p ~/.ssh && ssh-add -L | head -1 | awk '{print \"your@email.com \" $0}' > ~/.ssh/allowed_signers"
```

The `mkdir -p` ensures `~/.ssh` exists on the host before writing the file. This matters on fresh machines or CI environments where the directory may not exist yet.

Then mount it and tell git about it:

```jsonc
// add to devcontainer.json mounts
"type=bind,source=${localEnv:HOME}/.ssh/allowed_signers,target=/home/vscode/.allowed_signers,readonly"
```

```bash
# add to postCreateCommand
git config --global gpg.ssh.allowedSignersFile ~/.allowed_signers
```

**Why mount outside `.ssh`?** When Docker creates intermediate directories for a bind mount, it sets ownership to `root:root`. If you mount into `/home/vscode/.ssh/allowed_signers` and `.ssh` doesn't already exist in the image, Docker creates it as `root:root`, making the entire directory inaccessible to the container user. Mounting to `~/.allowed_signers` (or any path outside `.ssh`) avoids this.

**Warning: never `chown`/`chmod` bind-mounted paths from inside the container.** These operations write through to the host filesystem in both rootful and rootless modes. With rootful Docker, it changes your host files to the container user's UID. With rootless Docker/Podman, it remaps through subordinate UID ranges, which can make your files owned by a UID your host user can't access. Either way, you can lose access to your own files. Mount to a path that avoids the problem instead. See [Troubleshooting](docs/troubleshooting.md#bind-mount-changed-permissions-on-host-files) if this already happened.

### Mounting your host gitconfig

If you already have git configured on your host (user name, email, aliases), you can mount your gitconfig read-only instead of recreating it.

**Image/Dockerfile-based:**

```jsonc
// devcontainer.json
{
  "mounts": [
    "type=bind,source=${localEnv:HOME}/.gitconfig,target=/home/vscode/.gitconfig,readonly"
  ]
}
```

**Docker Compose-based:**

```yaml
# docker-compose.yml
services:
  app:
    volumes:
      - ${HOME}/.gitconfig:/home/vscode/.gitconfig:ro
```

Note: if your host gitconfig references paths that don't exist in the container (e.g. credential helpers, include files), git will warn or error. A dedicated container gitconfig via `postCreateCommand` avoids this.

## Design Principles

- **Implicit workspace resolution.** `cd` into a project directory and run commands. crib walks up from your current directory to find `.devcontainer/devcontainer.json`. No workspace names to remember.
- **No agent injection.** All container setup happens via `docker exec` from the host. Nothing gets installed inside your container that you didn't ask for.
- **No SSH, no providers, no IDE integration.** crib is a CLI tool. It starts containers. What you do inside them is your business.
- **Docker and Podman as first-class runtimes.** Auto-detected, configurable via `CRIB_RUNTIME`.
- **Human-readable naming.** Containers show up as `crib-myproject` in `docker ps`, not opaque hashes.

## Status

Working and usable. Core devcontainer workflows (image, Dockerfile, and Docker Compose based) are implemented, including lifecycle hooks, feature installation, and workspace state management.

## Why

The [devcontainer spec](https://containers.dev/) is a good idea. A JSON file describes your development environment, and tooling builds a container from it. But the existing tools layer on complexity that gets in the way.

[DevPod](https://github.com/loft-sh/devpod) was the most promising open-source option: provider-agnostic, IDE-agnostic, well-designed. But it was built for a broader scope than most people need. Providers, agents injected into containers, SSH tunnels, gRPC, IDE integrations. For someone who just wants to `docker exec` into a container and use their terminal, that is a lot of moving parts between you and your shell.

Then [DevPod seems to be effectively abandoned](https://github.com/loft-sh/devpod/issues/1963) when Loft Labs shifted focus to vCluster. The project stopped receiving updates in April 2025, with no official statement and no path forward for the community.

crib takes a different approach: do less, but do it well. Read the devcontainer config, build the image, run the container, set up the user and lifecycle hooks, done. No agents, no SSH, no providers, no IDE assumptions. Just Docker (or Podman) doing what Docker does.

## Background

This isn't the first time [@fgrehm](https://github.com/fgrehm) has gone down this road. [vagrant-boxen](https://github.com/fgrehm/vagrant-boxen) (2013) tried to make Vagrant machines manageable without needing Puppet or Chef expertise. [Ventriloquist](https://fabiorehm.com/blog/2013/09/11/announcing-ventriloquist/) (2013) combined Vagrant and Docker to give developers portable, disposable dev VMs. [devstep](https://fabiorehm.com/blog/2014/08/26/devstep-development-environments-powered-by-docker-and-buildpacks/) (2014) took it further with "git clone, one command, hack" using Docker and Heroku-style buildpacks. The devcontainer spec has since standardized what that project was trying to achieve, so crib builds on that foundation instead of reinventing it.

The [experience of using DevPod as a terminal-first developer](https://fabiorehm.com/blog/2025/11/11/devpod-ssh-devcontainers/), treating devcontainers as remote machines you SSH into rather than IDE-managed environments, shaped many of crib's design decisions. The pain points (broken git signing wrappers, unnecessary cache invalidation, port forwarding conflicts, agent-related complexity) all pointed toward the same conclusion: the simplest path is often the best one.

## Documentation

- [Devcontainer Spec Reference](docs/devcontainers-spec.md) - distilled reference of the devcontainer spec for quick lookup
- [Implementation Notes](docs/implementation-notes.md) - quirks, workarounds, and spec compliance status
- [Troubleshooting](docs/troubleshooting.md) - common issues and solutions
- [Development](docs/development.md) - building, testing, and contributing

## License

MIT

---

Logo created with ChatGPT image generation, prompted by Claude.

Built with [Claude Code](https://claude.ai/code) (Opus 4.6, Sonnet 4.6, Haiku 4.5).
