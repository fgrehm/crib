package feature

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTarBuffer(t *testing.T, entries []tarEntry) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: e.typeflag,
			Mode:     0o644,
		}
		switch e.typeflag {
		case tar.TypeDir:
			hdr.Mode = 0o755
		case tar.TypeReg:
			hdr.Size = int64(len(e.body))
		case tar.TypeSymlink:
			hdr.Linkname = e.linkname
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("writing tar header %q: %v", e.name, err)
		}
		if e.typeflag == tar.TypeReg && len(e.body) > 0 {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("writing tar body %q: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}
	return &buf
}

type tarEntry struct {
	name     string
	typeflag byte
	body     string
	linkname string
}

func TestExtractTar_RegularFiles(t *testing.T) {
	dir := t.TempDir()
	buf := createTarBuffer(t, []tarEntry{
		{name: "hello.txt", typeflag: tar.TypeReg, body: "hello"},
		{name: "sub/nested.txt", typeflag: tar.TypeReg, body: "nested"},
	})

	if err := extractTar(buf, dir); err != nil {
		t.Fatalf("extractTar: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatalf("reading hello.txt: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("hello.txt = %q, want %q", data, "hello")
	}

	data, err = os.ReadFile(filepath.Join(dir, "sub", "nested.txt"))
	if err != nil {
		t.Fatalf("reading sub/nested.txt: %v", err)
	}
	if string(data) != "nested" {
		t.Errorf("sub/nested.txt = %q, want %q", data, "nested")
	}
}

func TestExtractTar_PathTraversalNeutralized(t *testing.T) {
	// "../escape.txt" is neutralized to "escape.txt" by filepath.Clean
	// before the traversal check. Verify it lands safely inside dir.
	dir := t.TempDir()
	buf := createTarBuffer(t, []tarEntry{
		{name: "../escape.txt", typeflag: tar.TypeReg, body: "safe"},
	})

	if err := extractTar(buf, dir); err != nil {
		t.Fatalf("extractTar: %v", err)
	}

	// Should be extracted as "escape.txt" inside dir (not outside).
	data, err := os.ReadFile(filepath.Join(dir, "escape.txt"))
	if err != nil {
		t.Fatalf("reading escape.txt: %v", err)
	}
	if string(data) != "safe" {
		t.Errorf("escape.txt = %q, want %q", data, "safe")
	}
	// Verify nothing was written outside dir.
	if _, err := os.Stat(filepath.Join(dir, "..", "escape.txt")); err == nil {
		t.Error("file should not have been created outside extraction dir")
	}
}

func TestExtractTar_SafeSymlink(t *testing.T) {
	dir := t.TempDir()
	buf := createTarBuffer(t, []tarEntry{
		{name: "real.txt", typeflag: tar.TypeReg, body: "target"},
		{name: "link.txt", typeflag: tar.TypeSymlink, linkname: "real.txt"},
	})

	if err := extractTar(buf, dir); err != nil {
		t.Fatalf("extractTar: %v", err)
	}

	target, err := os.Readlink(filepath.Join(dir, "link.txt"))
	if err != nil {
		t.Fatalf("reading symlink: %v", err)
	}
	if target != "real.txt" {
		t.Errorf("symlink target = %q, want %q", target, "real.txt")
	}
}

func TestExtractTar_SymlinkAbsoluteEscape(t *testing.T) {
	dir := t.TempDir()
	buf := createTarBuffer(t, []tarEntry{
		{name: "evil", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
	})

	err := extractTar(buf, dir)
	if err == nil {
		t.Fatal("expected error for absolute symlink escape")
	}
	if !strings.Contains(err.Error(), "absolute symlink") {
		t.Errorf("error = %q, want mention of absolute symlink", err)
	}
}

func TestExtractTar_SymlinkRelativeEscape(t *testing.T) {
	dir := t.TempDir()
	buf := createTarBuffer(t, []tarEntry{
		{name: "sub/evil", typeflag: tar.TypeSymlink, linkname: "../../etc/passwd"},
	})

	err := extractTar(buf, dir)
	if err == nil {
		t.Fatal("expected error for relative symlink escape")
	}
	if !strings.Contains(err.Error(), "symlink escaping") {
		t.Errorf("error = %q, want mention of symlink escaping", err)
	}
}

func TestExtractTar_SymlinkToSubdir(t *testing.T) {
	dir := t.TempDir()
	buf := createTarBuffer(t, []tarEntry{
		{name: "sub/file.txt", typeflag: tar.TypeReg, body: "data"},
		{name: "link", typeflag: tar.TypeSymlink, linkname: "sub/file.txt"},
	})

	if err := extractTar(buf, dir); err != nil {
		t.Fatalf("extractTar: %v", err)
	}

	// Symlink within dir should be created.
	target, err := os.Readlink(filepath.Join(dir, "link"))
	if err != nil {
		t.Fatalf("reading symlink: %v", err)
	}
	if target != "sub/file.txt" {
		t.Errorf("symlink target = %q, want %q", target, "sub/file.txt")
	}
}
