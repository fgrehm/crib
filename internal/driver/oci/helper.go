package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
)

// Helper wraps the docker/podman CLI binary for executing commands.
type Helper struct {
	command string
	logger  *slog.Logger
}

// NewHelper creates a Helper that shells out to the given command (e.g. "docker" or "podman").
func NewHelper(command string, logger *slog.Logger) *Helper {
	return &Helper{
		command: command,
		logger:  logger,
	}
}

// Command returns the base command name (e.g. "docker" or "podman").
func (h *Helper) Command() string {
	return h.command
}

// Run executes the command with the given args and attached I/O streams.
// If the command exits non-zero, the returned error includes captured stderr.
func (h *Helper) Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	h.logger.Debug("exec", "cmd", h.command, "args", args)

	cmd := exec.CommandContext(ctx, h.command, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout

	// Capture stderr for error messages while also writing to the caller's stderr.
	var stderrBuf bytes.Buffer
	if stderr != nil {
		cmd.Stderr = io.MultiWriter(stderr, &stderrBuf)
	} else {
		cmd.Stderr = &stderrBuf
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %v: %w: %s", h.command, scrubArgs(args), err, stderrBuf.String())
	}
	return nil
}

// sensitiveKeys contains substrings that identify env var names whose values
// should be redacted from error messages.
var sensitiveKeys = []string{
	"TOKEN", "SECRET", "KEY", "PASSWORD", "PASSPHRASE",
	"CREDENTIAL", "AUTH_SOCK",
}

// scrubArgs returns a copy of args with sensitive -e VAR=VALUE pairs redacted.
// Only the value is replaced; the variable name is preserved for debugging.
func scrubArgs(args []string) []string {
	result := make([]string, len(args))
	copy(result, args)
	for i, arg := range result {
		// Look for env var values: the arg after "-e" or args containing "=".
		if i > 0 && args[i-1] == "-e" {
			if k, _, ok := strings.Cut(arg, "="); ok && isSensitiveKey(k) {
				result[i] = k + "=***"
			}
		}
	}
	return result
}

// isSensitiveKey returns true if the env var name contains a sensitive substring.
func isSensitiveKey(name string) bool {
	upper := strings.ToUpper(name)
	for _, key := range sensitiveKeys {
		if strings.Contains(upper, key) {
			return true
		}
	}
	return false
}

// Output executes the command and returns captured stdout.
func (h *Helper) Output(ctx context.Context, args ...string) ([]byte, error) {
	var stdout bytes.Buffer
	if err := h.Run(ctx, args, nil, &stdout, nil); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

// Inspect runs `<cmd> inspect --type <inspectType>` on the given IDs and unmarshals
// the JSON result into the provided pointer.
func (h *Helper) Inspect(ctx context.Context, ids []string, inspectType string, result any) error {
	args := []string{"inspect", "--type", inspectType}
	args = append(args, ids...)

	out, err := h.Output(ctx, args...)
	if err != nil {
		return err
	}
	return json.Unmarshal(out, result)
}
