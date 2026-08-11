package driver

import (
	"fmt"
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

// WriteEnvTempFile writes env to a 0600 temp file suitable for `--env-file`
// and returns its path plus a cleanup func that removes it. It returns
// ("", nil, nil) when env is empty or contains a value the env-file parser
// would mutate (a newline), so the caller can fall back to -e flags. When
// path != "" the caller MUST call cleanup (typically via defer).
func WriteEnvTempFile(env []string) (path string, cleanup func(), err error) {
	if len(env) == 0 || !EnvFileSafe(env) {
		return "", nil, nil
	}
	f, err := os.CreateTemp("", "crib-env-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating env file: %w", err)
	}
	path = f.Name()
	if err := WriteEnvFileTo(f, env); err != nil {
		f.Close()
		os.Remove(path)
		return "", nil, fmt.Errorf("writing env file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", nil, fmt.Errorf("closing env file: %w", err)
	}
	return path, func() { os.Remove(path) }, nil
}

// EnvFileSafe reports whether all env entries can be represented in an
// env-file for `docker run/exec --env-file` / `podman run/exec --env-file`.
// That parser is literal: only a leading `#` starts a comment and the rest of
// the line is taken verbatim, so the only unrepresentable value is one
// containing a newline (which would break the line structure).
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
	if !EnvFileSafe(env) {
		return false
	}
	for _, e := range env {
		_, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
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
