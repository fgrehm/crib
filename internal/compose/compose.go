package compose

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

const (
	// ProjectLabel is the label used by Docker Compose to identify project containers.
	ProjectLabel = "com.docker.compose.project"

	// ServiceLabel is the label used by Docker Compose to identify service containers.
	ServiceLabel = "com.docker.compose.service"
)

// Helper wraps the compose CLI for executing compose commands.
type Helper struct {
	// baseCommand is the runtime command (e.g. "docker" or "podman").
	baseCommand string
	// argsPrefix is prepended to all compose commands (e.g. ["compose"]).
	argsPrefix []string
	// version is the detected compose version string.
	version string
	logger  *slog.Logger
}

// NewHelperFromRuntime creates a Helper with the given runtime command without
// probing for compose availability. Useful for cases where only the runtime
// identity is needed (e.g. checking if Podman is in use).
func NewHelperFromRuntime(runtimeCommand string) *Helper {
	return &Helper{baseCommand: runtimeCommand}
}

// NewHelper detects the compose CLI and returns a Helper.
// It probes `<runtimeCommand> compose version --short` to verify availability.
func NewHelper(runtimeCommand string, logger *slog.Logger) (*Helper, error) {
	// Try `<cmd> compose version --short`.
	// Stdout and stderr are captured separately so that warnings printed to
	// stderr (e.g. podman-compose's "backed by docker-compose" notice) don't
	// pollute the version string or the caller's terminal.
	cmd := exec.Command(runtimeCommand, "compose", "version", "--short")
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s compose not available: %w: %s", runtimeCommand, err, strings.TrimSpace(stderrBuf.String()))
	}

	// Take the first non-empty line in case the output contains extra lines.
	version := firstLine(stdoutBuf.String())
	logger.Info("detected compose", "command", runtimeCommand+" compose", "version", version)

	return &Helper{
		baseCommand: runtimeCommand,
		argsPrefix:  []string{"compose"},
		version:     version,
		logger:      logger,
	}, nil
}

// Version returns the detected compose version string.
func (h *Helper) Version() string {
	return h.version
}

// RuntimeCommand returns the base runtime command (e.g. "docker" or "podman").
func (h *Helper) RuntimeCommand() string {
	return h.baseCommand
}

// Run executes a compose command with the given args and I/O streams.
// extraEnv is appended to the current process environment for the subprocess.
func (h *Helper) Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, extraEnv []string) error {
	fullArgs := append(h.argsPrefix, args...)
	h.logger.Debug("exec compose", "cmd", h.baseCommand, "args", fullArgs)

	cmd := exec.CommandContext(ctx, h.baseCommand, fullArgs...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}

	var stderrBuf bytes.Buffer
	if stderr != nil {
		cmd.Stderr = io.MultiWriter(stderr, &stderrBuf)
	} else {
		cmd.Stderr = &stderrBuf
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s compose %v: %w: %s", h.baseCommand, args, err, stderrBuf.String())
	}
	return nil
}

// Build runs `compose build` for the given project.
// extraEnv is appended to the subprocess environment for variable substitution.
func (h *Helper) Build(ctx context.Context, projectName string, files []string, services []string, stdout, stderr io.Writer, extraEnv []string) error {
	args := projectArgs(projectName, files)
	args = append(args, "build")
	args = append(args, services...)
	return h.Run(ctx, args, nil, stdout, stderr, extraEnv)
}

// Up runs `compose up -d` for the given project.
// extraEnv is appended to the subprocess environment for variable substitution.
func (h *Helper) Up(ctx context.Context, projectName string, files []string, services []string, stdout, stderr io.Writer, extraEnv []string) error {
	args := projectArgs(projectName, files)
	args = append(args, "up", "-d")
	args = append(args, services...)
	return h.Run(ctx, args, nil, stdout, stderr, extraEnv)
}

// Stop runs `compose stop` for the given project.
// extraEnv is appended to the subprocess environment for variable substitution.
func (h *Helper) Stop(ctx context.Context, projectName string, files []string, stdout, stderr io.Writer, extraEnv []string) error {
	args := projectArgs(projectName, files)
	args = append(args, "stop")
	return h.Run(ctx, args, nil, stdout, stderr, extraEnv)
}

// Restart runs `compose restart` for the given project.
// extraEnv is appended to the subprocess environment for variable substitution.
func (h *Helper) Restart(ctx context.Context, projectName string, files []string, stdout, stderr io.Writer, extraEnv []string) error {
	args := projectArgs(projectName, files)
	args = append(args, "restart")
	return h.Run(ctx, args, nil, stdout, stderr, extraEnv)
}

// Down runs `compose down` for the given project.
// extraEnv is appended to the subprocess environment for variable substitution.
func (h *Helper) Down(ctx context.Context, projectName string, files []string, stdout, stderr io.Writer, extraEnv []string) error {
	args := projectArgs(projectName, files)
	args = append(args, "down")
	return h.Run(ctx, args, nil, stdout, stderr, extraEnv)
}

// ListContainers returns the container IDs for a compose project.
// Returns only the IDs without any filtering or parsing.
func (h *Helper) ListContainers(ctx context.Context, projectName string, files []string, extraEnv []string) ([]string, error) {
	args := projectArgs(projectName, files)
	args = append(args, "ps", "-q")

	cmd := exec.CommandContext(ctx, h.baseCommand, append(h.argsPrefix, args...)...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}

	// Capture stdout and stderr separately to avoid polluting the output with warnings.
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s compose %v: %w: %s", h.baseCommand, args, err, stderrBuf.String())
	}

	return parseLines(stdoutBuf.String()), nil
}

// ProjectName returns the compose project name for a workspace.
// It respects the COMPOSE_PROJECT_NAME env var, falling back to "crib-<wsID>".
func ProjectName(workspaceID string) string {
	if name := os.Getenv("COMPOSE_PROJECT_NAME"); name != "" {
		return name
	}
	return "crib-" + workspaceID
}

// parseLines splits output by newlines and removes empty strings.
func parseLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// firstLine returns the first non-empty line of s, trimmed of whitespace.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return strings.TrimSpace(s)
}

// projectArgs builds the common prefix args for a compose command:
// --project-name <name> [-f file1 -f file2 ...]
func projectArgs(projectName string, files []string) []string {
	args := []string{"--project-name", projectName}
	for _, f := range files {
		args = append(args, "-f", f)
	}
	return args
}
