package driver

import (
	"io"
	"os"
	"strings"
)

// WriteEnvFile writes KEY=VALUE lines to path with 0600 permissions. Values
// are written verbatim (no quoting or interpolation), matching the env-file
// format accepted by docker and podman.
func WriteEnvFile(path string, env []string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return WriteEnvFileTo(f, env)
}

// WriteEnvFileTo writes KEY=VALUE lines to w, one per line, verbatim.
func WriteEnvFileTo(w io.Writer, env []string) error {
	var b strings.Builder
	for _, e := range env {
		b.WriteString(e)
		b.WriteByte('\n')
	}
	_, err := w.Write([]byte(b.String()))
	return err
}

// EnvFileSafe reports whether all env entries can be represented in an
// env-file for `docker run --env-file` / `podman run --env-file`. That parser
// is literal: only a leading `#` starts a comment and the rest of the line is
// taken verbatim, so the only unrepresentable value is one containing a
// newline (which would break the line structure).
func EnvFileSafe(env []string) bool {
	for _, e := range env {
		if _, v, ok := strings.Cut(e, "="); ok && strings.Contains(v, "\n") {
			return false
		}
	}
	return true
}

// EnvFileSafeForCompose reports whether all env entries can be represented in
// a compose `env_file:`. Compose's env-file parser is richer than docker run's:
// it strips surrounding quotes, trims leading/trailing whitespace, treats a
// space before `#` as an inline comment, and interpolates unquoted/double-quoted
// values. Values containing any of those constructs would be silently mutated,
// so they must fall back to the inline `environment:` block instead.
func EnvFileSafeForCompose(env []string) bool {
	for _, e := range env {
		_, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		if strings.Contains(v, "\n") {
			return false
		}
		if strings.ContainsAny(v, `"'`) {
			return false
		}
		if v != strings.TrimSpace(v) {
			return false
		}
		if strings.Contains(v, " #") {
			return false
		}
	}
	return true
}
