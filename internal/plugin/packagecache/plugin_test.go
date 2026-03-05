package packagecache

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestPlugin_CargoSetsCARGOHOME(t *testing.T) {
	p := New([]string{"cargo"})

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
	if resp.Env == nil {
		t.Fatal("expected env to be set for cargo provider")
	}
	got := resp.Env["CARGO_HOME"]
	if got != "/home/vscode/.cargo" {
		t.Errorf("CARGO_HOME = %q, want /home/vscode/.cargo", got)
	}
	if resp.Mounts[0].Target != "/home/vscode/.cargo" {
		t.Errorf("mount target = %q, want /home/vscode/.cargo", resp.Mounts[0].Target)
	}
}

func TestPlugin_BundlerSetsBUNDLEPATH(t *testing.T) {
	tmpDir := t.TempDir()
	p := New([]string{"bundler"})

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
	if resp.Env["BUNDLE_PATH"] != "/home/vscode/.bundle" {
		t.Errorf("BUNDLE_PATH = %q, want /home/vscode/.bundle", resp.Env["BUNDLE_PATH"])
	}
	if resp.Env["BUNDLE_BIN"] != "/home/vscode/.bundle/bin" {
		t.Errorf("BUNDLE_BIN = %q, want /home/vscode/.bundle/bin", resp.Env["BUNDLE_BIN"])
	}

	// Should have a profile.d script for PATH.
	var foundProfile bool
	for _, cp := range resp.Copies {
		if cp.Target == "/etc/profile.d/crib-bundler-path.sh" {
			foundProfile = true
			data, err := os.ReadFile(cp.Source)
			if err != nil {
				t.Fatalf("reading profile script: %v", err)
			}
			if !strings.Contains(string(data), ".bundle/bin") {
				t.Errorf("profile script missing .bundle/bin: %q", string(data))
			}
		}
	}
	if !foundProfile {
		t.Error("expected profile.d copy for bundler PATH")
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

func TestBuildCacheMounts(t *testing.T) {
	t.Run("home-relative providers use /root", func(t *testing.T) {
		mounts := BuildCacheMounts([]string{"npm", "pip"})
		expected := map[string]bool{
			"/root/.npm":       true,
			"/root/.cache/pip": true,
		}
		if len(mounts) != len(expected) {
			t.Fatalf("expected %d mounts, got %d: %v", len(expected), len(mounts), mounts)
		}
		for _, m := range mounts {
			if !expected[m] {
				t.Errorf("unexpected mount %q", m)
			}
		}
	})

	t.Run("apt includes both cache and lists", func(t *testing.T) {
		mounts := BuildCacheMounts([]string{"apt"})
		expected := map[string]bool{
			"/var/cache/apt":     true,
			"/var/lib/apt/lists": true,
		}
		if len(mounts) != len(expected) {
			t.Fatalf("expected %d mounts, got %d: %v", len(expected), len(mounts), mounts)
		}
		for _, m := range mounts {
			if !expected[m] {
				t.Errorf("unexpected mount %q", m)
			}
		}
	})

	t.Run("unknown providers skipped", func(t *testing.T) {
		mounts := BuildCacheMounts([]string{"bogus"})
		if len(mounts) != 0 {
			t.Errorf("expected no mounts, got %v", mounts)
		}
	})

	t.Run("nil providers", func(t *testing.T) {
		mounts := BuildCacheMounts(nil)
		if len(mounts) != 0 {
			t.Errorf("expected no mounts, got %v", mounts)
		}
	})

	t.Run("go uses /root with GOMODCACHE path", func(t *testing.T) {
		mounts := BuildCacheMounts([]string{"go"})
		if len(mounts) != 1 || mounts[0] != "/root/go/pkg/mod" {
			t.Errorf("go mounts = %v, want [/root/go/pkg/mod]", mounts)
		}
	})
}

func TestPlugin_DownloadsSetsCRIBCACHE(t *testing.T) {
	p := New([]string{"downloads"})

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
	if resp.Mounts[0].Target != "/home/vscode/.cache/crib" {
		t.Errorf("mount target = %q, want /home/vscode/.cache/crib", resp.Mounts[0].Target)
	}
	if resp.Mounts[0].Source != "crib-cache-myproject-downloads" {
		t.Errorf("mount source = %q, want crib-cache-myproject-downloads", resp.Mounts[0].Source)
	}
	if resp.Env["CRIB_CACHE"] != "/home/vscode/.cache/crib" {
		t.Errorf("CRIB_CACHE = %q, want /home/vscode/.cache/crib", resp.Env["CRIB_CACHE"])
	}
}

func TestBuildCacheMounts_Downloads(t *testing.T) {
	mounts := BuildCacheMounts([]string{"downloads"})
	if len(mounts) != 1 || mounts[0] != "/root/.cache/crib" {
		t.Errorf("downloads build mounts = %v, want [/root/.cache/crib]", mounts)
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
