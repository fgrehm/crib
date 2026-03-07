---
applyTo: "docs/**,website/**,CHANGELOG.md,README.md"
---

# Documentation and Naming

## Naming conventions

- **`devcontainer`** (one word, lowercase) for files, directories, and config
  references: `devcontainer.json`, `.devcontainer/`, `devcontainer-feature.json`.
- **"dev container"** (two words, lowercase) for the generic concept: "start a
  dev container", "your dev container environment".
- **"DevContainer Features"** and **"DevContainer Spec"** (PascalCase) when
  referring to these as proper nouns from the specification.
- **`crib exec --`** always use `--` separator in docs examples before the
  command to run.
- Reserve **"Dev Containers"** (capitalized, two words) for the VS Code extension
  product name only.

## Docs site workflow

Canonical docs live in `docs/` and are symlinked into the website for publishing.
When adding a new doc:

1. Create the file in `docs/`.
2. Symlink into the website: `ln -s ../../../../../docs/<file>.md website/src/content/docs/<section>/<file>.md`
3. Add a sidebar entry in `website/astro.config.mjs`.

## Changelog

`CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
format. Update `[Unreleased]` for user-facing changes. Internal refactors that
preserve behavior need no entry.

At release time, entries move to a versioned section. The CI release workflow
intentionally fails if no release notes exist for the tagged version; this is by
design.

Mirror the version section into `website/src/content/docs/reference/changelog.md`
(no `[Unreleased]` on the site; version headers link to GitHub releases).

## Troubleshooting

`docs/troubleshooting.md` collects common issues. Add entries there even when the
root cause is outside crib (base image permissions, runtime quirks). Users look
for help in crib's docs first.
