package compose

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"whitespace only", "  \n  \n  ", nil},
		{"single line", "abc123", []string{"abc123"}},
		{"multiple lines", "abc\ndef\n", []string{"abc", "def"}},
		{"trims whitespace", "  abc  \n  def  \n", []string{"abc", "def"}},
		{"skips empty lines", "abc\n\ndef\n\n", []string{"abc", "def"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLines(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseLines() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"single line", "hello", "hello"},
		{"with trailing newline", "hello\n", "hello"},
		{"multiple lines", "first\nsecond\nthird", "first"},
		{"leading empty lines", "\n\nfirst\n", "first"},
		{"whitespace lines", "  \n  \nhello\n", "hello"},
		{"trims result", "  hello  \n", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstLine(tt.input)
			if got != tt.want {
				t.Errorf("firstLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCutString(t *testing.T) {
	tests := []struct {
		name       string
		s, sep     string
		wantBefore string
		wantAfter  string
		wantOK     bool
	}{
		{"basic", "KEY=VALUE", "=", "KEY", "VALUE", true},
		{"no separator", "noequalssign", "=", "noequalssign", "", false},
		{"empty value", "KEY=", "=", "KEY", "", true},
		{"multiple separators", "K=V=extra", "=", "K", "V=extra", true},
		{"separator at start", "=VALUE", "=", "", "VALUE", true},
		{"empty string", "", "=", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, after, ok := cutString(tt.s, tt.sep)
			if before != tt.wantBefore || after != tt.wantAfter || ok != tt.wantOK {
				t.Errorf("cutString(%q, %q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.s, tt.sep, before, after, ok, tt.wantBefore, tt.wantAfter, tt.wantOK)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"single line no newline", "hello", []string{"hello"}},
		{"single line with newline", "hello\n", []string{"hello"}},
		{"multiple lines", "a\nb\nc", []string{"a", "b", "c"}},
		{"trailing newline", "a\nb\n", []string{"a", "b"}},
		{"empty lines", "a\n\nb", []string{"a", "", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitLines(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTrimSpace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"no whitespace", "hello", "hello"},
		{"leading spaces", "  hello", "hello"},
		{"trailing spaces", "hello  ", "hello"},
		{"both sides", "  hello  ", "hello"},
		{"tabs", "\thello\t", "hello"},
		{"carriage return", "\rhello\r", "hello"},
		{"mixed", " \t\rhello \t\r", "hello"},
		{"preserves interior", "hello world", "hello world"},
		{"preserves newlines", "hello\nworld", "hello\nworld"},
		{"only whitespace", "  \t\r  ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimSpace(tt.input)
			if got != tt.want {
				t.Errorf("trimSpace(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseEnvFile(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")
		os.WriteFile(path, []byte("FOO=bar\nBAZ=qux\n"), 0o644)

		got, err := parseEnvFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got["FOO"] != "bar" || got["BAZ"] != "qux" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("comments and empty lines", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")
		os.WriteFile(path, []byte("# comment\nFOO=bar\n\n# another\nBAZ=qux\n"), 0o644)

		got, err := parseEnvFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got["FOO"] != "bar" || got["BAZ"] != "qux" {
			t.Errorf("got %v, want {FOO:bar, BAZ:qux}", got)
		}
	})

	t.Run("value with equals", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")
		os.WriteFile(path, []byte("URL=postgres://host/db?ssl=true\n"), 0o644)

		got, err := parseEnvFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got["URL"] != "postgres://host/db?ssl=true" {
			t.Errorf("URL = %q", got["URL"])
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")
		os.WriteFile(path, []byte("  FOO  =  bar  \n"), 0o644)

		got, err := parseEnvFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got["FOO"] != "bar" {
			t.Errorf("FOO = %q, want %q", got["FOO"], "bar")
		}
	})

	t.Run("line without equals is skipped", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")
		os.WriteFile(path, []byte("FOO=bar\nno-equals\nBAZ=qux\n"), 0o644)

		got, err := parseEnvFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Errorf("got %d entries, want 2: %v", len(got), got)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := parseEnvFile("/nonexistent/.env")
		if err == nil {
			t.Error("expected error for missing file")
		}
	})
}
