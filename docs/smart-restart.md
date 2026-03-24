---
title: Smart Restart
description: How crib restart detects changes and picks the fastest strategy.
---

`crib restart` is faster than `crib rebuild` because it knows what changed. When you edit your devcontainer config, `restart` compares the current config against the stored one and picks the right strategy:

| What changed | What happens | Lifecycle hooks |
|---|---|---|
| Nothing | Simple container restart (`docker restart`) | `postStartCommand` + `postAttachCommand` |
| Volumes, mounts, ports, env, runArgs, user | Container recreated with new config | `postStartCommand` + `postAttachCommand` |
| Compose file contents (volumes, ports, env, etc.) | Container recreated with new config | `postStartCommand` + `postAttachCommand` |
| Image, Dockerfile, features, build args | Error, suggests `crib rebuild` | - |

This follows the [devcontainer spec's Resume Flow](https://containers.dev/implementors/spec/#lifecycle): on restart, only `postStartCommand` and `postAttachCommand` run. Creation-time hooks (`onCreateCommand`, `updateContentCommand`, `postCreateCommand`) are skipped since they already ran when the container was first created.

The practical effect: you can tweak a volume mount or add an environment variable, run `crib restart`, and be back in your container in seconds instead of waiting for a full rebuild and all creation hooks to re-execute.

```bash
# Changed a volume in docker-compose.yml? Or added a mount in devcontainer.json?
crib restart   # recreates the container, skips creation hooks

# Changed the base image or added a feature?
crib restart   # tells you to run 'crib rebuild' instead
```

## When to use restart vs rebuild

Use **`crib restart`** when you changed:
- Environment variables (`remoteEnv`, `containerEnv`)
- Volume mounts or bind mounts
- Port mappings (`forwardPorts`, `appPort`)
- `runArgs` or `remoteUser`
- Docker Compose file contents (volumes, ports, environment, networks, etc.)

Use **`crib rebuild`** when you changed:
- The base image (`image` or `FROM` in Dockerfile)
- The Dockerfile itself
- DevContainer Features (added, removed, or changed options)
- Build arguments (`build.args`)
- Anything that affects the built image

**Rule of thumb:** if the change affects how the container runs, use `restart`. If it affects what the image contains, use `rebuild`.

:::note
Compose file change detection uses a content hash, not a parsed comparison. If you change build-related fields in a compose file (like `build:` or `image:`), `restart` will recreate the container but will not rebuild the image. Use `crib rebuild` to pick up compose build changes.
:::
