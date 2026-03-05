package cmd

import "testing"

func TestParseVolumeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantWS   string
		wantProv string
	}{
		{
			name:     "standard format",
			input:    "crib-cache-myws-npm",
			wantWS:   "myws",
			wantProv: "npm",
		},
		{
			name:     "hyphenated workspace ID",
			input:    "crib-cache-my-project-apt",
			wantWS:   "my-project",
			wantProv: "apt",
		},
		{
			name:     "compose prefixed",
			input:    "crib-web_crib-cache-web-bundler",
			wantWS:   "web",
			wantProv: "bundler",
		},
		{
			name:     "compose prefixed with hyphenated workspace",
			input:    "crib-my-app_crib-cache-my-app-downloads",
			wantWS:   "my-app",
			wantProv: "downloads",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws, prov := parseVolumeName(tt.input)
			if ws != tt.wantWS {
				t.Errorf("workspaceID = %q, want %q", ws, tt.wantWS)
			}
			if prov != tt.wantProv {
				t.Errorf("provider = %q, want %q", prov, tt.wantProv)
			}
		})
	}
}
