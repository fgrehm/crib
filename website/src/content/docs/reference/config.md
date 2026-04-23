---
title: Configuration
description: crib configuration files and options.
---

crib reads settings from two TOML files: a user-level global config at
`~/.config/crib/config.toml` and a per-project `.cribrc` in the project root.
Project-level values override global values on key conflicts.

## Global config: `~/.config/crib/config.toml`

Respects `$XDG_CONFIG_HOME`. The file is optional; missing sections fall back
to crib's built-in defaults.

### `[dotfiles]`

| Key | Type | Default | Description |
|---|---|---|---|
| `repository` | string | | Git URL for dotfiles repository |
| `targetPath` | string | `~/dotfiles` | Clone destination inside the container |
| `installCommand` | string | | Command to run after cloning |

### `[plugins]`

| Key | Type | Default | Description |
|---|---|---|---|
| `disable` | array of strings | | Plugin names to skip globally |
| `disable_all` | boolean | `false` | Kill switch: skip every bundled plugin |

### `[workspace]`

Settings applied to every `crib up` regardless of project. Lower priority
than project-level config: project values win on key conflicts.

| Key | Type | Default | Description |
|---|---|---|---|
| `env` | map of string to string | | Environment variables injected into every container |
| `mount` | array of strings | | Mount specs (same format as devcontainer.json `mounts`) |
| `run_args` | array of strings | | Extra container runtime arguments (single-container mode only) |

Global `run_args` are honored only for single-container workspaces. For
compose-based workspaces, set runtime options directly in the compose YAML.

Example:

```toml
[workspace]
env = { DOTMEM_PATH = "/home/fabio/.mem" }
mount = ["type=bind,source=/home/fabio/.mem,target=/home/fabio/.mem"]
run_args = ["--cap-add", "SYS_PTRACE"]
```

## Per-project config: `.cribrc`

Placed in the project root. Merges with the global config; project values
override global on conflicts.

| Key | Type | Description |
|---|---|---|
| `config` | string | Devcontainer config directory (same as `-C` / `--config`) |
| `cache` | array of strings, or comma-separated string | Package cache providers (e.g. `"npm", "pip"`) |
| `dotfiles.repository` | string | Dotfiles repo URL (overrides global) |
| `dotfiles.targetPath` | string | Clone destination (overrides global) |
| `dotfiles.installCommand` | string | Install command (overrides global) |
| `dotfiles` | `"false"` | Kill switch: skip dotfiles for this project |
| `plugins.disable` | array of strings, or comma-separated string | Plugin names to skip for this project |
| `plugins` | `"false"` | Kill switch: skip every plugin for this project |
| `workspace.env` | map | Extra env for this project |
| `workspace.mount` | array | Extra mounts for this project |
| `workspace.run_args` | array | Extra runtime args for this project |

Both TOML array syntax and the legacy comma-separated string form are
accepted for list values:

```toml
# TOML array (preferred)
plugins.disable = ["ssh", "dotfiles"]
cache = ["npm", "pip"]

# Comma-separated string (legacy format, still supported)
plugins.disable = "ssh, dotfiles"
cache = "npm, pip"
```

Example `.cribrc`:

```toml
config = ".devcontainer-custom"
cache = ["npm", "pip"]

[dotfiles]
repository = "git@github.com:user/dots"

[plugins]
disable = ["ssh"]

[workspace]
env = { PROJECT_FLAG = "on" }
```
