#!/usr/bin/env bash
# Generates website/src/content/docs/reference/changelog.md from CHANGELOG.md.
# Strips [Unreleased], adds GitHub release tag links to version headers.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$REPO_ROOT/website/src/content/docs/reference/changelog.md"

cat > "$OUT" <<'FRONTMATTER'
---
title: CHANGELOG
description: All notable changes to crib.
---

All notable changes to this project will be documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

FRONTMATTER

sed -n '/^## \[0/,$p' "$REPO_ROOT/CHANGELOG.md" \
  | sed 's|^## \[\([0-9][^]]*\)\] - \(.*\)|## [\1](https://github.com/fgrehm/crib/releases/tag/v\1) - \2|' \
  >> "$OUT"
