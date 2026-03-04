package packagecache

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgrehm/crib/internal/plugin"
)

func TestPlugin_SingleProvider(t *testing.T) {
	p := New([]string{"npm"})

	resp, err := p.PreContainerRun(context.Background(), &plugin.PreContainerRunRequest{
		WorkspaceID: "myproject",
		RemoteUser:  "vscode",
	})
	if err != nil {
		t.Fatalf("PreContainerRun: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(resp.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(resp.Mounts))
	}

	m := resp.Mounts[0]
	if m.Type != "volume" {
		t.Errorf("mount type = %q, want volume", m.Type)
	}
	if m.Source != "crib-cache-myproject-npm" {
		t.Errorf("mount source = %q, want crib-cache-myproject-npm", m.Source)
	}
	if m.Target != "/home/vscode/.npm" {
		t.Errorf("mount target = %q, want /home/vscode/.npm", m.Target)
	}
}

func TestPlugin_MultipleProviders(t *testing.T) {
	p := New([]string{"npm", "pip", "go"})

	resp, err := p.PreContainerRun(context.Background(), &plugin.PreContainerRunRequest{
		WorkspaceID: "polyglot",
		RemoteUser:  "developer",
	})
	if err != nil {
		t.Fatalf("PreContainerRun: %v", err)
	}
	if resp == nil || len(resp.Mounts) != 3 {
		t.Fatalf("expected 3 mounts, got %d", len(resp.Mounts))
	}

	targets := map[string]bool{}
	for _, m := range resp.Mounts {
		targets[m.Target] = true
	}

	expected := []string{
		"/home/developer/.npm",
		"/home/developer/.cache/pip",
		"/home/developer/go/pkg/mod",
	}
	for _, e := range expected {
		if !targets[e] {
			t.Errorf("expected mount target %q not found", e)
		}
	}
}

func TestPlugin_SystemProvider(t *testing.T) {
	tmpDir := t.TempDir()
	p := New([]string{"apt"})

	resp, err := p.PreContainerRun(context.Background(), &plugin.PreContainerRunRequest{
		WorkspaceID:  "myproject",
		WorkspaceDir: tmpDir,
		RemoteUser:   "vscode",
	})
	if err != nil {
		t.Fatalf("PreContainerRun: %v", err)
	}
	if resp == nil || len(resp.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(resp.Mounts))
	}

	if resp.Mounts[0].Target != "/var/cache/apt" {
		t.Errorf("apt target = %q, want /var/cache/apt", resp.Mounts[0].Target)
	}
}

