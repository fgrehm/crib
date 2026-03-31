package workspace

import (
	"path/filepath"
	"testing"
)

func TestInferID_ConfigDir(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".devcontainer")
	mkdirAll(t, cfgDir)
	writeFile(t, filepath.Join(cfgDir, "devcontainer.json"), `{"image":"alpine"}`)

	id, err := InferID(cfgDir, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := GenerateID(dir)
	if id != want {
		t.Errorf("InferID = %q, want %q", id, want)
	}
}

func TestInferID_ConfigDir_NoDevcontainer(t *testing.T) {
	dir := t.TempDir()
	// dir exists but has no devcontainer.json -> fallback to GenerateID
	id, err := InferID(dir, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// should fall back to the parent of the given dir
	want := GenerateID(filepath.Dir(dir))
	if id != want {
		t.Errorf("InferID = %q, want %q", id, want)
	}
}

func TestInferID_Dir(t *testing.T) {
	dir := t.TempDir()
	mkdirAll(t, filepath.Join(dir, ".devcontainer"))
	writeFile(t, filepath.Join(dir, ".devcontainer", "devcontainer.json"), `{"image":"alpine"}`)

	id, err := InferID("", dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := GenerateID(dir)
	if id != want {
		t.Errorf("InferID = %q, want %q", id, want)
	}
}

func TestInferID_Dir_NoDevcontainer(t *testing.T) {
	dir := t.TempDir()

	id, err := InferID("", dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := GenerateID(dir)
	if id != want {
		t.Errorf("InferID = %q, want %q", id, want)
	}
}

func TestInferID_Cwd(t *testing.T) {
	dir := t.TempDir()
	mkdirAll(t, filepath.Join(dir, ".devcontainer"))
	writeFile(t, filepath.Join(dir, ".devcontainer", "devcontainer.json"), `{"image":"alpine"}`)

	id, err := InferID("", "", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := GenerateID(dir)
	if id != want {
		t.Errorf("InferID = %q, want %q", id, want)
	}
}

func TestInferID_Cwd_NoDevcontainer(t *testing.T) {
	dir := t.TempDir()

	id, err := InferID("", "", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := GenerateID(dir)
	if id != want {
		t.Errorf("InferID = %q, want %q", id, want)
	}
}
