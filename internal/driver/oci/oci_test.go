package oci

import "testing"

func TestContainerName(t *testing.T) {
	tests := []struct {
		wsID string
		want string
	}{
		{"myproject", "crib-myproject"},
		{"foo-bar", "crib-foo-bar"},
		{"a", "crib-a"},
	}
	for _, tt := range tests {
		if got := ContainerName(tt.wsID); got != tt.want {
			t.Errorf("ContainerName(%q) = %q, want %q", tt.wsID, got, tt.want)
		}
	}
}

func TestImageName(t *testing.T) {
	tests := []struct {
		wsID string
		tag  string
		want string
	}{
		{"myproject", "latest", "crib-myproject:latest"},
		{"foo", "abc123", "crib-foo:abc123"},
	}
	for _, tt := range tests {
		if got := ImageName(tt.wsID, tt.tag); got != tt.want {
			t.Errorf("ImageName(%q, %q) = %q, want %q", tt.wsID, tt.tag, got, tt.want)
		}
	}
}

func TestWorkspaceLabel(t *testing.T) {
	tests := []struct {
		wsID string
		want string
	}{
		{"myproject", "crib.workspace=myproject"},
		{"foo-bar", "crib.workspace=foo-bar"},
	}
	for _, tt := range tests {
		if got := WorkspaceLabel(tt.wsID); got != tt.want {
			t.Errorf("WorkspaceLabel(%q) = %q, want %q", tt.wsID, got, tt.want)
		}
	}
}

func TestRuntimeString(t *testing.T) {
	if got := RuntimeDocker.String(); got != "docker" {
		t.Errorf("RuntimeDocker.String() = %q, want %q", got, "docker")
	}
	if got := RuntimePodman.String(); got != "podman" {
		t.Errorf("RuntimePodman.String() = %q, want %q", got, "podman")
	}
}
