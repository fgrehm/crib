package plugin

import "testing"

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"hello", "hello"},
		{"/home/user/.ssh", "/home/user/.ssh"},
		{"it's", "it'\\''s"},
		{"it''s", "it'\\'''\\''s"},
		{"'start", "'\\''start"},
		{"end'", "end'\\''"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ShellQuote(tt.input)
			if got != tt.want {
				t.Errorf("ShellQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
