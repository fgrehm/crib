package gitssh

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/fgrehm/crib/internal/plugin"
)

// Name returns the plugin's display name.
func Name() string {
	return "git-ssh-signing"
}

// PreContainerRun reads the host's global git configuration and generates a
// minimal .gitconfig for the container. It copies SSH signing settings when
// present, and always copies user.name / user.email when available so that
// git commits work without additional setup.
func PreContainerRun(ctx context.Context) (*plugin.PreContainerRunResponse, error) {
	raw, err := readHostGitConfig(ctx)
	if err != nil {
		slog.DebugContext(ctx, "git-ssh-signing: cannot read host gitconfig", "error", err)
		return nil, nil
	}

	vals, err := parseGitConfig(raw)
	if err != nil {
		slog.WarnContext(ctx, "git-ssh-signing: cannot parse gitconfig", "error", err)
		return nil, nil
	}

	// Generate a container config when there is anything worth copying:
	// SSH signing, or a user identity.
	useSSH := vals["gpg.format"] == "ssh"
	if !useSSH && vals["user.name"] == "" && vals["user.email"] == "" {
		slog.DebugContext(ctx, "git-ssh-signing: no signing or identity on host")
		return nil, nil
	}

	content := buildConfigContent(vals, useSSH)

	resp := &plugin.PreContainerRunResponse{}
	resp.FileCopies = append(resp.FileCopies, plugin.FileCopy{
		Content: []byte(content),
		Target:  "/etc/gitconfig",
	})

	return resp, nil
}

// readHostGitConfig retrieves the host's global git configuration.
// It first tries `git config --list --global`; if that fails it falls back
// to reading $HOME/.gitconfig directly.
func readHostGitConfig(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "config", "--list", "--global")
	out, err := cmd.Output()
	if err == nil {
		return string(out), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(home + "/.gitconfig")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// parseGitConfig parses git config output (key=value lines) into a map.
func parseGitConfig(raw string) (map[string]string, error) {
	vals := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		vals[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return vals, nil
}

// buildConfigContent renders a minimal gitconfig file. The [gpg] section is
// only included when the host uses SSH-based signing.
func buildConfigContent(vals map[string]string, useSSH bool) string {
	var b strings.Builder
	b.WriteString("[user]\n")
	if name, ok := vals["user.name"]; ok {
		b.WriteString("\tname = " + name + "\n")
	}
	if email, ok := vals["user.email"]; ok {
		b.WriteString("\temail = " + email + "\n")
	}
	if useSSH {
		b.WriteString("[gpg]\n")
		b.WriteString("\tformat = ssh\n")
		if program, ok := vals["gpg.ssh.program"]; ok {
			b.WriteString("\tsshProgram = " + program + "\n")
		}
		if keyFile, ok := vals["gpg.ssh.key-file"]; ok {
			b.WriteString("\tsshKeyFilePath = " + keyFile + "\n")
		}
	}
	return b.String()
}
