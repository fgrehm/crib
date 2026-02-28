<p align="center">
  <img src="./images/logo.png" alt="crib logo" width="200">
</p>

# crib

Devcontainers without the ceremony.

Your `devcontainer.json`, minus the IDE. crib builds the container and stays out of your way.

**[Documentation](https://fgrehm.github.io/crib/)**

> **Note:** This is the `main` branch where active development happens.
> Docs here may describe unreleased features. For documentation matching
> the latest release, see the [`stable`](https://github.com/fgrehm/crib/tree/stable) branch.

```
cd my-project
crib up        # build and start the devcontainer
crib shell     # drop into a shell
crib restart   # restart (picks up config changes)
crib down      # stop and remove the container
crib remove    # remove container and workspace state
```

## Installation

Install with [mise](https://mise.jdx.dev/):

```bash
mise use github:fgrehm/crib
```

Or download the latest binary from [GitHub releases](https://github.com/fgrehm/crib/releases):

```bash
# Replace OS and ARCH as needed (linux/darwin, amd64/arm64)
curl -Lo crib.tar.gz https://github.com/fgrehm/crib/releases/latest/download/crib_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz
tar xzf crib.tar.gz crib
install -m 755 crib ~/.local/bin/crib
rm crib.tar.gz
```

Make sure `~/.local/bin` is in your `PATH`.

## License

MIT

---

Logo created with ChatGPT image generation, prompted by Claude.

Built with [Claude Code](https://claude.ai/code) (Opus 4.6, Sonnet 4.6, Haiku 4.5).
