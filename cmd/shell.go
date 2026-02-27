package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:     "shell",
	Aliases: []string{"sh"},
	Short:   "Open an interactive shell in the current workspace container",
	Long: `Open an interactive shell in the current workspace container.

The shell command automatically detects the best available shell
(zsh, bash, or sh in order of preference), sets the SHELL environment
variable, and starts it as a login shell inside the running container.
Working directory is set to the workspace folder if available.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		eng, ociDrv, store, err := newEngine()
		if err != nil {
			return err
		}

		ws, err := currentWorkspace(store, false)
		if err != nil {
			return err
		}

		status, err := eng.Status(cmd.Context(), ws)
		if err != nil {
			return fmt.Errorf("finding container: %w", err)
		}
		if status.Container == nil {
			return fmt.Errorf("no container found (run 'crib up' first)")
		}
		container := status.Container

		// Detect which shell is available in the container
		var buf bytes.Buffer
		detectionCmd := []string{"/bin/sh", "-c", "command -v zsh || command -v bash || command -v sh"}
		_ = ociDrv.ExecContainer(cmd.Context(), ws.ID, container.ID, detectionCmd, nil, &buf, nil, nil, "")
		shellPath := strings.TrimSpace(buf.String())
		if shellPath == "" {
			shellPath = "/bin/sh" // final fallback
		}

		runtimeBin, err := exec.LookPath(ociDrv.Runtime().String())
		if err != nil {
			return fmt.Errorf("finding container runtime: %w", err)
		}

		// Replace the current process with docker/podman exec.
		// Always allocate a pseudo-TTY (-t) since this is an interactive shell.
		execArgs := []string{runtimeBin, "exec", "-i", "-t"}

		// Set SHELL environment variable so the shell and its child processes
		// know which shell is running
		execArgs = append(execArgs, "-e", "SHELL="+shellPath)

		// Inject remoteEnv variables and set working directory from saved result.
		result, _ := store.LoadResult(ws.ID)
		if result != nil && result.RemoteUser != "" {
			execArgs = append(execArgs, "-u", result.RemoteUser)
		}
		execArgs = appendRemoteEnv(execArgs, result)
		if result != nil && result.WorkspaceFolder != "" {
			execArgs = append(execArgs, "-w", result.WorkspaceFolder)
		}

		execArgs = append(execArgs, container.ID, shellPath, "-l")

		return syscall.Exec(runtimeBin, execArgs, os.Environ())
	},
}
