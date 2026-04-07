package dotfiles

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/fgrehm/crib/internal/globalconfig"
	"github.com/fgrehm/crib/internal/plugin"
)

// fakeExec records commands that were executed.
type fakeExec struct {
	calls []fakeExecCall
}

type fakeExecCall struct {
	cmd     []string
	user    string
	workDir string
}

func (f *fakeExec) fn(_ context.Context, cmd []string, user string, workDir string) ([]byte, error) {
	f.calls = append(f.calls, fakeExecCall{cmd: cmd, user: user, workDir: workDir})
	// "test -d" should fail by default (directory doesn't exist yet).
	if len(cmd) >= 2 && cmd[0] == "test" && cmd[1] == "-d" {
		return nil, &fakeError{}
	}
	return nil, nil
}

func (f *fakeExec) streamFn(_ context.Context, cmd []string, user string, workDir string, _, _ io.Writer) error {
	f.calls = append(f.calls, fakeExecCall{cmd: cmd, user: user, workDir: workDir})
	return nil
}

func (f *fakeExec) request(user string) *plugin.PostContainerCreateRequest {
	return &plugin.PostContainerCreateRequest{
		RemoteUser: user,
		Exec:       f.fn,
		StreamExec: f.streamFn,
	}
}

func TestPostContainerCreate_NoRepository_Noop(t *testing.T) {
	p := New(globalconfig.DotfilesConfig{})
	exec := &fakeExec{}

	_, err := p.PostContainerCreate(context.Background(), exec.request("vscode"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 0 {
		t.Errorf("expected no exec calls, got %d", len(exec.calls))
	}
}

func TestPostContainerCreate_NoGit_Skips(t *testing.T) {
	p := New(globalconfig.DotfilesConfig{
		Repository: "https://github.com/user/dotfiles",
	})

	execFn := func(_ context.Context, cmd []string, _ string, _ string) ([]byte, error) {
		if cmd[0] == "which" {
			return nil, &fakeError{}
		}
		t.Fatal("should not exec anything after which fails")
		return nil, nil
	}

	_, err := p.PostContainerCreate(context.Background(), &plugin.PostContainerCreateRequest{
		RemoteUser: "vscode",
		Exec:       execFn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPostContainerCreate_ClonesRepository(t *testing.T) {
	p := New(globalconfig.DotfilesConfig{
		Repository: "https://github.com/user/dotfiles",
	})
	exec := &fakeExec{}

	_, err := p.PostContainerCreate(context.Background(), exec.request("vscode"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Call order: which git (0), test -d (1), git clone (2).
	if len(exec.calls) < 3 {
		t.Fatalf("expected at least 3 exec calls, got %d", len(exec.calls))
	}

	cloneCmd := strings.Join(exec.calls[2].cmd, " ")
	if !strings.Contains(cloneCmd, "git clone") {
		t.Errorf("expected git clone command, got: %s", cloneCmd)
	}
	if !strings.Contains(cloneCmd, "https://github.com/user/dotfiles") {
		t.Errorf("expected repository URL in command, got: %s", cloneCmd)
	}
	// Default targetPath is ~/dotfiles -> /home/vscode/dotfiles.
	if !strings.Contains(cloneCmd, "/home/vscode/dotfiles") {
		t.Errorf("expected default target path, got: %s", cloneCmd)
	}
	if exec.calls[2].user != "vscode" {
		t.Errorf("expected user vscode, got %s", exec.calls[2].user)
	}
}

func TestPostContainerCreate_SSHRepo_AcceptsNewHostKey(t *testing.T) {
	p := New(globalconfig.DotfilesConfig{
		Repository: "git@github.com:user/dotfiles",
	})
	exec := &fakeExec{}

	_, err := p.PostContainerCreate(context.Background(), exec.request("vscode"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Clone via SSH should use sh -c with GIT_SSH_COMMAND.
	cloneCmd := strings.Join(exec.calls[2].cmd, " ")
	if !strings.Contains(cloneCmd, "StrictHostKeyChecking=accept-new") {
		t.Errorf("expected StrictHostKeyChecking=accept-new for SSH repo, got: %s", cloneCmd)
	}
	if !strings.Contains(cloneCmd, "git@github.com:user/dotfiles") {
		t.Errorf("expected repository URL in command, got: %s", cloneCmd)
	}
}

func TestPostContainerCreate_CustomTargetPath(t *testing.T) {
	p := New(globalconfig.DotfilesConfig{
		Repository: "https://github.com/user/dotfiles",
		TargetPath: "~/my-dotfiles",
	})
	exec := &fakeExec{}

	_, err := p.PostContainerCreate(context.Background(), exec.request("vscode"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cloneCmd := strings.Join(exec.calls[2].cmd, " ")
	if !strings.Contains(cloneCmd, "/home/vscode/my-dotfiles") {
		t.Errorf("expected custom target path with tilde expanded, got: %s", cloneCmd)
	}
}

func TestPostContainerCreate_RootUser(t *testing.T) {
	p := New(globalconfig.DotfilesConfig{
		Repository: "https://github.com/user/dotfiles",
	})
	exec := &fakeExec{}

	_, err := p.PostContainerCreate(context.Background(), exec.request("root"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cloneCmd := strings.Join(exec.calls[2].cmd, " ")
	if !strings.Contains(cloneCmd, "/root/dotfiles") {
		t.Errorf("expected root home path, got: %s", cloneCmd)
	}
	if exec.calls[2].user != "root" {
		t.Errorf("expected user root, got %s", exec.calls[2].user)
	}
}

func TestPostContainerCreate_AutoDetectsInstallScript(t *testing.T) {
	// When no installCommand is set, the plugin checks for common scripts.
	// The exec call for test -f <script> returns nil (success) for install.sh.
	p := New(globalconfig.DotfilesConfig{
		Repository: "https://github.com/user/dotfiles",
	})

	execFn := func(_ context.Context, cmd []string, _ string, _ string) ([]byte, error) {
		cmdStr := strings.Join(cmd, " ")
		// which git succeeds.
		if cmd[0] == "which" {
			return nil, nil
		}
		// test -d (directory exists check) fails (not cloned yet).
		if len(cmd) >= 2 && cmd[0] == "test" && cmd[1] == "-d" {
			return nil, &fakeError{}
		}
		// For test -f checks, succeed on install.sh only.
		if strings.Contains(cmdStr, "test -f") && strings.Contains(cmdStr, "install.sh") {
			return nil, nil
		}
		// Other test -f checks fail.
		if strings.Contains(cmdStr, "test -f") {
			return nil, &fakeError{}
		}
		return nil, nil
	}
	streamFn := func(_ context.Context, cmd []string, _ string, _ string, _, _ io.Writer) error {
		cmdStr := strings.Join(cmd, " ")
		// git clone and install script execution succeed.
		if strings.Contains(cmdStr, "git clone") || strings.Contains(cmdStr, "install.sh") {
			return nil
		}
		return nil
	}

	_, err := p.PostContainerCreate(context.Background(), &plugin.PostContainerCreateRequest{
		RemoteUser: "vscode",
		Exec:       execFn,
		StreamExec: streamFn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPostContainerCreate_InstallCommandOverride(t *testing.T) {
	p := New(globalconfig.DotfilesConfig{
		Repository:     "https://github.com/user/dotfiles",
		InstallCommand: "make install",
	})
	exec := &fakeExec{}

	_, err := p.PostContainerCreate(context.Background(), exec.request("vscode"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have which git (exec) + test -d (exec) + clone (stream) + install (stream).
	if len(exec.calls) != 4 {
		t.Fatalf("expected 4 exec calls (which + test -d + clone + install), got %d", len(exec.calls))
	}

	installCmd := strings.Join(exec.calls[3].cmd, " ")
	if !strings.Contains(installCmd, "make install") {
		t.Errorf("expected install command override, got: %s", installCmd)
	}
	// Install should run in the target directory.
	if exec.calls[3].workDir != "/home/vscode/dotfiles" {
		t.Errorf("expected workDir /home/vscode/dotfiles, got %s", exec.calls[3].workDir)
	}
}

func TestPostContainerCreate_AbsoluteTargetPath(t *testing.T) {
	p := New(globalconfig.DotfilesConfig{
		Repository: "https://github.com/user/dotfiles",
		TargetPath: "/opt/dotfiles",
	})
	exec := &fakeExec{}

	_, err := p.PostContainerCreate(context.Background(), exec.request("vscode"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cloneCmd := strings.Join(exec.calls[2].cmd, " ")
	if !strings.Contains(cloneCmd, "/opt/dotfiles") {
		t.Errorf("expected absolute target path, got: %s", cloneCmd)
	}
}

func TestPostContainerCreate_SkipsCloneWhenTargetExists(t *testing.T) {
	p := New(globalconfig.DotfilesConfig{
		Repository: "https://github.com/user/dotfiles",
	})

	execFn := func(_ context.Context, cmd []string, _ string, _ string) ([]byte, error) {
		if cmd[0] == "which" {
			return nil, nil
		}
		// test -d succeeds: directory already exists.
		if len(cmd) >= 2 && cmd[0] == "test" && cmd[1] == "-d" {
			return nil, nil
		}
		t.Fatalf("unexpected exec call: %v", cmd)
		return nil, nil
	}

	var streamCalls int
	streamFn := func(_ context.Context, _ []string, _ string, _ string, _, _ io.Writer) error {
		streamCalls++
		return nil
	}

	_, err := p.PostContainerCreate(context.Background(), &plugin.PostContainerCreateRequest{
		RemoteUser: "vscode",
		Exec:       execFn,
		StreamExec: streamFn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if streamCalls > 0 {
		t.Error("should not clone or run install when target directory already exists")
	}
}

func TestIsSSHRepo(t *testing.T) {
	tests := []struct {
		repo string
		want bool
	}{
		{"git@github.com:user/repo.git", true},
		{"ssh://git@github.com/user/repo.git", true},
		{"https://github.com/user/repo.git", false},
		{"https://user@github.com/repo.git", false},
		{"/local/path", false},
	}
	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			got := isSSHRepo(tt.repo)
			if got != tt.want {
				t.Errorf("isSSHRepo(%q) = %v, want %v", tt.repo, got, tt.want)
			}
		})
	}
}

// findCloneCall returns the first call whose cmd contains "git" and "clone".
func findCloneCall(calls []fakeExecCall) (fakeExecCall, bool) {
	for _, c := range calls {
		hasGit, hasClone := false, false
		for _, arg := range c.cmd {
			if arg == "git" {
				hasGit = true
			}
			if arg == "clone" {
				hasClone = true
			}
		}
		if hasGit && hasClone {
			return c, true
		}
	}
	return fakeExecCall{}, false
}

func TestPostContainerCreate_SSHCloneCmd_ExecAtSign(t *testing.T) {
	p := New(globalconfig.DotfilesConfig{
		Repository: "git@github.com:user/dotfiles",
	})
	exec := &fakeExec{}

	_, err := p.PostContainerCreate(context.Background(), exec.request("vscode"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cloneCall, ok := findCloneCall(exec.calls)
	if !ok {
		t.Fatal("no clone call found")
	}
	// Layout: sh -c <script> -- git clone -- <repo> <target> (8 elements minimum)
	if len(cloneCall.cmd) < 8 {
		t.Fatalf("expected at least 8 elements in clone cmd, got %d: %v", len(cloneCall.cmd), cloneCall.cmd)
	}
	// Must use exec "$@" pattern, not string interpolation.
	if cloneCall.cmd[0] != "sh" || cloneCall.cmd[1] != "-c" {
		t.Errorf("expected sh -c at [0:2], got %v", cloneCall.cmd[:2])
	}
	if !strings.Contains(cloneCall.cmd[2], `exec "$@"`) {
		t.Errorf("expected exec \"$@\" in shell script, got: %q", cloneCall.cmd[2])
	}
	if cloneCall.cmd[3] != "--" {
		t.Errorf("expected -- at [3], got %q", cloneCall.cmd[3])
	}
	if cloneCall.cmd[4] != "git" || cloneCall.cmd[5] != "clone" {
		t.Errorf("expected git clone at [4:6], got %v", cloneCall.cmd[4:6])
	}
	// [6] is the -- end-of-options separator for git clone itself
	if cloneCall.cmd[6] != "--" {
		t.Errorf("expected -- at [6] (git clone end-of-options), got %q", cloneCall.cmd[6])
	}
	if cloneCall.cmd[7] != "git@github.com:user/dotfiles" {
		t.Errorf("expected repo as positional arg at [7], got %q", cloneCall.cmd[7])
	}
}

func TestPostContainerCreate_SSHCloneCmd_SingleQuoteInRepo(t *testing.T) {
	// A repo name containing a single quote must not break the shell command.
	p := New(globalconfig.DotfilesConfig{
		Repository: "git@github.com:user/it's-fine.git",
	})
	exec := &fakeExec{}

	_, err := p.PostContainerCreate(context.Background(), exec.request("vscode"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cloneCall, ok := findCloneCall(exec.calls)
	if !ok {
		t.Fatal("no clone call found")
	}
	if len(cloneCall.cmd) < 8 {
		t.Fatalf("expected at least 8 elements in clone cmd, got %d: %v", len(cloneCall.cmd), cloneCall.cmd)
	}
	// The repo must appear verbatim as a positional arg (no shell quoting needed).
	if cloneCall.cmd[7] != "git@github.com:user/it's-fine.git" {
		t.Errorf("repo with single quote should be passed verbatim, got %q", cloneCall.cmd[7])
	}
}

// fakeError implements error for simulating exec failures.
type fakeError struct{}

func (e *fakeError) Error() string { return "fake error" }
