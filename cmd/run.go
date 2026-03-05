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

var runCmd = &cobra.Command{
	Use:   "run [-- cmd...]",
	Short: "Run a command in the container through a login shell",
	Long: `Run a command in the workspace container through a login shell.

Unlike exec, run wraps your command in a login shell so that shell init
files (.zshrc, .bashrc, .profile) are sourced first. This makes tools
installed by version managers (mise, asdf, nvm, rbenv) available on PATH.

Use -- to separate crib flags from the container command:
  crib run -- ruby -v
  crib run -- bundle install
  crib run -- npm test`,
	Args: cobra.MinimumNArgs(1),
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

		// Detect the user's shell in the container (same logic as crib shell).
		var buf bytes.Buffer
		detectionCmd := []string{"/bin/sh", "-c", "command -v zsh || command -v bash || command -v sh"}
		_ = ociDrv.ExecContainer(cmd.Context(), ws.ID, container.ID, detectionCmd, nil, &buf, nil, nil, "")
		shellPath := strings.TrimSpace(buf.String())
		if shellPath == "" {
			shellPath = "/bin/sh"
		}

		runtimeBin, err := exec.LookPath(ociDrv.Runtime().String())
		if err != nil {
			return fmt.Errorf("finding container runtime: %w", err)
		}

		execArgs := []string{runtimeBin, "exec"}
		if stdinIsTerminal() {
			execArgs = append(execArgs, "-i", "-t")
		}

		result, _ := store.LoadResult(ws.ID)

		user, _ := cmd.Flags().GetString("user")
		if user == "" && result != nil {
			user = result.RemoteUser
		}
		if user != "" {
			execArgs = append(execArgs, "-u", user)
		}

		workdir, _ := cmd.Flags().GetString("workdir")
		if workdir == "" && result != nil && result.WorkspaceFolder != "" {
			workdir = result.WorkspaceFolder
		}
		if workdir != "" {
			execArgs = append(execArgs, "-w", workdir)
		}
		execArgs = appendRemoteEnv(execArgs, result)

		envVars, _ := cmd.Flags().GetStringSlice("env")
		for _, envVar := range envVars {
			execArgs = append(execArgs, "-e", envVar)
		}

		// Wrap the user's command in a login shell: $SHELL -lc 'cmd arg1 arg2'
		escaped := shellQuoteJoin(args)
		execArgs = append(execArgs, container.ID, shellPath, "-lc", escaped)

		return syscall.Exec(runtimeBin, execArgs, os.Environ())
	},
}

// shellQuoteJoin joins args into a single shell-safe string.
// Each argument is single-quoted with internal single quotes escaped.
func shellQuoteJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		// Replace ' with '\'' (end quote, escaped quote, start quote).
		quoted[i] = "'" + strings.ReplaceAll(a, "'", "'\\''") + "'"
	}
	return strings.Join(quoted, " ")
}

func init() {
	runCmd.Flags().StringP("user", "u", "", "Username or UID (format: \"<name|uid>[:<group|gid>]\")")
	runCmd.Flags().StringP("workdir", "w", "", "Working directory inside the container")
	runCmd.Flags().StringSliceP("env", "e", nil, "Set environment variables")
}
