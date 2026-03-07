---
applyTo: "internal/**,cmd/**"
---

# Logging and Output

Four output mechanisms, each with a distinct purpose:

| Mechanism | Audience | Controlled by |
|-----------|----------|---------------|
| `internal/ui` (stdout) | User: results and errors | always visible; `cmd/` layer only |
| Engine progress callback | User: operation status | always visible |
| Engine stdout/stderr writers | User: subprocess output | `-V` / `--verbose` |
| `log/slog` (stderr) | Developer diagnostics | `--debug` |

**slog levels**: `Debug` for exec commands and internal decisions; `Warn` for
non-fatal fallbacks; `Info` for one-time startup events only (runtime/compose
detection).

**`-V`** passes subprocess stdout through instead of discarding it. Does not
change the slog level. To echo a command in verbose mode, write it to the
engine's stderr writer, not slog.

**`--debug`** sets slog to Debug and also implies verbose.

Rules:
- Do not use slog in `cmd/` for user messages.
- Do not promote exec logging above `Debug` to fake verbose output.
- Do not hardcode `io.Discard` where the verbose flag should decide.
- Guard expensive log argument evaluation (like `scrubArgs`) behind
  `logger.Enabled(ctx, slog.LevelDebug)`.
