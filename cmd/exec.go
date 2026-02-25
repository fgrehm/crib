package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec [-- cmd...]",
	Short: "Execute a command in the current workspace container",
	Long: `Execute a command in the current workspace container.

Use -- to separate crib flags from the container command:
  crib exec -- bash
  crib exec -- bash -c "echo hello"`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		eng, ociDrv, store, err := newEngine()
		if err != nil {
			return err
		}

		ws, err := currentWorkspace(store, false)
		if err != nil {
			return err
		}

		container, err := eng.Status(cmd.Context(), ws)
		if err != nil {
			return fmt.Errorf("finding container: %w", err)
		}
		if container == nil {
			return fmt.Errorf("no container found (run 'crib up' first)")
		}

		shellArgs := args
		if len(shellArgs) == 0 {
			shellArgs = []string{"/bin/sh"}
		}

		runtimeBin, err := exec.LookPath(ociDrv.Runtime().String())
		if err != nil {
			return fmt.Errorf("finding container runtime: %w", err)
		}

		// Replace the current process with docker/podman exec.
		// Only allocate a pseudo-TTY (-t) when stdin is an interactive terminal;
		// omitting -t allows non-interactive use (pipes, scripts, tests).
		execArgs := []string{runtimeBin, "exec", "-i"}
		if stdinIsTerminal() {
			execArgs = append(execArgs, "-t")
		}

		// Inject remoteEnv variables (before user-specified --env so user flags take precedence).
		result, _ := store.LoadResult(ws.ID)

		// Determine user: explicit --user flag takes precedence, then remoteUser from config.
		user, _ := cmd.Flags().GetString("user")
		if user == "" && result != nil {
			user = result.RemoteUser
		}
		if user != "" {
			execArgs = append(execArgs, "-u", user)
		}

		// Add workdir: explicit flag takes precedence, otherwise use workspace folder.
		workdir, _ := cmd.Flags().GetString("workdir")
		if workdir == "" && result != nil && result.WorkspaceFolder != "" {
			workdir = result.WorkspaceFolder
		}
		if workdir != "" {
			execArgs = append(execArgs, "-w", workdir)
		}
		execArgs = appendRemoteEnv(execArgs, result)

		// Add env vars if provided
		envVars, _ := cmd.Flags().GetStringSlice("env")
		for _, envVar := range envVars {
			execArgs = append(execArgs, "-e", envVar)
		}

		// Add env file if provided
		envFiles, _ := cmd.Flags().GetStringSlice("env-file")
		for _, envFile := range envFiles {
			execArgs = append(execArgs, "--env-file", envFile)
		}

		// Add privileged flag if set
		privileged, _ := cmd.Flags().GetBool("privileged")
		if privileged {
			execArgs = append(execArgs, "--privileged")
		}

		execArgs = append(execArgs, container.ID)
		execArgs = append(execArgs, shellArgs...)

		return syscall.Exec(runtimeBin, execArgs, os.Environ())
	},
}

func init() {
	execCmd.Flags().StringP("user", "u", "", "Username or UID (format: \"<name|uid>[:<group|gid>]\")")
	execCmd.Flags().StringP("workdir", "w", "", "Working directory inside the container")
	execCmd.Flags().StringSliceP("env", "e", nil, "Set environment variables")
	execCmd.Flags().StringSlice("env-file", nil, "Read in a file of environment variables")
	execCmd.Flags().Bool("privileged", false, "Give extended privileges to the command")
}

// stdinIsTerminal reports whether stdin is an interactive terminal.
func stdinIsTerminal() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
