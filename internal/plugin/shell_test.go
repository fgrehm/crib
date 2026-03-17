package plugin

import "testing"

func TestShellQuoteJoin(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"single", []string{"ruby"}, "'ruby'"},
		{"multiple", []string{"bundle", "install"}, "'bundle' 'install'"},
		{"with spaces", []string{"echo", "hello world"}, "'echo' 'hello world'"},
		{"with single quotes", []string{"echo", "it's"}, `'echo' 'it'\''s'`},
		{"with special chars", []string{"bash", "-c", "echo $HOME && ls"}, `'bash' '-c' 'echo $HOME && ls'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShellQuoteJoin(tt.args)
			if got != tt.want {
				t.Errorf("ShellQuoteJoin(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

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
