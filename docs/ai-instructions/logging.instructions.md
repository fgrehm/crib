---
applyTo: "internal/**,cmd/**"
---

# Logging and Output

## Rules

- Use `internal/ui` for user-facing messages in `cmd/` (slog is for diagnostics).
- Keep exec logging at `Debug` level. Use the engine's stderr writer for verbose
  command echoing.
- Use `e.stdout`/`e.stderr` (verbose-aware writers) for subprocess output. These
  resolve to `io.Discard` when verbose is off.
- Guard expensive log argument evaluation (like `scrubArgs`) behind
  `logger.Enabled(ctx, slog.LevelDebug)`.

## Output mechanisms

| Mechanism | Audience | Controlled by |
|-----------|----------|---------------|
| `internal/ui` (stdout) | User: results and errors | always visible; `cmd/` layer only |
| Engine progress callback | User: operation status | always visible |
| Engine stdout/stderr writers | User: subprocess output | `--verbose` |
| `log/slog` (stderr) | Developer diagnostics | `--debug` |

**slog levels**: `Debug` for exec commands and internal decisions; `Warn` for
non-fatal fallbacks; `Info` for one-time startup events only (runtime/compose
detection).

**`--verbose`** passes subprocess stdout through. Does not change the slog level.

**`--debug`** sets slog to Debug and also implies verbose.
