---
title: Lifecycle Hooks
description: How crib handles devcontainer lifecycle hooks.
---

The devcontainer spec defines [lifecycle hooks](https://containers.dev/implementors/spec/#lifecycle) that run at different stages. `crib` supports all of them:

| Hook | Runs on | When | Runs once? |
|------|---------|------|------------|
| `initializeCommand` | Host | Before image build/pull, every `crib up` | No |
| `onCreateCommand` | Container | After first container creation | Yes |
| `updateContentCommand` | Container | After first container creation | Yes |
| `postCreateCommand` | Container | After `onCreateCommand` + `updateContentCommand` | Yes |
| `postStartCommand` | Container | After every container start | No |
| `postAttachCommand` | Container | On every `crib up` | No |

Note: in the official spec, `updateContentCommand` re-runs when source content changes (e.g. git pull in Codespaces). `crib` doesn't detect content updates, so it behaves identically to `onCreateCommand`. Similarly, `postAttachCommand` maps to "attach" in editors. `crib` runs it on every `crib up` since there's no separate attach step.

Each hook accepts a string, an array, or a map of named commands:

```jsonc
// string
"postCreateCommand": "npm install"

// array
"postCreateCommand": ["npm", "install"]

// named commands — all run in parallel, all must succeed
"postCreateCommand": {
  "deps": "npm install",
  "db": "rails db:setup"
}
```

For `initializeCommand` (host-side), the array form runs as a direct exec without a shell. For container hooks, both string and array forms are run through `sh -c`.

Here's a `devcontainer.json` showing all hooks:

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

## `initializeCommand`

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

**Multiple checks with named commands (run in parallel):**

```jsonc
{
  "initializeCommand": {
    "env": "test -f .env || cp .env.example .env",
    "credentials": "test -f config/master.key || (echo 'Missing config/master.key' >&2 && exit 1)"
  }
}
```

## `onCreateCommand`

Runs once after the container is first created. Use it for one-time setup that should survive container restarts.

**Install dependencies and set up the database:**

```jsonc
{
  "onCreateCommand": "bundle install && rails db:setup"
}
```

**Multiple setup tasks with named commands (run in parallel):**

```jsonc
{
  "onCreateCommand": {
    "deps": "npm install",
    "db": "rails db:setup",
    "tools": "mise install"
  }
}
```

## `postCreateCommand`

Runs once after `onCreateCommand` and `updateContentCommand` finish. Good for configuration that depends on installed dependencies.

**Configure git safe directory:**

```jsonc
{
  "postCreateCommand": "git config --global --add safe.directory ${containerWorkspaceFolder}"
}
```

## `postStartCommand`

Runs after every container start (including restarts). Use it for services that need to be running.

**Start background services:**

```jsonc
{
  "postStartCommand": {
    "redis": "redis-server --daemonize yes",
    "postgres": "pg_ctlcluster 16 main start"
  }
}
```

## `postAttachCommand`

Runs on every `crib up`. Use it for per-session output or checks.

**Show tool versions:**

```jsonc
{
  "postAttachCommand": "node -v && npm -v"
}
```

## `waitFor`

`waitFor` controls when `crib up` reports "Container ready." in its progress output. It doesn't skip any hooks — all hooks still run to completion. It only affects when the ready message appears.

Default: `updateContentCommand`.

Valid values: `initializeCommand`, `onCreateCommand`, `updateContentCommand`, `postCreateCommand`, `postStartCommand`.

**Wait until postCreate before reporting ready:**

```jsonc
{
  "waitFor": "postCreateCommand",
  "postCreateCommand": "bundle install"
}
```

With this config, `crib up` shows "Container ready." only after `bundle install` finishes. Useful when `postCreateCommand` is a prerequisite for the container to be usable.
