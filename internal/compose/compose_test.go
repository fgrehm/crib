package compose

import (
	"testing"
)

func TestProjectName_Default(t *testing.T) {
	t.Setenv("COMPOSE_PROJECT_NAME", "")
	got := ProjectName("myproject")
	if got != "crib-myproject" {
		t.Errorf("ProjectName(%q) = %q, want %q", "myproject", got, "crib-myproject")
	}
}

func TestProjectName_EnvOverride(t *testing.T) {
	t.Setenv("COMPOSE_PROJECT_NAME", "custom-name")
	got := ProjectName("myproject")
	if got != "custom-name" {
		t.Errorf("ProjectName(%q) = %q, want %q", "myproject", got, "custom-name")
	}
}

func TestProjectArgs_NoFiles(t *testing.T) {
	args := projectArgs("myproj", nil)
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if args[0] != "--project-name" || args[1] != "myproj" {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestProjectArgs_WithFiles(t *testing.T) {
	args := projectArgs("myproj", []string{"a.yml", "b.yml"})
	expected := []string{"--project-name", "myproj", "-f", "a.yml", "-f", "b.yml"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, want := range expected {
		if args[i] != want {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want)
		}
	}
}
