package ui

import (
	"bytes"
	"strings"
	"testing"
)

func newTestUI() (*UI, *bytes.Buffer, *bytes.Buffer) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	u := New(out, errOut)
	return u, out, errOut
}

func TestHeader(t *testing.T) {
	u, out, _ := newTestUI()
	u.Header("Starting workspace")
	got := out.String()
	if !strings.Contains(got, "==> Starting workspace") {
		t.Errorf("Header output = %q, want to contain %q", got, "==> Starting workspace")
	}
}

func TestSuccess(t *testing.T) {
	u, out, _ := newTestUI()
	u.Success("Image built")
	got := out.String()
	// Non-TTY uses "ok" prefix.
	if !strings.Contains(got, "  ok Image built") {
		t.Errorf("Success output = %q, want to contain %q", got, "  ok Image built")
	}
}

func TestKeyval(t *testing.T) {
	u, out, _ := newTestUI()
	u.Keyval("container", "abc123def456")
	got := out.String()
	if !strings.Contains(got, "container") || !strings.Contains(got, "abc123def456") {
		t.Errorf("Keyval output = %q, want to contain key and value", got)
	}
	// Verify indentation.
	if !strings.HasPrefix(got, "  ") {
		t.Errorf("Keyval output should start with two spaces, got %q", got)
	}
}

func TestDim(t *testing.T) {
	u, out, _ := newTestUI()
	u.Dim("No workspaces")
	got := out.String()
	if !strings.Contains(got, "No workspaces") {
		t.Errorf("Dim output = %q, want to contain %q", got, "No workspaces")
	}
}

func TestError(t *testing.T) {
	u, _, errOut := newTestUI()
	u.Error("something failed")
	got := errOut.String()
	if !strings.Contains(got, "error: something failed") {
		t.Errorf("Error output = %q, want to contain %q", got, "error: something failed")
	}
}

func TestStatusColor_NonTTY(t *testing.T) {
	u, _, _ := newTestUI()
	got := u.StatusColor("running")
	if got != "running" {
		t.Errorf("StatusColor() = %q, want %q (no ANSI in non-TTY)", got, "running")
	}
}

func TestTable(t *testing.T) {
	u, out, _ := newTestUI()
	headers := []string{"WORKSPACE", "SOURCE"}
	rows := [][]string{
		{"myproject", "/home/user/myproject"},
		{"other", "/home/user/other"},
	}
	u.Table(headers, rows)
	got := out.String()
	if !strings.Contains(got, "WORKSPACE") {
		t.Errorf("Table output missing header, got %q", got)
	}
	if !strings.Contains(got, "myproject") {
		t.Errorf("Table output missing row data, got %q", got)
	}
	// Verify alignment: headers and first row should start at the same column.
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) < 2 {
		t.Fatalf("Table should have at least 2 lines, got %d", len(lines))
	}
	hdrSourceIdx := strings.Index(lines[0], "SOURCE")
	rowSourceIdx := strings.Index(lines[1], "/home/user/myproject")
	if hdrSourceIdx != rowSourceIdx {
		t.Errorf("Column alignment mismatch: header SOURCE at %d, row data at %d", hdrSourceIdx, rowSourceIdx)
	}
}

func TestTable_Empty(t *testing.T) {
	u, out, _ := newTestUI()
	u.Table(nil, nil)
	if out.String() != "" {
		t.Errorf("Table with no headers should produce no output, got %q", out.String())
	}
}
