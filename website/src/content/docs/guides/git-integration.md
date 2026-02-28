---
title: Git Integration
description: Setting up git push, commit signing, and SSH agent forwarding inside devcontainers.
---

Since crib doesn't inject agents or set up SSH tunnels, git push and commit signing need to be configured through your devcontainer config. The setup has two parts: forwarding your SSH agent into the container, and configuring git to use SSH signing.

## SSH agent forwarding

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

## Commit signing with SSH keys

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

**Optional: local signature verification.** If you also want `git log --show-signature` to work inside the container, you need an `allowed_signers` file on your host. Use [`initializeCommand`](/crib/guides/lifecycle-hooks/#initializecommand) to generate it automatically on every `crib up`, so the file is ready for bind mounting:

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

:::caution
Never `chown`/`chmod` bind-mounted paths from inside the container. These operations write through to the host filesystem in both rootful and rootless modes. With rootful Docker, it changes your host files to the container user's UID. With rootless Docker/Podman, it remaps through subordinate UID ranges, which can make your files owned by a UID your host user can't access. Either way, you can lose access to your own files. Mount to a path that avoids the problem instead. See [Troubleshooting](/crib/reference/troubleshooting/#bind-mount-changed-permissions-on-host-files) if this already happened.
:::

## Mounting your host gitconfig

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
