package compose

import (
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
