---
title: Custom Config Directory
description: Using a custom devcontainer directory with crib.
---

By default `crib` finds your devcontainer config by walking up from the current directory, looking for `.devcontainer/devcontainer.json`. If your config lives elsewhere (e.g. you have multiple configs or a non-standard name), use `--config` / `-C` to point directly to the folder that contains `devcontainer.json`:

```bash
crib -C .devcontainer-custom up
crib -C .devcontainer-custom shell
```

To avoid repeating that flag, create a `.cribrc` file in the directory you run `crib` from:

```ini
# .cribrc
config = .devcontainer-custom
```

An explicit `--config` on the command line takes precedence over `.cribrc`.

## Other `.cribrc` keys

`.cribrc` also carries per-project settings for bundled plugins:

```ini
# Devcontainer config location (same as --config / -C).
config = .devcontainer-custom

# Package cache providers (see guides/plugins).
cache = npm, pip, go

# Dotfiles overrides (see guides/plugins).
dotfiles.repository = https://github.com/user/work-dotfiles
dotfiles.targetPath = ~/work-dotfiles
dotfiles.installCommand = make install
# Or disable dotfiles for this project:
# dotfiles = false

# Skip specific bundled plugins for this project.
plugins.disable = ssh, dotfiles
# Or disable every bundled plugin:
# plugins = false
```

Format is one `key = value` per line. Lines starting with `#` are comments. See [Built-in Plugins](/crib/guides/plugins/) for what each plugin does and the full list of names you can pass to `plugins.disable`.
