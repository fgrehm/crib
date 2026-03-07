---
applyTo: "docs/**,website/**,CHANGELOG.md,README.md"
---

# Documentation and Naming

## Naming conventions

- **`devcontainer`** (one word, lowercase) for files, directories, and config
  references: `devcontainer.json`, `.devcontainer/`, `devcontainer-feature.json`.
- **"dev container"** (two words, lowercase) for the generic concept: "start a
  dev container", "your dev container environment".
- **"DevContainer Features"** (PascalCase, proper noun) for the spec's Feature
  system. Same for "DevContainer Spec" when referring to the specification.
- **`crib exec --`** always use `--` separator in docs examples before the
  command to run.
- Never use "Dev Containers" (space, both capitalized) unless referring to the
  VS Code extension by its product name.

## Docs site workflow

Canonical docs live in `docs/` and are symlinked into the website for publishing.
When adding a new doc:

1. Create the file in `docs/`.
2. Symlink into the website: `ln -s ../../../../../docs/<file>.md website/src/content/docs/<section>/<file>.md`
3. Add a sidebar entry in `website/astro.config.mjs`.

## Changelog

`CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
format. Update `[Unreleased]` for user-facing changes. Internal refactors don't
need entries.

At release time, entries move to a versioned section. The CI release workflow
intentionally fails if no release notes exist for the tagged version; this is by
design.

When releasing, also mirror the version section into
`website/src/content/docs/reference/changelog.md` (no `[Unreleased]` section on
the site; version headers are links to GitHub releases).

## Troubleshooting

`docs/troubleshooting.md` collects common issues. Add entries there even when the
root cause is outside crib (base image permissions, runtime quirks). Users look
for help in crib's docs first.
