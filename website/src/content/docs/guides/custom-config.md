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
