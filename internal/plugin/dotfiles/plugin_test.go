package dotfiles

import (
	"context"
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
	return nil, nil
}

func TestPostContainerCreate_NoRepository_Noop(t *testing.T) {
	p := New(globalconfig.DotfilesConfig{})
	exec := &fakeExec{}

	_, err := p.PostContainerCreate(context.Background(), &plugin.PostContainerCreateRequest{
		RemoteUser: "vscode",
		Exec:       exec.fn,
	})
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

	exec := func(_ context.Context, cmd []string, _ string, _ string) ([]byte, error) {
		if cmd[0] == "which" {
			return nil, &fakeError{}
		}
		t.Fatal("should not exec anything after which fails")
		return nil, nil
	}

	_, err := p.PostContainerCreate(context.Background(), &plugin.PostContainerCreateRequest{
		RemoteUser: "vscode",
		Exec:       exec,
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

	_, err := p.PostContainerCreate(context.Background(), &plugin.PostContainerCreateRequest{
		RemoteUser: "vscode",
		Exec:       exec.fn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(exec.calls) == 0 {
		t.Fatal("expected at least one exec call for clone")
	}

	// First call is which git, second is git clone.
	cloneCmd := strings.Join(exec.calls[1].cmd, " ")
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
	if exec.calls[1].user != "vscode" {
		t.Errorf("expected user vscode, got %s", exec.calls[1].user)
	}
}

func TestPostContainerCreate_CustomTargetPath(t *testing.T) {
	p := New(globalconfig.DotfilesConfig{
		Repository: "https://github.com/user/dotfiles",
		TargetPath: "~/my-dotfiles",
	})
	exec := &fakeExec{}

	_, err := p.PostContainerCreate(context.Background(), &plugin.PostContainerCreateRequest{
		RemoteUser: "vscode",
		Exec:       exec.fn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cloneCmd := strings.Join(exec.calls[1].cmd, " ")
	if !strings.Contains(cloneCmd, "/home/vscode/my-dotfiles") {
		t.Errorf("expected custom target path with tilde expanded, got: %s", cloneCmd)
	}
}

func TestPostContainerCreate_RootUser(t *testing.T) {
	p := New(globalconfig.DotfilesConfig{
		Repository: "https://github.com/user/dotfiles",
	})
	exec := &fakeExec{}

	_, err := p.PostContainerCreate(context.Background(), &plugin.PostContainerCreateRequest{
		RemoteUser: "root",
		Exec:       exec.fn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cloneCmd := strings.Join(exec.calls[1].cmd, " ")
	if !strings.Contains(cloneCmd, "/root/dotfiles") {
		t.Errorf("expected root home path, got: %s", cloneCmd)
	}
	if exec.calls[1].user != "root" {
		t.Errorf("expected user root, got %s", exec.calls[1].user)
	}
}

func TestPostContainerCreate_AutoDetectsInstallScript(t *testing.T) {
	// When no installCommand is set, the plugin checks for common scripts.
	// The exec call for test -f <script> returns nil (success) for install.sh.
	p := New(globalconfig.DotfilesConfig{
		Repository: "https://github.com/user/dotfiles",
	})

	exec := func(_ context.Context, cmd []string, _ string, _ string) ([]byte, error) {
		cmdStr := strings.Join(cmd, " ")
		// which git and git clone succeed.
		if cmd[0] == "which" || strings.Contains(cmdStr, "git clone") {
			return nil, nil
		}
		// For test -f checks, succeed on install.sh only.
		if strings.Contains(cmdStr, "test -f") && strings.Contains(cmdStr, "install.sh") {
			return nil, nil
		}
		// Other test -f checks fail.
		if strings.Contains(cmdStr, "test -f") {
			return nil, &fakeError{}
		}
		// The actual install.sh execution.
		return nil, nil
	}

	_, err := p.PostContainerCreate(context.Background(), &plugin.PostContainerCreateRequest{
		RemoteUser: "vscode",
		Exec:       exec,
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

	var calls []fakeExecCall
	exec := func(_ context.Context, cmd []string, user string, workDir string) ([]byte, error) {
		calls = append(calls, fakeExecCall{cmd: cmd, user: user, workDir: workDir})
		return nil, nil
	}

	_, err := p.PostContainerCreate(context.Background(), &plugin.PostContainerCreateRequest{
		RemoteUser: "vscode",
		Exec:       exec,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have which git + clone + install command (no test -f probing).
	if len(calls) != 3 {
		t.Fatalf("expected 3 exec calls (which + clone + install), got %d", len(calls))
	}

	installCmd := strings.Join(calls[2].cmd, " ")
	if !strings.Contains(installCmd, "make install") {
		t.Errorf("expected install command override, got: %s", installCmd)
	}
	// Install should run in the target directory.
	if calls[2].workDir != "/home/vscode/dotfiles" {
		t.Errorf("expected workDir /home/vscode/dotfiles, got %s", calls[2].workDir)
	}
}

func TestPostContainerCreate_AbsoluteTargetPath(t *testing.T) {
	p := New(globalconfig.DotfilesConfig{
		Repository: "https://github.com/user/dotfiles",
		TargetPath: "/opt/dotfiles",
	})
	exec := &fakeExec{}

	_, err := p.PostContainerCreate(context.Background(), &plugin.PostContainerCreateRequest{
		RemoteUser: "vscode",
		Exec:       exec.fn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cloneCmd := strings.Join(exec.calls[1].cmd, " ")
	if !strings.Contains(cloneCmd, "/opt/dotfiles") {
		t.Errorf("expected absolute target path, got: %s", cloneCmd)
	}
}

// fakeError implements error for simulating exec failures.
type fakeError struct{}

func (e *fakeError) Error() string { return "fake error" }
