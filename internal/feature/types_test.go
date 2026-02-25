package feature

import (
	"encoding/json"
	"testing"
)

func TestDependsOnUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantLen int
	}{
		{
			name:    "valid object",
			input:   `{"ghcr.io/devcontainers/features/common-utils:2": {}}`,
			wantLen: 1,
		},
		{
			name:    "valid object with options",
			input:   `{"ghcr.io/devcontainers/features/common-utils:2": {"username": "vscode"}}`,
			wantLen: 1,
		},
		{
			name:    "empty object",
			input:   `{}`,
			wantLen: 0,
		},
		{
			name:    "null",
			input:   `null`,
			wantLen: 0,
		},
		{
			name:    "rejects array",
			input:   `["ghcr.io/devcontainers/features/common-utils:2"]`,
			wantErr: true,
		},
		{
			name:    "rejects string",
			input:   `"ghcr.io/devcontainers/features/common-utils:2"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d DependsOn
			err := json.Unmarshal([]byte(tt.input), &d)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(d) != tt.wantLen {
				t.Errorf("got len %d, want %d", len(d), tt.wantLen)
			}
		})
	}
}
