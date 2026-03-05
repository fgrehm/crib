package cmd

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
			got := shellQuoteJoin(tt.args)
			if got != tt.want {
				t.Errorf("shellQuoteJoin(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
