package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fgrehm/crib/internal/workspace"
)

func TestLiveRemoteUser_RemoteUser(t *testing.T) {
	dir := t.TempDir()
	writeDevcontainerJSON(t, dir, `{"remoteUser":"liveuser"}`)
	ws := wsAt(dir)

	got := liveRemoteUser(ws)
	if got != "liveuser" {
		t.Errorf("got %q, want liveuser", got)
	}
}

func TestLiveRemoteUser_ContainerUserFallback(t *testing.T) {
	dir := t.TempDir()
	writeDevcontainerJSON(t, dir, `{"containerUser":"cuser"}`)
	ws := wsAt(dir)

	got := liveRemoteUser(ws)
	if got != "cuser" {
		t.Errorf("got %q, want cuser", got)
	}
}

func TestLiveRemoteUser_RemoteUserTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	writeDevcontainerJSON(t, dir, `{"remoteUser":"ruser","containerUser":"cuser"}`)
	ws := wsAt(dir)

	got := liveRemoteUser(ws)
	if got != "ruser" {
		t.Errorf("got %q, want ruser (remoteUser wins)", got)
	}
}

func TestLiveRemoteUser_MissingConfig(t *testing.T) {
	dir := t.TempDir()
	ws := wsAt(dir)
	// No .devcontainer/ directory.
	got := liveRemoteUser(ws)
	if got != "" {
		t.Errorf("got %q, want empty string for missing config", got)
	}
}

func TestLiveRemoteUser_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	writeDevcontainerJSON(t, dir, `{bad json`)
	ws := wsAt(dir)

	got := liveRemoteUser(ws)
	if got != "" {
		t.Errorf("got %q, want empty string for malformed JSON", got)
	}
}

func TestLiveRemoteUser_NeitherSet(t *testing.T) {
	dir := t.TempDir()
	writeDevcontainerJSON(t, dir, `{"image":"alpine:3.20"}`)
	ws := wsAt(dir)

	got := liveRemoteUser(ws)
	if got != "" {
		t.Errorf("got %q, want empty string when neither user field is set", got)
	}
}

// wsAt creates a minimal Workspace pointing to dir with a standard devcontainer path.
func wsAt(dir string) *workspace.Workspace {
	return &workspace.Workspace{
		ID:               "test-ws",
		Source:           dir,
		DevContainerPath: ".devcontainer/devcontainer.json",
	}
}

// writeDevcontainerJSON writes content to dir/.devcontainer/devcontainer.json.
func writeDevcontainerJSON(t *testing.T, dir, content string) {
	t.Helper()
	devDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devDir, "devcontainer.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
