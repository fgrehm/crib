package packagecache

import "testing"

func TestParseVolumeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantWS   string
		wantProv string
	}{
		{"simple", "crib-cache-myws-apt", "myws", "apt"},
		{"compose prefix", "crib-web_crib-cache-web-npm", "web", "npm"},
		{"hyphenated wsid", "crib-cache-my-project-go", "my-project", "go"},
		{"empty wsid", "crib-cache--apt", "", "apt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWS, gotProv := ParseVolumeName(tt.input)
			if gotWS != tt.wantWS || gotProv != tt.wantProv {
				t.Errorf("ParseVolumeName(%q) = (%q, %q), want (%q, %q)",
					tt.input, gotWS, gotProv, tt.wantWS, tt.wantProv)
			}
		})
	}
}
