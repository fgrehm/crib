package config

import (
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestMergeConfiguration_EmptyMetadata(t *testing.T) {
	config := &DevContainerConfig{
		ImageContainer: ImageContainer{Image: "ubuntu:22.04"},
		DevContainerConfigBase: DevContainerConfigBase{
			RemoteUser: "vscode",
		},
	}

	merged := MergeConfiguration(config, nil)

	if merged.Image != "ubuntu:22.04" {
		t.Errorf("Image = %q, want %q", merged.Image, "ubuntu:22.04")
	}
	if merged.RemoteUser != "vscode" {
		t.Errorf("RemoteUser = %q, want %q", merged.RemoteUser, "vscode")
	}
}

func TestMergeConfiguration_RemoteUser(t *testing.T) {
	// Base config has no remoteUser, metadata entry provides one.
	config := &DevContainerConfig{
		ImageContainer: ImageContainer{Image: "ubuntu"},
	}
	metadata := []*ImageMetadata{
		{DevContainerConfigBase: DevContainerConfigBase{RemoteUser: "dev"}},
	}

	merged := MergeConfiguration(config, metadata)

	if merged.RemoteUser != "dev" {
		t.Errorf("RemoteUser = %q, want %q", merged.RemoteUser, "dev")
	}
}

func TestMergeConfiguration_RemoteUserBaseWins(t *testing.T) {
	config := &DevContainerConfig{
		DevContainerConfigBase: DevContainerConfigBase{RemoteUser: "root"},
	}
	metadata := []*ImageMetadata{
		{DevContainerConfigBase: DevContainerConfigBase{RemoteUser: "dev"}},
	}

	merged := MergeConfiguration(config, metadata)

	if merged.RemoteUser != "root" {
		t.Errorf("RemoteUser = %q, want %q", merged.RemoteUser, "root")
	}
}

func TestMergeConfiguration_LifecycleHooks(t *testing.T) {
	config := &DevContainerConfig{
		DevContainerActions: DevContainerActions{
			OnCreateCommand:  LifecycleHook{"": {"echo base"}},
			PostStartCommand: LifecycleHook{"": {"echo start"}},
		},
	}
	metadata := []*ImageMetadata{
		{
			DevContainerActions: DevContainerActions{
				OnCreateCommand: LifecycleHook{"": {"echo feature1"}},
			},
		},
		{
			DevContainerActions: DevContainerActions{
				OnCreateCommand: LifecycleHook{"": {"echo feature2"}},
			},
		},
	}

	merged := MergeConfiguration(config, metadata)

	// OnCreate: feature2 (reversed first), feature1, then base.
	if len(merged.OnCreateCommands) != 3 {
		t.Fatalf("OnCreateCommands length = %d, want 3", len(merged.OnCreateCommands))
	}

	// PostStart: only base.
	if len(merged.PostStartCommands) != 1 {
		t.Fatalf("PostStartCommands length = %d, want 1", len(merged.PostStartCommands))
	}
}

func TestMergeConfiguration_Entrypoints(t *testing.T) {
	config := &DevContainerConfig{}
	metadata := []*ImageMetadata{
		{Entrypoint: "/entry1.sh"},
		{Entrypoint: "/entry2.sh"},
	}

	merged := MergeConfiguration(config, metadata)

	// Reversed order: entry2 first, then entry1.
	if len(merged.Entrypoints) != 2 {
		t.Fatalf("Entrypoints length = %d, want 2", len(merged.Entrypoints))
	}
	if merged.Entrypoints[0] != "/entry2.sh" {
		t.Errorf("Entrypoints[0] = %q, want %q", merged.Entrypoints[0], "/entry2.sh")
	}
	if merged.Entrypoints[1] != "/entry1.sh" {
		t.Errorf("Entrypoints[1] = %q, want %q", merged.Entrypoints[1], "/entry1.sh")
	}
}

func TestMergeConfiguration_RemoteEnv(t *testing.T) {
	config := &DevContainerConfig{
		DevContainerConfigBase: DevContainerConfigBase{
			RemoteEnv: map[string]string{"BASE": "val", "SHARED": "base"},
		},
	}
	metadata := []*ImageMetadata{
		{DevContainerConfigBase: DevContainerConfigBase{
			RemoteEnv: map[string]string{"FEATURE": "fval", "SHARED": "feature"},
		}},
	}

	merged := MergeConfiguration(config, metadata)

	if merged.RemoteEnv["BASE"] != "val" {
		t.Errorf("RemoteEnv[BASE] = %q, want %q", merged.RemoteEnv["BASE"], "val")
	}
	if merged.RemoteEnv["FEATURE"] != "fval" {
		t.Errorf("RemoteEnv[FEATURE] = %q, want %q", merged.RemoteEnv["FEATURE"], "fval")
	}
	// Base wins on shared keys.
	if merged.RemoteEnv["SHARED"] != "base" {
		t.Errorf("RemoteEnv[SHARED] = %q, want %q (base should win)", merged.RemoteEnv["SHARED"], "base")
	}
}

func TestMergeConfiguration_ForwardPorts(t *testing.T) {
	config := &DevContainerConfig{
		DevContainerConfigBase: DevContainerConfigBase{
			ForwardPorts: StrIntArray{"8080", "3000"},
		},
	}
	metadata := []*ImageMetadata{
		{DevContainerConfigBase: DevContainerConfigBase{
			ForwardPorts: StrIntArray{"8080", "9090"},
		}},
	}

	merged := MergeConfiguration(config, metadata)

	// Should be {8080, 3000, 9090} (deduplicated).
	if len(merged.ForwardPorts) != 3 {
		t.Fatalf("ForwardPorts length = %d, want 3", len(merged.ForwardPorts))
	}
	want := map[string]bool{"8080": true, "3000": true, "9090": true}
	for _, p := range merged.ForwardPorts {
		if !want[p] {
			t.Errorf("unexpected port %q", p)
		}
	}
}

func TestMergeConfiguration_Mounts(t *testing.T) {
	config := &DevContainerConfig{
		NonComposeBase: NonComposeBase{
			Mounts: []Mount{
				{Type: "bind", Source: "/host-a", Target: "/container-a"},
				{Type: "bind", Source: "/host-b", Target: "/shared"},
			},
		},
	}
	metadata := []*ImageMetadata{
		{NonComposeBase: NonComposeBase{
			Mounts: []Mount{
				{Type: "volume", Source: "data", Target: "/shared"},
				{Type: "bind", Source: "/host-c", Target: "/container-c"},
			},
		}},
	}

	merged := MergeConfiguration(config, metadata)

	// Base mounts win for /shared target.
	if len(merged.Mounts) != 3 {
		t.Fatalf("Mounts length = %d, want 3", len(merged.Mounts))
	}

	mountsByTarget := make(map[string]Mount)
	for _, m := range merged.Mounts {
		mountsByTarget[m.Target] = m
	}

	if m := mountsByTarget["/shared"]; m.Source != "/host-b" {
		t.Errorf("/shared mount source = %q, want %q (base should win)", m.Source, "/host-b")
	}
}

func TestMergeConfiguration_BoolPointers(t *testing.T) {
	config := &DevContainerConfig{}
	metadata := []*ImageMetadata{
		{NonComposeBase: NonComposeBase{Init: boolPtr(true)}},
		{NonComposeBase: NonComposeBase{Init: boolPtr(false)}},
	}

	merged := MergeConfiguration(config, metadata)

	// First non-nil wins (reversed: second entry checked first).
	if merged.Init == nil {
		t.Fatal("Init should not be nil")
	}
}

func TestMergeConfiguration_CapAdd(t *testing.T) {
	config := &DevContainerConfig{
		NonComposeBase: NonComposeBase{
			CapAdd: []string{"SYS_PTRACE", "NET_ADMIN"},
		},
	}
	metadata := []*ImageMetadata{
		{NonComposeBase: NonComposeBase{
			CapAdd: []string{"SYS_PTRACE", "SYS_CHROOT"},
		}},
	}

	merged := MergeConfiguration(config, metadata)

	if len(merged.CapAdd) != 3 {
		t.Fatalf("CapAdd length = %d, want 3", len(merged.CapAdd))
	}
	want := map[string]bool{"SYS_PTRACE": true, "NET_ADMIN": true, "SYS_CHROOT": true}
	for _, c := range merged.CapAdd {
		if !want[c] {
			t.Errorf("unexpected cap %q", c)
		}
	}
}

func TestMergeConfiguration_PreservesOrigin(t *testing.T) {
	config := &DevContainerConfig{
		Origin: "/path/to/devcontainer.json",
	}

	merged := MergeConfiguration(config, nil)

	if merged.Origin != config.Origin {
		t.Errorf("Origin = %q, want %q", merged.Origin, config.Origin)
	}
}
