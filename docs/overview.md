---
title: Overview
description: What crib is, why it exists, and the philosophy behind it.
---

`crib` is a CLI tool that reads your `.devcontainer/devcontainer.json` config, builds the container, and gets out of your way. No agents injected into your container, no SSH tunnels, no IDE integration. Just Docker (or Podman) doing what Docker does.

:::note[🐧 Linux only]
`crib` is Linux-only. macOS and Windows support may be added if there's interest.
:::

## Principles

- **Implicit workspace resolution.** `cd` into a project directory and run commands. `crib` walks up from your current directory to find `.devcontainer/devcontainer.json`. No workspace names to remember.
- **No agent injection.** All container setup happens via `docker exec` from the host. Nothing gets installed inside your container that you didn't ask for.
- **No SSH, no providers, no IDE integration.** `crib` is a CLI tool. It starts containers. What you do inside them is your business.
- **Docker and Podman as first-class runtimes.** Auto-detected, configurable via `CRIB_RUNTIME`.
- **Human-readable naming.** Containers show up as `crib-myproject` in `docker ps`, not opaque hashes.

## Why

The [devcontainer spec](https://containers.dev/) is a good idea. A JSON file describes your development environment, and tooling builds a container from it. But the existing tools layer on complexity that gets in the way.

[DevPod](https://github.com/loft-sh/devpod) was the most promising open-source option: provider-agnostic, IDE-agnostic, well-designed. But it was built for a broader scope than most people need. Providers, agents injected into containers, SSH tunnels, gRPC, IDE integrations. For someone who just wants to `docker exec` into a container and use their terminal, that is a lot of moving parts between you and your shell.

Then [DevPod seems to be effectively abandoned](https://github.com/loft-sh/devpod/issues/1915) when Loft Labs shifted focus to vCluster. The project stopped receiving updates in April 2025, with no official statement and no path forward for the community.

`crib` takes a different approach: do less, but do it well. Read the devcontainer config, build the image, run the container, set up the user and lifecycle hooks, done. No agents, no SSH, no providers, no IDE assumptions. Just Docker (or Podman) doing what Docker does.

## Background

This isn't the first time [@fgrehm](https://github.com/fgrehm) has gone down this road. [vagrant-boxen](https://github.com/fgrehm/vagrant-boxen) (2013) tried to make Vagrant machines manageable without needing Puppet or Chef expertise. [Ventriloquist](https://fabiorehm.com/blog/2013/09/11/announcing-ventriloquist/) (2013) combined Vagrant and Docker to give developers portable, disposable dev VMs. [devstep](https://fabiorehm.com/blog/2014/08/26/devstep-development-environments-powered-by-docker-and-buildpacks/) (2014) took it further with "git clone, one command, hack" using Docker and Heroku-style buildpacks. The devcontainer spec has since standardized what that project was trying to achieve, so `crib` builds on that foundation instead of reinventing it.

The [experience of using DevPod as a terminal-first developer](https://fabiorehm.com/blog/2025/11/11/devpod-ssh-devcontainers/), treating devcontainers as remote machines you SSH into rather than IDE-managed environments, shaped many of `crib`'s design decisions. The pain points (broken git signing wrappers, unnecessary cache invalidation, port forwarding conflicts, agent-related complexity) all pointed toward the same conclusion: the simplest path is often the best one.