func TestPlugin_AptDisablesDockerClean(t *testing.T) {
	tmpDir := t.TempDir()
	p := New([]string{"apt"})

	resp, err := p.PreContainerRun(context.Background(), &plugin.PreContainerRunRequest{
		WorkspaceID:  "myproject",
		WorkspaceDir: tmpDir,
		RemoteUser:   "vscode",
	})
	if err != nil {
		t.Fatalf("PreContainerRun: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have a FileCopy that targets docker-clean.
	if len(resp.Copies) != 1 {
		t.Fatalf("expected 1 copy, got %d", len(resp.Copies))
	}
	cp := resp.Copies[0]
	if cp.Target != "/etc/apt/apt.conf.d/docker-clean" {
		t.Errorf("copy target = %q, want /etc/apt/apt.conf.d/docker-clean", cp.Target)
	}

	// Staged file should exist and contain only a comment.
	data, err := os.ReadFile(cp.Source)
	if err != nil {
		t.Fatalf("reading staged file: %v", err)
	}
	if string(data) != "# Disabled by crib (package-cache plugin) to preserve apt cache across rebuilds.\n" {
		t.Errorf("staged file content = %q", string(data))
	}

	// Should be under the workspace plugins dir.
	expected := filepath.Join(tmpDir, "plugins", "package-cache", "docker-clean")
	if cp.Source != expected {
		t.Errorf("copy source = %q, want %q", cp.Source, expected)
	}
}

func TestPlugin_NonAptNoCopies(t *testing.T) {
	p := New([]string{"npm"})

	resp, err := p.PreContainerRun(context.Background(), &plugin.PreContainerRunRequest{
		WorkspaceID: "myproject",
		RemoteUser:  "vscode",
	})
	if err != nil {
		t.Fatalf("PreContainerRun: %v", err)
	}
	if len(resp.Copies) != 0 {
		t.Errorf("expected no copies for npm, got %d", len(resp.Copies))
	}
}

func TestPlugin_RootUser(t *testing.T) {
	p := New([]string{"npm"})

	resp, err := p.PreContainerRun(context.Background(), &plugin.PreContainerRunRequest{
		WorkspaceID: "myproject",
		RemoteUser:  "root",
	})
	if err != nil {
		t.Fatalf("PreContainerRun: %v", err)
	}
	if resp == nil || len(resp.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(resp.Mounts))
	}

	if resp.Mounts[0].Target != "/root/.npm" {
		t.Errorf("root target = %q, want /root/.npm", resp.Mounts[0].Target)
	}
}

func TestPlugin_UnknownProvider(t *testing.T) {
	p := New([]string{"nonexistent"})

	resp, err := p.PreContainerRun(context.Background(), &plugin.PreContainerRunRequest{
		WorkspaceID: "myproject",
		RemoteUser:  "vscode",
	})
	if err != nil {
		t.Fatalf("PreContainerRun: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response for unknown provider, got %+v", resp)
	}
}

func TestPlugin_EmptyProviders(t *testing.T) {
	p := New(nil)

	resp, err := p.PreContainerRun(context.Background(), &plugin.PreContainerRunRequest{
		WorkspaceID: "myproject",
		RemoteUser:  "vscode",
	})
	if err != nil {
		t.Fatalf("PreContainerRun: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response for empty providers, got %+v", resp)
	}
}

func TestPlugin_Name(t *testing.T) {
	p := New(nil)
	if p.Name() != "package-cache" {
		t.Errorf("Name() = %q, want package-cache", p.Name())
	}
}

func TestValidateProviders(t *testing.T) {
	t.Run("all valid", func(t *testing.T) {
		unknown := ValidateProviders([]string{"npm", "pip", "go"})
		if len(unknown) != 0 {
			t.Errorf("expected no unknown providers, got %v", unknown)
		}
	})

	t.Run("some unknown", func(t *testing.T) {
		unknown := ValidateProviders([]string{"npm", "bogus", "pip", "fake"})
		if len(unknown) != 2 {
			t.Fatalf("expected 2 unknown providers, got %v", unknown)
		}
		if unknown[0] != "bogus" || unknown[1] != "fake" {
			t.Errorf("unknown = %v, want [bogus fake]", unknown)
		}
	})

	t.Run("empty", func(t *testing.T) {
		unknown := ValidateProviders(nil)
		if len(unknown) != 0 {
			t.Errorf("expected no unknown providers, got %v", unknown)
		}
	})
}

func TestPlugin_GoSetsGOMODCACHE(t *testing.T) {
	p := New([]string{"go"})

	resp, err := p.PreContainerRun(context.Background(), &plugin.PreContainerRunRequest{
		WorkspaceID: "myproject",
		RemoteUser:  "dev",
	})
	if err != nil {
		t.Fatalf("PreContainerRun: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Env == nil {
		t.Fatal("expected env to be set for go provider")
	}
	got := resp.Env["GOMODCACHE"]
	if got != "/home/dev/go/pkg/mod" {
		t.Errorf("GOMODCACHE = %q, want /home/dev/go/pkg/mod", got)
	}
}

func TestPlugin_NpmNoEnvVar(t *testing.T) {
	p := New([]string{"npm"})

	resp, err := p.PreContainerRun(context.Background(), &plugin.PreContainerRunRequest{
		WorkspaceID: "myproject",
		RemoteUser:  "vscode",
	})
	if err != nil {
		t.Fatalf("PreContainerRun: %v", err)
	}
	if resp.Env != nil {
		t.Errorf("expected no env for npm provider, got %v", resp.Env)
	}
}

func TestVolumeName(t *testing.T) {
	if got := VolumeName("myws", "npm"); got != "crib-cache-myws-npm" {
		t.Errorf("VolumeName(myws, npm) = %q, want crib-cache-myws-npm", got)
	}
}

func TestVolumePrefix(t *testing.T) {
	if got := VolumePrefix("myws"); got != "crib-cache-myws-" {
		t.Errorf("VolumePrefix(myws) = %q, want crib-cache-myws-", got)
	}
}
