package codingagents

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgrehm/crib/internal/plugin"
)

func testReq(workspaceDir, remoteUser string) *plugin.PreContainerRunRequest {
	return &plugin.PreContainerRunRequest{
		WorkspaceID:     "test-ws",
		WorkspaceDir:    workspaceDir,
		SourceDir:       "/home/user/project",
		Runtime:         "docker",
		ImageName:       "ubuntu:22.04",
		RemoteUser:      remoteUser,
		WorkspaceFolder: "/workspaces/project",
		ContainerName:   "crib-test-ws",
	}
}

func testReqWithCustomizations(workspaceDir, remoteUser string, customizations map[string]any) *plugin.PreContainerRunRequest {
	req := testReq(workspaceDir, remoteUser)
	req.Customizations = customizations
	return req
}

// setupCredentials creates the minimal ~/.claude/.credentials.json file.
func setupCredentials(t *testing.T, home string) {
	t.Helper()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	creds := `{"claudeAiOauth":{"accessToken":"sk-ant-oat01-test","refreshToken":"sk-ant-ort01-test","expiresAt":9999999999999}}`
	if err := os.WriteFile(filepath.Join(claudeDir, ".credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestName(t *testing.T) {
	p := New()
	if p.Name() != "coding-agents" {
		t.Errorf("expected name coding-agents, got %s", p.Name())
	}
}

// --- Host mode tests (default behavior) ---

func TestPreContainerRun_CredentialsExist(t *testing.T) {
	home := t.TempDir()
	setupCredentials(t, home)

	wsDir := t.TempDir()
	p := &Plugin{homeDir: home}
	resp, err := p.PreContainerRun(context.Background(), testReq(wsDir, "vscode"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should produce two file copies, no mounts.
	if len(resp.Mounts) != 0 {
		t.Errorf("expected 0 mounts, got %d", len(resp.Mounts))
	}
	if len(resp.Copies) != 2 {
		t.Fatalf("expected 2 copies, got %d", len(resp.Copies))
	}

	// First copy: credentials file.
	creds := resp.Copies[0]
	expectedCredsSource := filepath.Join(wsDir, "plugins", "coding-agents", "claude-code", "credentials.json")
	if creds.Source != expectedCredsSource {
		t.Errorf("credentials source: expected %s, got %s", expectedCredsSource, creds.Source)
	}
	if creds.Target != "/home/vscode/.claude/.credentials.json" {
		t.Errorf("credentials target: expected /home/vscode/.claude/.credentials.json, got %s", creds.Target)
	}
	if creds.Mode != "0600" {
		t.Errorf("credentials mode: expected 0600, got %s", creds.Mode)
	}
	if creds.User != "vscode" {
		t.Errorf("credentials user: expected vscode, got %s", creds.User)
	}

	// Second copy: generated claude.json.
	config := resp.Copies[1]
	expectedConfigSource := filepath.Join(wsDir, "plugins", "coding-agents", "claude-code", "claude.json")
	if config.Source != expectedConfigSource {
		t.Errorf("config source: expected %s, got %s", expectedConfigSource, config.Source)
	}
	if config.Target != "/home/vscode/.claude.json" {
		t.Errorf("config target: expected /home/vscode/.claude.json, got %s", config.Target)
	}
	if config.User != "vscode" {
		t.Errorf("config user: expected vscode, got %s", config.User)
	}
	if !config.IfNotExists {
		t.Errorf("config IfNotExists: expected true (don't overwrite user's ~/.claude.json)")
	}
}

func TestPreContainerRun_NoCredentialsFile(t *testing.T) {
	home := t.TempDir()
	// Create ~/.claude/ but no .credentials.json.
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	p := &Plugin{homeDir: home}
	resp, err := p.PreContainerRun(context.Background(), testReq(t.TempDir(), "vscode"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response when .credentials.json missing, got %+v", resp)
	}
}

func TestPreContainerRun_NoClaudeDir(t *testing.T) {
	home := t.TempDir() // no .claude/ directory at all

	p := &Plugin{homeDir: home}
	resp, err := p.PreContainerRun(context.Background(), testReq(t.TempDir(), "vscode"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response for missing ~/.claude, got %+v", resp)
	}
}

func TestPreContainerRun_RemoteUserVscode(t *testing.T) {
	home := t.TempDir()
	setupCredentials(t, home)

	p := &Plugin{homeDir: home}
	resp, err := p.PreContainerRun(context.Background(), testReq(t.TempDir(), "vscode"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Copies[0].Target != "/home/vscode/.claude/.credentials.json" {
		t.Errorf("credentials target: expected /home/vscode/.claude/.credentials.json, got %s", resp.Copies[0].Target)
	}
	if resp.Copies[0].User != "vscode" {
		t.Errorf("credentials user: expected vscode, got %s", resp.Copies[0].User)
	}
	if resp.Copies[1].Target != "/home/vscode/.claude.json" {
		t.Errorf("config target: expected /home/vscode/.claude.json, got %s", resp.Copies[1].Target)
	}
}

func TestPreContainerRun_RemoteUserRoot(t *testing.T) {
	home := t.TempDir()
	setupCredentials(t, home)

	p := &Plugin{homeDir: home}
	resp, err := p.PreContainerRun(context.Background(), testReq(t.TempDir(), "root"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Copies[0].Target != "/root/.claude/.credentials.json" {
		t.Errorf("credentials target: expected /root/.claude/.credentials.json, got %s", resp.Copies[0].Target)
	}
	if resp.Copies[0].User != "root" {
		t.Errorf("credentials user: expected root, got %s", resp.Copies[0].User)
	}
	if resp.Copies[1].Target != "/root/.claude.json" {
		t.Errorf("config target: expected /root/.claude.json, got %s", resp.Copies[1].Target)
	}
}

func TestPreContainerRun_RemoteUserEmpty(t *testing.T) {
	home := t.TempDir()
	setupCredentials(t, home)

	p := &Plugin{homeDir: home}
	resp, err := p.PreContainerRun(context.Background(), testReq(t.TempDir(), ""))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Copies[0].Target != "/root/.claude/.credentials.json" {
		t.Errorf("credentials target: expected /root/.claude/.credentials.json, got %s", resp.Copies[0].Target)
	}
	if resp.Copies[0].User != "root" {
		t.Errorf("credentials user: expected root, got %s", resp.Copies[0].User)
	}
	if resp.Copies[1].Target != "/root/.claude.json" {
		t.Errorf("config target: expected /root/.claude.json, got %s", resp.Copies[1].Target)
	}
}

func TestPreContainerRun_CredentialsCopiedToStaging(t *testing.T) {
	home := t.TempDir()
	setupCredentials(t, home)

	wsDir := t.TempDir()
	p := &Plugin{homeDir: home}
	if _, err := p.PreContainerRun(context.Background(), testReq(wsDir, "vscode")); err != nil {
		t.Fatal(err)
	}

	copiedCreds := filepath.Join(wsDir, "plugins", "coding-agents", "claude-code", "credentials.json")
	data, err := os.ReadFile(copiedCreds)
	if err != nil {
		t.Fatalf("expected credentials to be staged: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("staged credentials not valid JSON: %v", err)
	}
	if _, ok := parsed["claudeAiOauth"]; !ok {
		t.Errorf("expected claudeAiOauth key in staged credentials")
	}
}

func TestPreContainerRun_ConfigGenerated(t *testing.T) {
	home := t.TempDir()
	setupCredentials(t, home)

	wsDir := t.TempDir()
	p := &Plugin{homeDir: home}
	if _, err := p.PreContainerRun(context.Background(), testReq(wsDir, "vscode")); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(wsDir, "plugins", "coding-agents", "claude-code", "claude.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected claude.json to be generated: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generated config not valid JSON: %v", err)
	}
	if parsed["hasCompletedOnboarding"] != true {
		t.Errorf("expected hasCompletedOnboarding=true, got %v", parsed["hasCompletedOnboarding"])
	}
}

func TestPreContainerRun_HostModeExplicit(t *testing.T) {
	home := t.TempDir()
	setupCredentials(t, home)

	wsDir := t.TempDir()
	p := &Plugin{homeDir: home}
	req := testReqWithCustomizations(wsDir, "vscode", map[string]any{
		"coding-agents": map[string]any{
			"credentials": "host",
		},
	})
	resp, err := p.PreContainerRun(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	// Same behavior as default: two copies, no mounts.
	if len(resp.Mounts) != 0 {
		t.Errorf("expected 0 mounts in host mode, got %d", len(resp.Mounts))
	}
	if len(resp.Copies) != 2 {
		t.Fatalf("expected 2 copies in host mode, got %d", len(resp.Copies))
	}
}

// --- Workspace mode tests ---

func TestPreContainerRun_WorkspaceMode(t *testing.T) {
	wsDir := t.TempDir()
	p := &Plugin{homeDir: t.TempDir()} // home doesn't matter in workspace mode
	req := testReqWithCustomizations(wsDir, "vscode", map[string]any{
		"coding-agents": map[string]any{
			"credentials": "workspace",
		},
	})
	resp, err := p.PreContainerRun(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should produce one mount (persistent ~/.claude/) and one copy (onboarding config).
	if len(resp.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(resp.Mounts))
	}
	mount := resp.Mounts[0]
	expectedSource := filepath.Join(wsDir, "plugins", "coding-agents", "claude-state")
	if mount.Source != expectedSource {
		t.Errorf("mount source: expected %s, got %s", expectedSource, mount.Source)
	}
	if mount.Target != "/home/vscode/.claude" {
		t.Errorf("mount target: expected /home/vscode/.claude, got %s", mount.Target)
	}
	if mount.Type != "bind" {
		t.Errorf("mount type: expected bind, got %s", mount.Type)
	}

	if len(resp.Copies) != 1 {
		t.Fatalf("expected 1 copy (onboarding config only), got %d", len(resp.Copies))
	}
	copy := resp.Copies[0]
	if copy.Target != "/home/vscode/.claude.json" {
		t.Errorf("config target: expected /home/vscode/.claude.json, got %s", copy.Target)
	}
	if copy.User != "vscode" {
		t.Errorf("config user: expected vscode, got %s", copy.User)
	}
	if !copy.IfNotExists {
		t.Errorf("config IfNotExists: expected true (don't overwrite user's ~/.claude.json)")
	}
}

func TestPreContainerRun_WorkspaceMode_CreatesStateDir(t *testing.T) {
	wsDir := t.TempDir()
	p := &Plugin{homeDir: t.TempDir()}
	req := testReqWithCustomizations(wsDir, "vscode", map[string]any{
		"coding-agents": map[string]any{
			"credentials": "workspace",
		},
	})

	if _, err := p.PreContainerRun(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	stateDir := filepath.Join(wsDir, "plugins", "coding-agents", "claude-state")
	info, err := os.Stat(stateDir)
	if err != nil {
		t.Fatalf("expected state dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected state dir to be a directory")
	}
}

func TestPreContainerRun_WorkspaceMode_RootUser(t *testing.T) {
	wsDir := t.TempDir()
	p := &Plugin{homeDir: t.TempDir()}
	req := testReqWithCustomizations(wsDir, "root", map[string]any{
		"coding-agents": map[string]any{
			"credentials": "workspace",
		},
	})

	resp, err := p.PreContainerRun(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Mounts[0].Target != "/root/.claude" {
		t.Errorf("mount target: expected /root/.claude, got %s", resp.Mounts[0].Target)
	}
	if resp.Copies[0].Target != "/root/.claude.json" {
		t.Errorf("config target: expected /root/.claude.json, got %s", resp.Copies[0].Target)
	}
	if resp.Copies[0].User != "root" {
		t.Errorf("config user: expected root, got %s", resp.Copies[0].User)
	}
}

func TestPreContainerRun_WorkspaceMode_EmptyUser(t *testing.T) {
	wsDir := t.TempDir()
	p := &Plugin{homeDir: t.TempDir()}
	req := testReqWithCustomizations(wsDir, "", map[string]any{
		"coding-agents": map[string]any{
			"credentials": "workspace",
		},
	})

	resp, err := p.PreContainerRun(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Mounts[0].Target != "/root/.claude" {
		t.Errorf("mount target: expected /root/.claude, got %s", resp.Mounts[0].Target)
	}
	if resp.Copies[0].User != "root" {
		t.Errorf("config user: expected root, got %s", resp.Copies[0].User)
	}
}

func TestPreContainerRun_WorkspaceMode_IgnoresHostCreds(t *testing.T) {
	home := t.TempDir()
	setupCredentials(t, home) // credentials exist on host but should be ignored

	wsDir := t.TempDir()
	p := &Plugin{homeDir: home}
	req := testReqWithCustomizations(wsDir, "vscode", map[string]any{
		"coding-agents": map[string]any{
			"credentials": "workspace",
		},
	})

	resp, err := p.PreContainerRun(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	// Should not copy credentials even though they exist on host.
	for _, c := range resp.Copies {
		if c.Target == "/home/vscode/.claude/.credentials.json" {
			t.Errorf("workspace mode should not copy host credentials")
		}
	}
}

// --- getCredentialsMode tests ---

func TestGetCredentialsMode(t *testing.T) {
	tests := []struct {
		name           string
		customizations map[string]any
		want           string
	}{
		{"nil", nil, "host"},
		{"empty", map[string]any{}, "host"},
		{"no coding-agents key", map[string]any{"other": "value"}, "host"},
		{"coding-agents not a map", map[string]any{"coding-agents": "string"}, "host"},
		{"credentials not set", map[string]any{"coding-agents": map[string]any{}}, "host"},
		{"credentials host", map[string]any{"coding-agents": map[string]any{"credentials": "host"}}, "host"},
		{"credentials workspace", map[string]any{"coding-agents": map[string]any{"credentials": "workspace"}}, "workspace"},
		{"credentials unknown", map[string]any{"coding-agents": map[string]any{"credentials": "unknown"}}, "host"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getCredentialsMode(tt.customizations)
			if got != tt.want {
				t.Errorf("getCredentialsMode() = %q, want %q", got, tt.want)
			}
		})
	}
}
