package engine

import (
	"context"
	"log/slog"
	"testing"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/workspace"
)

func TestComposeBackend_CanResumeFromStored_ReturnsTrue(t *testing.T) {
	b := &composeBackend{}
	if !b.canResumeFromStored() {
		t.Error("canResumeFromStored() = false, want true")
	}
}

func TestComposeBackend_PluginUser_Delegates(t *testing.T) {
	// When config has remoteUser set, resolveComposeUser returns ""
	// (the config user is used as fallback by dispatchPlugins).
	eng := &Engine{logger: slog.Default()}

	cfg := &config.DevContainerConfig{}
	cfg.RemoteUser = "vscode"

	b := &composeBackend{
		e:   eng,
		cfg: cfg,
		inv: composeInvocation{files: []string{}},
	}

	user := b.pluginUser(context.Background())
	// resolveComposeUser returns "" when config already has remoteUser.
	if user != "" {
		t.Errorf("pluginUser() = %q, want empty (config has remoteUser)", user)
	}
}

func TestComposeBackend_BuildImage_SkipsWhenNoFeatures(t *testing.T) {
	eng := &Engine{logger: slog.Default()}

	cfg := &config.DevContainerConfig{}
	// No features configured.

	b := &composeBackend{
		e:   eng,
		cfg: cfg,
	}

	result, err := b.buildImage(context.Background())
	if err != nil {
		t.Fatalf("buildImage: %v", err)
	}
	if result.imageName != "" {
		t.Errorf("imageName = %q, want empty (no features)", result.imageName)
	}
	if result.hasEntrypoints {
		t.Error("hasEntrypoints = true, want false (no features)")
	}
}

func TestComposeBackend_PluginUser_NoConfigUser_ReturnsEmpty(t *testing.T) {
	eng := &Engine{logger: slog.Default()}

	// No remoteUser or containerUser. resolveComposeUser will try to inspect
	// the compose service but with no files, it returns "".
	cfg := &config.DevContainerConfig{}
	cfg.Service = "app"

	b := &composeBackend{
		e:   eng,
		cfg: cfg,
		inv: composeInvocation{files: []string{}},
	}

	user := b.pluginUser(context.Background())
	if user != "" {
		t.Errorf("pluginUser() = %q, want empty", user)
	}
}

func TestComposeBackend_DeleteExisting_RequiresCompose(t *testing.T) {
	// deleteExisting calls composeDown which uses e.compose.
	// Without compose, it should error.
	store := workspace.NewStoreAt(t.TempDir())
	eng := &Engine{
		logger:  slog.Default(),
		compose: nil,
		store:   store,
	}

	ws := &workspace.Workspace{ID: "ws-compose-del", Source: "/home/user/project"}

	b := &composeBackend{
		e:   eng,
		ws:  ws,
		cfg: &config.DevContainerConfig{},
		inv: composeInvocation{files: []string{}},
	}

	// compose is nil, so composeDown will panic or fail. Since compose.Down
	// is called directly, we expect a nil pointer dereference. This is
	// expected behavior (compose should always be set for compose backends).
	// We test this to verify the delegation path.
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when compose is nil")
		}
	}()
	_ = b.deleteExisting(context.Background())
}
