package compose

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadProject(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	testdataDir := filepath.Join(filepath.Dir(thisFile), "testdata")
	composePath := filepath.Join(testdataDir, "simple-compose.yml")

	ctx := context.Background()
	project, err := LoadProject(ctx, []string{composePath}, nil)
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	if project == nil {
		t.Fatal("LoadProject returned nil")
	}

	// Verify the service is loaded.
	svc, err := project.GetService("app")
	if err != nil {
		t.Fatalf("GetService(app): %v", err)
	}

	if svc.Image != "alpine:3.20" {
		t.Errorf("service image = %q, want %q", svc.Image, "alpine:3.20")
	}
}

func TestLoadProject_NoFiles(t *testing.T) {
	ctx := context.Background()
	_, err := LoadProject(ctx, nil, nil)
	if err == nil {
		t.Error("expected error for no files, got nil")
	}
}

func TestGetServiceInfo_ImageBased(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	testdataDir := filepath.Join(filepath.Dir(thisFile), "testdata")
	composePath := filepath.Join(testdataDir, "simple-compose.yml")

	ctx := context.Background()
	info, err := GetServiceInfo(ctx, []string{composePath}, "app", nil)
	if err != nil {
		t.Fatalf("GetServiceInfo: %v", err)
	}

	if info.Image != "alpine:3.20" {
		t.Errorf("Image = %q, want %q", info.Image, "alpine:3.20")
	}
	if info.HasBuild {
		t.Error("HasBuild = true, want false")
	}
	if info.User != "" {
		t.Errorf("User = %q, want empty", info.User)
	}
}

func TestGetServiceInfo_BuildBased(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	testdataDir := filepath.Join(filepath.Dir(thisFile), "testdata")
	composePath := filepath.Join(testdataDir, "build-compose.yml")

	ctx := context.Background()
	info, err := GetServiceInfo(ctx, []string{composePath}, "web", nil)
	if err != nil {
		t.Fatalf("GetServiceInfo: %v", err)
	}

	if info.Image != "" {
		t.Errorf("Image = %q, want empty", info.Image)
	}
	if !info.HasBuild {
		t.Error("HasBuild = false, want true")
	}
	if info.Dockerfile != "Dockerfile.dev" {
		t.Errorf("Dockerfile = %q, want %q", info.Dockerfile, "Dockerfile.dev")
	}
	if info.User != "vscode" {
		t.Errorf("User = %q, want %q", info.User, "vscode")
	}
}

func TestGetServiceInfo_NotFound(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	testdataDir := filepath.Join(filepath.Dir(thisFile), "testdata")
	composePath := filepath.Join(testdataDir, "simple-compose.yml")

	ctx := context.Background()
	_, err := GetServiceInfo(ctx, []string{composePath}, "nonexistent", nil)
	if err == nil {
		t.Error("expected error for nonexistent service, got nil")
	}
}

func TestBuiltImageName(t *testing.T) {
	tests := []struct {
		name    string
		runtime string
		want    string
	}{
		{"docker", "docker", "crib-web-rails-app"},
		{"podman", "podman", "crib-web_rails-app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHelperFromRuntime(tt.runtime)
			got := h.BuiltImageName("crib-web", "rails-app")
			if got != tt.want {
				t.Errorf("BuiltImageName = %q, want %q", got, tt.want)
			}
		})
	}
}
