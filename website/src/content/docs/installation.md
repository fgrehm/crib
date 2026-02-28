---
title: Installation
description: How to install crib on your system.
---

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
