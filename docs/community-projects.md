---
title: Community Projects
description: Tools built by the community around crib.
---

Tools and extensions built by the crib community. These are independent projects, not part of crib itself.

Have something that integrates with crib, or an idea for one? Open a discussion or PR on the [crib repo](https://github.com/fgrehm/crib) and we can list it here.

## Crib for VS Code and Cursor

**[crib-vscode](https://github.com/glsorre/crib-vscode)** (maintained by [@glsorre](https://github.com/glsorre)) is a VS Code and Cursor extension that wraps the `crib` CLI and turns its dev container workflow into a one-click experience inside your editor. Open a project with a `.devcontainer` config, run **Up** or **Attach**, and you're inside a running container - without the rebuild-and-wait cycle of "Reopen in Container," and without giving up the CLI for everything else.

The extension adds a **Crib → Workspaces** sidebar that lists your crib-managed containers, plus commands for the full lifecycle: up, attach, down, restart, rebuild, and remove. Attach goes through the **Dev Containers** extension: the plugin syncs a generated `nameConfig` from your `devcontainer.json` and hands off to "Attach to Running Container." It also merges feature-contributed VS Code extensions, supports SSH agent forwarding for crib's `ssh` plugin, and logs everything to a dedicated **Crib** output channel.

It ships as two extensions on the [Visual Studio Marketplace](https://marketplace.visualstudio.com/) and [Open VSX](https://open-vsx.org/):

- **[Crib Extension](https://marketplace.visualstudio.com/items?itemName=rightright-me.crib-vscode-main)** (`rightright-me.crib-vscode-main`) - workspace-side lifecycle and the Workspaces view
- **[Crib Attach Bridge](https://marketplace.visualstudio.com/items?itemName=rightright-me.crib-vscode-attach-bridge)** (`rightright-me.crib-vscode-attach-bridge`) - a small UI-host companion required for Remote SSH (and similar setups where the editor window and the machine running `crib` are different hosts)

For a local folder window, install the main extension plus Dev Containers. For Remote SSH, install both extensions - main on the remote, bridge on your laptop.
